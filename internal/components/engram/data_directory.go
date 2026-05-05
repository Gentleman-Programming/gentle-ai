// Package engram handles Engram data directory configuration, migration,
// and management. It provides a DataBackend abstraction for filesystem
// operations and a DataDirService that orchestrates the user flow with
// transactional safety guarantees.
package engram

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/platform"
	"github.com/gentleman-programming/gentle-ai/internal/state"
)

// Action represents the user's choice for Engram data directory handling.
type Action int

const (
	ActionKeepDefault Action = iota
	ActionMigrate
	ActionCopy
	ActionSetActive
	ActionStartFresh
	ActionClean
)

// Sentinel errors for DataBackend and DataDirService operations.
var (
	ErrLocked            = errors.New("engram data is locked by another process")
	ErrInsufficientSpace = errors.New("insufficient disk space")
	ErrPathNotWritable   = errors.New("path is not writable")
	ErrInvalidPath       = errors.New("invalid path")
	ErrTargetHasData     = errors.New("target already contains engram data")
	ErrNoEngramData      = errors.New("selected directory does not contain engram data")
)

// FileInfo describes a single Engram SQLite file for preview purposes.
type FileInfo struct {
	Name string
	Size uint64
}

// Preview holds the information shown to the user before a destructive action.
type Preview struct {
	Files                   []FileInfo
	TotalBytes              uint64
	AvailableSpace          uint64
	SpaceErr                error  // nil when space check succeeded
	ExpandedPath            string // absolute path with ~ expanded
	PartialMigrationWarning string // set when data exists in both source and target
}

// HasEnoughSpace reports whether the target has enough space for the operation.
// Operations with no bytes to transfer are always safe. Otherwise this checks
// AvailableSpace >= TotalBytes for the selected target.
func (p Preview) HasEnoughSpace() bool {
	if p.TotalBytes == 0 {
		return true
	}
	return p.AvailableSpace >= p.TotalBytes
}

// Result holds the outcome of a successful DataDirService.Execute call.
type Result struct {
	FilesMoved   int
	BytesMoved   uint64
	FilesCopied  int
	BytesCopied  uint64
	FilesDeleted int
	BytesDeleted uint64
	Message      string
}

// DataBackend abstracts all filesystem operations related to Engram data.
// This is the single extension point for swapping local ↔ sync backends.
type DataBackend interface {
	// Path resolution
	DefaultDataDir() string
	HardDefaultDataDir() string
	ExpandPath(path string) (string, error)

	// Detection
	DetectExistingData(dir string) bool
	ExistingFiles(dir string) []string
	DetectLockedData(dir string) (bool, error)

	// Operations
	EstimateMigration(source string) ([]FileInfo, uint64, error)
	CopyData(source, target string) (Result, error)
	MoveData(source, target string) (Result, error)
	DeleteData(dir string) (Result, error)
	MigrateData(source, target string) (Result, error) // deprecated: use CopyData/MoveData
	CleanData(dir string) error
	EnsureDir(dir string) error

	// Space checking
	AvailableSpace(dir string) (uint64, error)

	// Validation
	CheckWritable(dir string) error
}

// EffectiveSourceDataDir returns the source directory that should be used when the
// user chooses an operation that depends on existing Engram data. Prefer the effective
// configured directory, but fall back to the hard default when the effective
// location is empty and legacy/default data still exists there.
func EffectiveSourceDataDir(backend DataBackend) string {
	effective := backend.DefaultDataDir()
	if backend.DetectExistingData(effective) {
		return effective
	}

	hardDefault := backend.HardDefaultDataDir()
	if !sameFilesystemPath(effective, hardDefault) && backend.DetectExistingData(hardDefault) {
		return hardDefault
	}

	return effective
}

// HasExistingSourceData reports whether any operation can use existing Engram data.
func HasExistingSourceData(backend DataBackend) bool {
	return backend.DetectExistingData(EffectiveSourceDataDir(backend))
}

// ConfigPersister handles where the ENGRAM_DATA_DIR configuration is stored.
// Separated from DataBackend because config storage (env vars, state file,
// shell profile) is independent from data storage location.
type ConfigPersister interface {
	Read() (string, error)
	Write(dir string) error
	Clear() error
}

