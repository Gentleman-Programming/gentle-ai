package reviewtransaction

// storeLockCoordinationBytes is the byte range the Windows lock primitive
// coordinates. POSIX flock remains whole-file advisory.
const storeLockCoordinationBytes int64 = 1
