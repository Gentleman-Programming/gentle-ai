package reviewtransaction

import (
	"os"
	"time"
)

// sharingViolationBackoffs is the bounded retry schedule for the transient
// Windows sharing-violation recovery on the final-file open of an immutable
// repository-context record. Five backoffs: 5/10/20/40/80 ms — 155 ms total
// sleep. The first attempt is free; the schedule is consumed only on
// ERROR_SHARING_VIOLATION. Every other open error fails immediately so
// non-sharing failures are not masked by a silent retry.
var sharingViolationBackoffs = []time.Duration{
	5 * time.Millisecond,
	10 * time.Millisecond,
	20 * time.Millisecond,
	40 * time.Millisecond,
	80 * time.Millisecond,
}

// openReviewRepositoryContextFile opens the immutable repository-context
// record at path, delegating to the platform-specific openReviewRepositoryContextFile
// implementation. On Windows the open tolerates a brief ERROR_SHARING_VIOLATION
// window that can surface after concurrent publication (the kernel may still
// hold the destination open for a short sharing-violation window after
// MoveFileEx). On non-Windows platforms this is a direct passthrough to
// os.Open; the transient sharing-violation retry applies only to Windows.
//
// Retry scope is limited to the final-file open. The Lstat, mode, file.Stat,
// io.ReadAll, and decode checks that follow retain their existing
// immediate-failure semantics — sharing-violation retry does not extend
// to read or decode failures.
func openReviewRepositoryContextFile(path string) (*os.File, error) {
	return openReviewRepositoryContextFileWith(path, os.Open, sharingViolationBackoffs, time.Sleep, isTransientSharingViolation)
}

// openReviewRepositoryContextFileWith is the testable form: the production
// openReviewRepositoryContextFile delegates to it with the real os.Open and
// time.Sleep; the tests inject a fake opener, a no-op sleeper, and a
// configurable isTransient predicate. Separating the retry loop from the
// platform-specific isTransient check lets the test drive the full retry
// schedule on any platform without needing to manufacture a real
// ERROR_SHARING_VIOLATION.
func openReviewRepositoryContextFileWith(
	path string,
	open func(string) (*os.File, error),
	backoffs []time.Duration,
	sleep func(time.Duration),
	isTransient func(error) bool,
) (*os.File, error) {
	file, err := open(path)
	if err == nil {
		return file, nil
	}
	if !isTransient(err) {
		return nil, err
	}
	for _, backoff := range backoffs {
		sleep(backoff)
		file, err = open(path)
		if err == nil {
			return file, nil
		}
		if !isTransient(err) {
			return nil, err
		}
	}
	return nil, err
}