// DataDirService orchestrates the Engram data directory user flow.
// It uses a DataBackend for filesystem operations and a ConfigPersister
// for saving the user's choice.
type DataDirService struct {
	backend            DataBackend
	persister          ConfigPersister
	afterConfigPersist func(action Action, target string) error
}

// NewDataDirService creates a service with the given backend and persister.
func NewDataDirService(backend DataBackend, persister ConfigPersister) *DataDirService {
	return &DataDirService{backend: backend, persister: persister}
}

// SetAfterConfigPersist installs a hook that runs after the active data-dir
// config is persisted and before any move-source cleanup happens. UI callers
// use this to reinject MCP configs before old data is removed.
func (s *DataDirService) SetAfterConfigPersist(fn func(action Action, target string) error) {
	s.afterConfigPersist = fn
}

func (s *DataDirService) runAfterConfigPersist(action Action, target string) error {
	if s.afterConfigPersist == nil {
		return nil
	}
	return s.afterConfigPersist(action, target)
}

// Preview returns a preview for the given action and input path.
// The path is only relevant for Migrate and StartFresh actions.
// Call this once when the user presses Continue (not on every keystroke).
func (s *DataDirService) Preview(action Action, inputPath string) (Preview, error) {
	var p Preview

	switch action {
	case ActionMigrate, ActionCopy, ActionSetActive, ActionStartFresh:
		expanded, err := s.backend.ExpandPath(inputPath)
		if err != nil {
			return p, fmt.Errorf("%w: %v", ErrInvalidPath, err)
		}
		p.ExpandedPath = expanded

		if action == ActionSetActive {
			files, total, err := s.backend.EstimateMigration(expanded)
			if err != nil {
				return p, err
			}
			p.Files = files
			p.TotalBytes = total
		} else {
			// Show existing files from the actual source so the user knows what will be moved, copied, or deleted.
			src := EffectiveSourceDataDir(s.backend)
			files, total, err := s.backend.EstimateMigration(src)
			if err != nil {
				return p, err
			}
			p.Files = files
			p.TotalBytes = total
		}

		space, err := s.backend.AvailableSpace(expanded)
		if err != nil {
			p.SpaceErr = err
		} else {
			p.AvailableSpace = space
		}

		// Check for interrupted migration: if both source and target have data,
		// warn the user that they might be in an inconsistent state.
		if action == ActionMigrate || action == ActionCopy {
			src := EffectiveSourceDataDir(s.backend)
			srcFiles := s.backend.ExistingFiles(src)
			dstFiles := s.backend.ExistingFiles(expanded)
			if len(srcFiles) > 0 && len(dstFiles) > 0 {
				operation := "Migration"
				if action == ActionCopy {
					operation = "Copy"
				}
				p.PartialMigrationWarning = fmt.Sprintf(
					"Found data in both locations. %s to %s is blocked until one location is cleaned or chosen explicitly.",
					operation,
					expanded,
				)
			}
		}
	case ActionClean:
		src := EffectiveSourceDataDir(s.backend)
		files, total, err := s.backend.EstimateMigration(src)
		if err != nil {
			return p, err
		}
		p.Files = files
		p.TotalBytes = total
	}

	return p, nil
}

