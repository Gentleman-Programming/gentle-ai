//go:build windows

package sddstatus

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

type changeInstanceMarkerReadObserver struct {
	beforeRead  func()
	beforeRetry func(time.Duration)
}

func readChangeInstanceMarkerFile(path string) ([]byte, error) {
	return readChangeInstanceMarkerFileWithObserver(path, changeInstanceMarkerReadObserver{})
}

func readChangeInstanceMarkerFileWithObserver(path string, observer changeInstanceMarkerReadObserver) ([]byte, error) {
	for _, delay := range [...]time.Duration{5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond, 80 * time.Millisecond} {
		if observer.beforeRead != nil {
			observer.beforeRead()
		}
		payload, err := os.ReadFile(path)
		if err == nil || !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return payload, err
		}
		if observer.beforeRetry != nil {
			observer.beforeRetry(delay)
		}
		time.Sleep(delay)
	}
	if observer.beforeRead != nil {
		observer.beforeRead()
	}
	return os.ReadFile(path)
}
