package backup

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"time"
)

// BackupInfo holds metadata and disk usage information for a single backup.
type BackupInfo struct {
	Name      string    `json:"name"`
	Timestamp string    `json:"timestamp"`
	FileCount int       `json:"file_count"`
	SizeBytes int64     `json:"size_bytes"`
	SizeHuman string    `json:"size_human"`
	Reason    string    `json:"reason,omitempty"`
	Pinned    bool      `json:"pinned,omitempty"`
	CreatedAt time.Time `json:"-"`
	RootDir   string    `json:"-"`
}

// BackupReport contains the inventory of backups and aggregate metrics.
type BackupReport struct {
	BackupRoot string       `json:"backup_root"`
	TotalCount int          `json:"total_count"`
	TotalBytes int64        `json:"total_bytes"`
	Backups    []BackupInfo `json:"backups"`
}

// FormatSize formats bytes into a human-readable string (B, KB, MB, GB).
func FormatSize(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// FormatAge returns a human-readable relative age string (e.g. "2h ago", "5d ago", "just now").
func FormatAge(t time.Time, now time.Time) string {
	diff := now.Sub(t)
	if diff < 0 {
		diff = 0
	}
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		m := int(diff.Minutes())
		return fmt.Sprintf("%dm ago", m)
	case diff < 24*time.Hour:
		h := int(diff.Hours())
		return fmt.Sprintf("%dh ago", h)
	default:
		d := int(diff.Hours() / 24)
		return fmt.Sprintf("%dd ago", d)
	}
}

// DirSize calculates the total size in bytes of all regular files in a directory.
func DirSize(path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// ListBackupsReport inspects backupDir and returns a complete inventory with sizes.
// Results are sorted newest-first by creation timestamp.
// If backupDir does not exist, an empty report is returned without error.
func ListBackupsReport(backupDir string) (BackupReport, error) {
	report := BackupReport{
		BackupRoot: backupDir,
		Backups:    []BackupInfo{},
	}

	manifests, err := listManifests(backupDir)
	if err != nil {
		return report, err
	}

	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].CreatedAt.After(manifests[j].CreatedAt)
	})

	for _, m := range manifests {
		size, err := DirSize(m.RootDir)
		if err != nil {
			size = 0
		}
		name := m.ID
		if name == "" {
			name = filepath.Base(m.RootDir)
		}

		reason := m.Description
		if reason == "" {
			reason = m.Source.Label()
		}

		fileCount := m.FileCount
		if fileCount == 0 && len(m.Entries) > 0 {
			for _, e := range m.Entries {
				if e.Existed {
					fileCount++
				}
			}
		}

		info := BackupInfo{
			Name:      name,
			Timestamp: m.CreatedAt.UTC().Format(time.RFC3339),
			FileCount: fileCount,
			SizeBytes: size,
			SizeHuman: FormatSize(size),
			Reason:    reason,
			Pinned:    m.Pinned,
			CreatedAt: m.CreatedAt,
			RootDir:   m.RootDir,
		}
		report.Backups = append(report.Backups, info)
		report.TotalBytes += size
	}
	report.TotalCount = len(report.Backups)

	return report, nil
}
