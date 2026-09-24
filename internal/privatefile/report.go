package privatefile

import "errors"

const maxReportSize = 256 * 1024

var (
	ErrInvalidPayload     = errors.New("invalid private report payload")
	ErrInvalidDestination = errors.New("invalid private report destination")
	ErrUnsafeParent       = errors.New("unsafe private report parent")
	ErrDestinationExists  = errors.New("private report destination already exists")
	ErrWriteFailed        = errors.New("private report write failed")
	ErrUnsupported        = errors.New("private report storage is unsupported")
)
