package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupInfo contains presentation-ready metadata and disk footprint for a single backup.
type BackupInfo struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Timestamp   time.Time    `json:"timestamp"`
	Source      BackupSource `json:"source,omitempty"`
	Description string       `json:"description,omitempty"`
	FileCount   int          `json:"file_count"`
	SizeBytes   int64        `json:"size_bytes"`
	SizeHuman   string       `json:"size_human"`
	Pinned      bool         `json:"pinned,omitempty"`
	Age         string       `json:"age,omitempty"`
}

// BackupListReport represents the aggregated result of querying the backup directory.
type BackupListReport struct {
	BackupRoot string       `json:"backup_root"`
	TotalCount int          `json:"total_count"`
	TotalBytes int64        `json:"total_bytes"`
	TotalHuman string       `json:"total_human"`
	Backups    []BackupInfo `json:"backups"`
}

// ListBackups scans backupDir, parses all valid manifests, calculates directory sizes,
// and returns a BackupListReport sorted newest-first.
func ListBackups(backupDir string) (BackupListReport, error) {
	manifests, err := listManifests(backupDir)
	if err != nil {
		return BackupListReport{BackupRoot: backupDir}, err
	}

	// Sort newest-first
	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].CreatedAt.After(manifests[j].CreatedAt)
	})

	now := time.Now()
	var (
		totalBytes int64
		backups    = make([]BackupInfo, 0, len(manifests))
	)

	for _, m := range manifests {
		sizeBytes, _ := DirSizeBytes(m.RootDir)
		totalBytes += sizeBytes

		fileCount := m.FileCount
		if fileCount == 0 && len(m.Entries) > 0 {
			for _, e := range m.Entries {
				if e.Existed {
					fileCount++
				}
			}
		}

		info := BackupInfo{
			ID:          m.ID,
			Name:        filepath.Base(m.RootDir),
			Timestamp:   m.CreatedAt,
			Source:      m.Source,
			Description: m.Description,
			FileCount:   fileCount,
			SizeBytes:   sizeBytes,
			SizeHuman:   FormatBytes(sizeBytes),
			Pinned:      m.Pinned,
			Age:         FormatAge(m.CreatedAt, now),
		}
		backups = append(backups, info)
	}

	return BackupListReport{
		BackupRoot: backupDir,
		TotalCount: len(backups),
		TotalBytes: totalBytes,
		TotalHuman: FormatBytes(totalBytes),
		Backups:    backups,
	}, nil
}

// DirSizeBytes computes the cumulative size of all regular files under dirPath.
// If dirPath does not exist, it returns (0, nil). Non-fatal read errors for individual
// files are skipped without failing the entire walk.
func DirSizeBytes(dirPath string) (int64, error) {
	info, err := os.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat dir %q: %w", dirPath, err)
	}
	if !info.IsDir() {
		return info.Size(), nil
	}

	var total int64
	err = filepath.WalkDir(dirPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // Skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}
		fi, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if fi.Mode().IsRegular() {
			total += fi.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk dir %q: %w", dirPath, err)
	}
	return total, nil
}

// FormatBytes formats byte counts into concise human-readable units (B, KB, MB, GB).
func FormatBytes(bytes int64) string {
	if bytes < 0 {
		return "0 B"
	}
	const (
		unitKB = 1024
		unitMB = 1024 * unitKB
		unitGB = 1024 * unitMB
	)
	switch {
	case bytes >= unitGB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(unitGB))
	case bytes >= unitMB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(unitMB))
	case bytes >= unitKB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(unitKB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// FormatAge returns a concise relative age string (e.g. "2h ago", "5d ago", "just now").
func FormatAge(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	diff := now.Sub(t)
	if diff < 0 {
		return "just now"
	}

	hours := int(diff.Hours())
	days := hours / 24
	if days > 0 {
		return fmt.Sprintf("%dd ago", days)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh ago", hours)
	}
	minutes := int(diff.Minutes())
	if minutes > 0 {
		return fmt.Sprintf("%dm ago", minutes)
	}
	return "just now"
}

// CleanBackups retains the keepCount most recent unpinned backups and deletes the rest.
// It delegates to Prune and returns the deleted backup IDs.
func CleanBackups(backupDir string, keepCount int) ([]string, error) {
	if keepCount < 0 {
		return nil, fmt.Errorf("invalid keep count %d: must be >= 0", keepCount)
	}
	return Prune(backupDir, keepCount)
}

// ShortReason returns a concise display string for the backup reason/source.
func (b BackupInfo) ShortReason() string {
	if b.Description != "" {
		return b.Description
	}
	if b.Source != "" {
		return b.Source.Label()
	}
	if strings.HasPrefix(b.Name, "upgrade-") {
		return "upgrade"
	}
	if strings.HasPrefix(b.Name, "pre-migration-") {
		return "pre-migration"
	}
	return "snapshot"
}