// Execute performs the confirmed action and returns the result.
func (s *DataDirService) Execute(action Action, path string) (Result, error) {
	switch action {
	case ActionKeepDefault:
		if err := s.persister.Clear(); err != nil {
			return Result{}, err
		}
		return Result{Message: "Using default Engram data location."}, nil
	case ActionClean:
		src := EffectiveSourceDataDir(s.backend)
		files, totalBytes, err := s.backend.EstimateMigration(src)
		if err != nil {
			return Result{}, err
		}
		if locked, _ := s.backend.DetectLockedData(src); locked {
			return Result{}, ErrLocked
		}
		// Best-effort temp backup before destructive operation.
		backupDir, backupErr := s.backupBeforeClean(src)
		if backupErr != nil {
			log.Printf("engram: clean temp backup failed (proceeding anyway): %v", backupErr)
		}
		_, delErr := s.backend.DeleteData(src)
		if delErr != nil {
			return Result{}, delErr
		}
		// Success: remove temp backup.
		if backupDir != "" {
			_ = os.RemoveAll(backupDir)
		}
		return Result{FilesDeleted: len(files), BytesDeleted: totalBytes, Message: "All Engram data has been permanently deleted."}, nil
	case ActionCopy:
		src := EffectiveSourceDataDir(s.backend)
		target, err := s.backend.ExpandPath(path)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrInvalidPath, err)
		}
		if sameFilesystemPath(src, target) {
			return Result{}, fmt.Errorf("%w: target is already the current Engram data location", ErrInvalidPath)
		}
		if len(s.backend.ExistingFiles(target)) > 0 {
			return Result{}, ErrTargetHasData
		}
		if locked, _ := s.backend.DetectLockedData(src); locked {
			return Result{}, ErrLocked
		}
		result, err := s.backend.CopyData(src, target)
		if err != nil {
			return Result{}, err
		}
		result.Message = "Engram data has been copied successfully. The current data location is unchanged."
		return result, nil
	case ActionSetActive:
		target, err := s.backend.ExpandPath(path)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrInvalidPath, err)
		}
		if !s.backend.DetectExistingData(target) {
			return Result{}, ErrNoEngramData
		}
		if locked, _ := s.backend.DetectLockedData(target); locked {
			return Result{}, ErrLocked
		}
		if err := s.backend.CheckWritable(target); err != nil {
			return Result{}, err
		}
		if err := s.persister.Write(target); err != nil {
			return Result{}, fmt.Errorf("active data directory %s could not be saved: %w", target, err)
		}
		if err := s.runAfterConfigPersist(action, target); err != nil {
			return Result{}, fmt.Errorf("active data directory %s was saved but MCP config could not be updated: %w", target, err)
		}
		files, totalBytes, _ := s.backend.EstimateMigration(target)
		return Result{FilesCopied: len(files), BytesCopied: totalBytes, Message: "Active Engram data directory has been updated."}, nil
	case ActionMigrate:
		src := EffectiveSourceDataDir(s.backend)
		target, err := s.backend.ExpandPath(path)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrInvalidPath, err)
		}
		if sameFilesystemPath(src, target) {
			return Result{}, fmt.Errorf("%w: target is already the current Engram data location", ErrInvalidPath)
		}
		if len(s.backend.ExistingFiles(target)) > 0 {
			return Result{}, ErrTargetHasData
		}
		if locked, _ := s.backend.DetectLockedData(src); locked {
			return Result{}, ErrLocked
		}
		result, err := s.backend.CopyData(src, target)
		if err != nil {
			return Result{}, err
		}
		result.FilesMoved = result.FilesCopied
		result.BytesMoved = result.BytesCopied
		result.FilesCopied = 0
		result.BytesCopied = 0
		if err := s.persister.Write(target); err != nil {
			return Result{}, fmt.Errorf("data copied to %s but config could not be saved: %w", target, err)
		}
		if err := s.runAfterConfigPersist(action, target); err != nil {
			return Result{}, fmt.Errorf("data copied to %s and config saved but MCP config could not be updated: %w", target, err)
		}
		// Config and MCP wiring are persisted — now it is safe to remove the source.
		_, delErr := s.backend.DeleteData(src)
		if delErr != nil {
			return Result{}, fmt.Errorf("config saved but could not remove old data from %s: %w", src, delErr)
		}
		result.Message = "Engram data has been moved successfully."
		return result, nil
	case ActionStartFresh:
		src := EffectiveSourceDataDir(s.backend)
		files, totalBytes, estimateErr := s.backend.EstimateMigration(src)
		if estimateErr != nil {
			return Result{}, estimateErr
		}
		target, err := s.backend.ExpandPath(path)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrInvalidPath, err)
		}
		if !sameFilesystemPath(src, target) && len(s.backend.ExistingFiles(target)) > 0 {
			return Result{}, ErrTargetHasData
		}
		if s.backend.DetectExistingData(src) {
			if locked, _ := s.backend.DetectLockedData(src); locked {
				return Result{}, ErrLocked
			}
			_, delErr := s.backend.DeleteData(src)
			if delErr != nil {
				return Result{}, delErr
			}
		}
		if err := s.backend.EnsureDir(target); err != nil {
			return Result{}, err
		}
		if err := s.persister.Write(target); err != nil {
			return Result{}, fmt.Errorf("new database directory ready at %s but config could not be saved: %w", target, err)
		}
		if err := s.runAfterConfigPersist(action, target); err != nil {
			return Result{}, fmt.Errorf("new database directory ready at %s and config saved but MCP config could not be updated: %w", target, err)
		}
		return Result{FilesDeleted: len(files), BytesDeleted: totalBytes, Message: "A new empty database will be created at the selected location."}, nil
	default:
		return Result{}, fmt.Errorf("unknown action: %v", action)
	}
}

