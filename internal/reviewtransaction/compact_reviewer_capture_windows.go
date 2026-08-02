//go:build windows

package reviewtransaction

import (
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

func compactReviewerFileSingleLink(file *os.File, _ fs.FileInfo) bool {
	if file == nil {
		// The opened-handle check below is authoritative on Windows.
		return true
	}
	var information windows.ByHandleFileInformation
	return windows.GetFileInformationByHandle(
		windows.Handle(file.Fd()),
		&information,
	) == nil && information.NumberOfLinks == 1
}
