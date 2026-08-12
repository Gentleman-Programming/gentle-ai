//go:build windows

package reviewtransaction

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

type reviewRepositoryContextOpenObserver struct {
	beforeOpen  func()
	beforeRetry func(time.Duration)
}

func openReviewRepositoryContext(path string) (*os.File, error) {
	return openReviewRepositoryContextWithObserver(path, reviewRepositoryContextOpenObserver{})
}

func openReviewRepositoryContextWithObserver(path string, observer reviewRepositoryContextOpenObserver) (*os.File, error) {
	for _, delay := range [...]time.Duration{5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond, 80 * time.Millisecond} {
		if observer.beforeOpen != nil {
			observer.beforeOpen()
		}
		file, err := os.Open(path)
		if err == nil || !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return file, err
		}
		if observer.beforeRetry != nil {
			observer.beforeRetry(delay)
		}
		time.Sleep(delay)
	}
	if observer.beforeOpen != nil {
		observer.beforeOpen()
	}
	return os.Open(path)
}
