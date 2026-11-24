package cli

import (
	"errors"
	"fmt"
	"os"
)

// publishedGuidanceStep runs a publisher whose output file names depend on
// the guidance the prompt writers staged, which does not exist when the run's
// backup is taken. It snapshots exactly what the publisher may touch right
// before it runs and restores that on rollback: files this run created are
// removed, previous ones come back byte for byte, and an emptied directory
// this run created is removed.
type publishedGuidanceStep struct {
	id           string
	dir          string
	paths        func() []string
	publish      func() ([]string, error)
	changedFiles *[]string
	before       map[string]syncFileSnapshot
	createdDir   bool
}

func (s *publishedGuidanceStep) ID() string { return s.id }

func (s *publishedGuidanceStep) Run() (runErr error) {
	if _, err := os.Lstat(s.dir); os.IsNotExist(err) {
		s.createdDir = true
	}
	before, err := snapshotSyncFiles(s.paths())
	if err != nil {
		return fmt.Errorf("snapshot %s: %w", s.id, err)
	}
	s.before = before
	defer func() {
		if runErr != nil {
			runErr = errors.Join(runErr, s.Rollback())
		}
	}()

	files, err := s.publish()
	if err != nil {
		return err
	}
	if s.changedFiles != nil {
		*s.changedFiles = append(*s.changedFiles, files...)
	}
	return nil
}

func (s *publishedGuidanceStep) Rollback() error {
	err := restoreSyncFiles(s.before)
	if s.createdDir {
		// Only an emptied directory this run created is removed.
		_ = os.Remove(s.dir)
	}
	return err
}