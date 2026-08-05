package reviewtransaction

// secureOpenLocalStoreLockBeforeOpen gives tests a deterministic substitution
// point immediately before the platform's atomic no-follow open.
var secureOpenLocalStoreLockBeforeOpen func(string)

func runSecureOpenLocalStoreLockBeforeOpen(path string) {
	if hook := secureOpenLocalStoreLockBeforeOpen; hook != nil {
		hook(path)
	}
}
