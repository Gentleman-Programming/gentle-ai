package engram

import (
	"errors"
	"fmt"

	"github.com/gentleman-programming/gentle-ai/internal/platform"
	"github.com/gentleman-programming/gentle-ai/internal/state"
)

// Action represents the user's choice for Engram data directory handling.
type Action int

const (
	ActionKeepDefault Action = iota
	ActionMigrate
	ActionStartFresh
	ActionClean
)

// Sentinel errors for DataBackend and DataDirService operations.
var (
	ErrLocked            = errors.New("engram data is locked by another process")
	ErrInsufficientSpace = errors.New("insufficient disk space")
	ErrPathNotWritable   = errors.New("path is not writable")
	ErrInvalidPath       = errors.New("invalid path")
)

// FileInfo describes a single Engram SQLite file for preview purposes.
type FileInfo struct {
	Name string
	Size uint64
}

// Preview holds the information shown to the user before a destructive action.
type Preview struct {
	Files          []FileInfo
	TotalBytes     uint64
	AvailableSpace uint64
	SpaceErr       string
	ExpandedPath   string // absolute path with ~ expanded
}

// HasEnoughSpace reports whether the target has enough space for the operation.
// For non-migration actions (StartFresh, Clean), this is always true.
// For Migrate, it checks AvailableSpace >= TotalBytes.
func (p Preview) HasEnoughSpace() bool {
	if p.TotalBytes == 0 {
		return true
	}
	return p.AvailableSpace >= p.TotalBytes
}

// Result holds the outcome of a successful DataDirService.Execute call.
type Result struct {
	FilesMoved int
	BytesMoved uint64
	Message    string
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
	MigrateData(source, target string) (Result, error)
	CleanData(dir string) error
	EnsureDir(dir string) error

	// Space checking
	AvailableSpace(dir string) (uint64, error)
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
	backend   DataBackend
	persister ConfigPersister
}

// NewDataDirService creates a service with the given backend and persister.
func NewDataDirService(backend DataBackend, persister ConfigPersister) *DataDirService {
	return &DataDirService{backend: backend, persister: persister}
}

// Preview returns a preview for the given action and input path.
// The path is only relevant for Migrate and StartFresh actions.
// Call this once when the user presses Continue (not on every keystroke).
func (s *DataDirService) Preview(action Action, inputPath string) (Preview, error) {
	var p Preview

	switch action {
	case ActionMigrate, ActionStartFresh:
		expanded, err := s.backend.ExpandPath(inputPath)
		if err != nil {
			return p, fmt.Errorf("%w: %v", ErrInvalidPath, err)
		}
		p.ExpandedPath = expanded

		// Show existing files from hard default so the user knows what they
		// stand to lose (migrate moves them, start-fresh deletes them).
		src := s.backend.HardDefaultDataDir()
		files, total, err := s.backend.EstimateMigration(src)
		if err != nil {
			return p, err
		}
		p.Files = files
		p.TotalBytes = total

		space, err := s.backend.AvailableSpace(expanded)
		if err != nil {
			p.SpaceErr = err.Error()
		} else {
			p.AvailableSpace = space
		}
	case ActionClean:
		src := s.backend.HardDefaultDataDir()
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
		src := s.backend.HardDefaultDataDir()
		if locked, _ := s.backend.DetectLockedData(src); locked {
			return Result{}, ErrLocked
		}
		if err := s.backend.CleanData(src); err != nil {
			return Result{}, err
		}
		return Result{Message: "All Engram data has been permanently deleted."}, nil
	case ActionMigrate:
		src := s.backend.HardDefaultDataDir()
		if locked, _ := s.backend.DetectLockedData(src); locked {
			return Result{}, ErrLocked
		}
		result, err := s.backend.MigrateData(src, path)
		if err != nil {
			return Result{}, err
		}
		if err := s.persister.Write(path); err != nil {
			return Result{}, fmt.Errorf("data moved to %s but config could not be saved: %w", path, err)
		}
		result.Message = "Engram data has been moved successfully."
		return result, nil
	case ActionStartFresh:
		src := s.backend.HardDefaultDataDir()
		if s.backend.DetectExistingData(src) {
			if locked, _ := s.backend.DetectLockedData(src); locked {
				return Result{}, ErrLocked
			}
			if err := s.backend.CleanData(src); err != nil {
				return Result{}, err
			}
		}
		if err := s.backend.EnsureDir(path); err != nil {
			return Result{}, err
		}
		if err := s.persister.Write(path); err != nil {
			return Result{}, fmt.Errorf("new database directory ready at %s but config could not be saved: %w", path, err)
		}
		return Result{Message: "A new empty database will be created at the selected location."}, nil
	default:
		return Result{}, fmt.Errorf("unknown action: %v", action)
	}
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
func (p *LocalConfigPersister) Write(dir string) error {
	if s, err := state.Read(p.homeDir); err == nil {
		s.EngramDataDir = dir
		if err := state.Write(p.homeDir, s); err != nil {
			return err
		}
	} else {
		if err := state.Write(p.homeDir, state.InstallState{EngramDataDir: dir}); err != nil {
			return err
		}
	}
	_ = setDataDirEnv(dir)
	_ = platform.PersistEngramEnv(dir)
	return nil
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