// backupBeforeClean creates a temporary backup of Engram SQLite files before
// a destructive Clean operation. The caller is responsible for removing the
// backup directory after successful cleanup. Returns ("", nil) on best-effort
// failure so that Clean can still proceed.
func (s *DataDirService) backupBeforeClean(src string) (string, error) {
	files := s.backend.ExistingFiles(src)
	if len(files) == 0 {
		return "", nil
	}
	backupDir, err := os.MkdirTemp("", "gentle-ai-engram-clean-*")
	if err != nil {
		return "", err
	}
	for _, f := range files {
		srcPath := filepath.Join(src, f)
		dstPath := filepath.Join(backupDir, f)
		info, err := os.Stat(srcPath)
		if err != nil {
			_ = os.RemoveAll(backupDir)
			return "", err
		}
		if err := copyFileBuffered(srcPath, dstPath, info.Mode()); err != nil {
			_ = os.RemoveAll(backupDir)
			return "", err
		}
	}
	return backupDir, nil
}

// LocalConfigPersister implements ConfigPersister using state.json + env var + shell profile.
type LocalConfigPersister struct {
	homeDir string
}

// NewLocalConfigPersister creates a persister for the given home directory.
func NewLocalConfigPersister(homeDir string) *LocalConfigPersister {
	return &LocalConfigPersister{homeDir: homeDir}
}

// Read returns the currently configured Engram data directory.
// Priority: env var > state file > empty.
func (p *LocalConfigPersister) Read() (string, error) {
	if dir := getDataDirEnv(); dir != "" {
		return dir, nil
	}
	if s, err := state.Read(p.homeDir); err == nil && s.EngramDataDir != "" {
		return s.EngramDataDir, nil
	}
	return "", nil
}

// Write persists the data directory to state, env var, and shell profile.
//
// The state file write is atomic: it writes to a temporary file and renames
// it into place. This prevents corruption if the process crashes mid-write.
// However, the read-modify-write sequence is still NOT safe for concurrent
// use across multiple processes. In practice this is acceptable because the
// Engram data directory is configured once during installation and not
// modified concurrently.
func (p *LocalConfigPersister) Write(dir string) error {
	var s state.InstallState
	if existing, err := state.Read(p.homeDir); err == nil {
		s = existing
	}
	s.EngramDataDir = dir

	if err := p.atomicStateWrite(s); err != nil {
		return err
	}

	_ = setDataDirEnv(dir)
	_ = platform.PersistEngramEnv(dir)
	return nil
}

// atomicStateWrite writes the state atomically using a temp file + rename.
func (p *LocalConfigPersister) atomicStateWrite(s state.InstallState) error {
	statePath := state.Path(p.homeDir)
	tmpPath := statePath + ".tmp"

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, statePath)
}

// Clear removes the custom data directory configuration.
func (p *LocalConfigPersister) Clear() error {
	_ = unsetDataDirEnv()
	if s, err := state.Read(p.homeDir); err == nil {
		s.EngramDataDir = ""
		_ = state.Write(p.homeDir, s)
	}
	_ = platform.RemoveEngramEnv()
	return nil
}

// getDataDirEnv, setDataDirEnv, unsetDataDirEnv are thin wrappers so env.go
// owns the env var name constant while data_directory.go can use them.
func getDataDirEnv() string {
	return getEngramDataDirEnv()
}

func setDataDirEnv(dir string) error {
	return setEngramDataDirEnv(dir)
}

func unsetDataDirEnv() error {
	return unsetEngramDataDirEnv()
}
