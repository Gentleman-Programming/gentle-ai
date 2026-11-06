package shellinstaller

// PrivateRuntimeError is declared on all platforms so callers can classify
// Linux private-runtime failures without excluding the unsupported-OS route.
type PrivateRuntimeError struct {
	Kind, Workspace, Destination string
	Cause                        error
}

func (e *PrivateRuntimeError) Error() string {
	message := "private installation: " + e.Kind
	if e.Cause != nil {
		message += ": " + e.Cause.Error()
	}
	return message
}
func (e *PrivateRuntimeError) Unwrap() error { return e.Cause }
