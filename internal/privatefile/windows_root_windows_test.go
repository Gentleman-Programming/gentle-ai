//go:build windows

package privatefile

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseWindowsPrivateDestination(t *testing.T) {
	for _, tt := range []struct {
		name, destination, root, leaf string
		parents                       []string
	}{
		{"drive path", `C:\private\reports\report.json`, `C:\`, "report.json", []string{"private", "reports"}},
		{"preserve spelling", `d:\My Folder\a.b\Report`, `d:\`, "Report", []string{"My Folder", "a.b"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, parents, leaf, err := parseWindowsPrivateDestination(tt.destination)
			if err != nil || root != tt.root || leaf != tt.leaf || !reflect.DeepEqual(parents, tt.parents) {
				t.Fatalf("parse = (%q, %q, %q, %v), want (%q, %q, %q, nil)", root, parents, leaf, err, tt.root, tt.parents, tt.leaf)
			}
		})
	}
}

func TestParseWindowsPrivateDestinationRefusesAmbiguousPaths(t *testing.T) {
	for _, tt := range []struct{ name, destination string }{
		{"empty", ""},
		{"relative", `private\report`},
		{"root relative", `\private\report`},
		{"drive relative", `C:private\report`},
		{"bare root", `C:\`},
		{"root leaf", `C:\report.json`},
		{"UNC", `\\server\share\private\report`},
		{"extended drive", `\\?\C:\private\report`},
		{"extended UNC", `\\?\UNC\server\share\report`},
		{"device", `\\.\C:\private\report`},
		{"NT namespace", `\??\C:\private\report`},
		{"NT device", `\Device\HarddiskVolume1\private\report`},
		{"volume GUID", `\\?\Volume{01234567-89ab-cdef-0123-456789abcdef}\private\report`},
		{"slash prefix", `C:/private/report`},
		{"mixed separators", `C:\private/report`},
		{"missing component", `C:\private\\report`},
		{"trailing separator", `C:\private\report\`},
		{"dot", `C:\private\.\report`},
		{"dotdot", `C:\private\..\report`},
		{"stream", `C:\private\report:secret`},
		{"parent stream", `C:\private:secret\report`},
		{"trailing dot", `C:\private\report.`},
		{"trailing space", `C:\private \report`},
		{"reserved leaf", `C:\private\NUL.txt`},
		{"reserved parent", `C:\COM1\report`},
		{"superscript port", "C:\\private\\COM¹.txt"},
		{"NUL byte", "C:\\private\\bad\x00name"},
		{"invalid UTF-8", "C:\\private\\bad\xffname"},
		{"drive alias", `1:\private\report`},
		{"wildcard", `C:\private\report?`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, parents, leaf, err := parseWindowsPrivateDestination(tt.destination)
			if !errors.Is(err, ErrInvalidDestination) || root != "" || parents != nil || leaf != "" {
				t.Fatalf("parse = (%q, %q, %q, %v), want empty result and ErrInvalidDestination", root, parents, leaf, err)
			}
		})
	}
}
