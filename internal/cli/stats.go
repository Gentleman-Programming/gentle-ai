package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/state"
	"github.com/gentleman-programming/gentle-ai/internal/storage"
)

// RunStats handles `gentle-ai stats <target>` subcommands.
func RunStats(args []string, w io.Writer) error {
	if len(args) == 0 {
		return errors.New(`stats: usage: gentle-ai stats engram`)
	}
	switch args[0] {
	case "--help", "-h":
		fmt.Fprintf(w, "usage: gentle-ai stats engram\n")
		fmt.Fprintf(w, "Print Engram data path (state vs default), SQLite file sizes, volume free space, and suggested locations.\n")
		return nil
	case "engram":
		if len(args) > 1 {
			return fmt.Errorf("stats engram: unexpected arguments: %s", strings.Join(args[1:], " "))
		}
		return writeEngramStats(w)
	default:
		return fmt.Errorf("stats: unknown target %q (supported: engram)", args[0])
	}
}

func nearestExistingPath(path string) string {
	clean := filepath.Clean(path)
	for {
		if _, err := os.Stat(clean); err == nil {
			return clean
		}
		parent := filepath.Dir(clean)
		if parent == clean {
			return clean
		}
		clean = parent
	}
}

func writeEngramStats(w io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("user home: %w", err)
	}

	defaultDir := engram.DefaultDir(home)
	dataDir := defaultDir
	source := "default (~/.engram)"

	var stateWarning error
	if s, err := state.Read(home); err == nil {
		if s.EngramDataDir != "" {
			dataDir = engram.DataDirRef(s.EngramDataDir).Resolve(home)
			source = "state.json (engram_data_dir)"
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		stateWarning = err
	}

	fmt.Fprintf(w, "Engram data directory\n")
	fmt.Fprintf(w, "  Path:            %s\n", dataDir)
	fmt.Fprintf(w, "  Source:          %s\n", source)
	fmt.Fprintf(w, "  Default if unset: %s\n", defaultDir)
	if stateWarning != nil {
		fmt.Fprintf(w, "  State warning:   %v\n", stateWarning)
	}

	spacePath := nearestExistingPath(dataDir)
	volFree, volErr := storage.AvailableBytes(spacePath)
	if volErr != nil {
		fmt.Fprintf(w, "  Volume free space: (unavailable: %v)\n", volErr)
	} else {
		fmt.Fprintf(w, "  Volume free space: %s\n", storage.FormatBytes(volFree))
	}

	fmt.Fprintf(w, "SQLite files (under data directory)\n")
	sum := int64(0)
	for _, p := range engram.SQLiteArtifactPaths(dataDir) {
		base := filepath.Base(p)
		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(w, "  %-18s  —\n", base)
				continue
			}
			fmt.Fprintf(w, "  %-18s  (error: %v)\n", base, err)
			continue
		}
		if info.IsDir() {
			fmt.Fprintf(w, "  %-18s  (unexpected directory)\n", base)
			continue
		}
		sz := info.Size()
		sum += sz
		fmt.Fprintf(w, "  %-18s  %s\n", base, storage.FormatBytes(sz))
	}
	fmt.Fprintf(w, "  Total (existing):   %s\n", storage.FormatBytes(sum))

	if sum > 0 {
		ok, needed, avail, err := engram.DiskSpaceOKForSQLiteArtifacts(dataDir, dataDir)
		if err != nil {
			fmt.Fprintf(w, "Duplicate-space check (same volume): unavailable (%v)\n", err)
		} else if ok {
			fmt.Fprintf(w, "Duplicate-space check (same volume): OK (needs %s; %s free)\n",
				storage.FormatBytes(needed), storage.FormatBytes(avail))
		} else {
			fmt.Fprintf(w, "Duplicate-space check (same volume): tight — needs %s to copy all artifacts; %s free\n",
				storage.FormatBytes(needed), storage.FormatBytes(avail))
		}
	}

	fmt.Fprintf(w, "Suggested locations (from configuration)\n")
	for _, loc := range engram.SuggestLocations(home, dataDir) {
		marker := ""
		if loc.IsCurrent {
			marker = " [current]"
		}
		fmt.Fprintf(w, "  %s%s\n", loc.Label, marker)
	}

	return nil
}
