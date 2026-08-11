//go:build !windows

package reviewtransaction

// isTransientSharingViolation reports whether err is a transient sharing
// violation. On non-Windows platforms no syscall returns the equivalent of
// ERROR_SHARING_VIOLATION from os.Open, so the predicate always reports
// false and the retry loop never engages. The retry machinery remains in
// the cross-platform file so the production open path and the test path use
// the same code; the only thing that varies between platforms is whether
// isTransientSharingViolation ever returns true.
func isTransientSharingViolation(err error) bool {
	return false
}
