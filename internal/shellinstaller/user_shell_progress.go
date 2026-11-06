package shellinstaller

// UserInstallProgress reports one ordered installation step to an in-process
// owner. It carries no installation authority and never substitutes for the
// inspected confirmation.
type UserInstallProgress struct {
	Index, Total int
	Label, State string
	Detail       string
}
