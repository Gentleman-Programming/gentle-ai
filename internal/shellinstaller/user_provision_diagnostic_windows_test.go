//go:build windows

package shellinstaller

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestUserWindowsProvisionDiagnosticWholeOutput(t *testing.T) {
	for _, tt := range []struct {
		name   string
		chunks [][]byte
		want   string
	}{
		{"empty", nil, `""`},
		{"complete chunks", [][]byte{[]byte("npm "), []byte("failed\n")}, `"npm failed\n"`},
		{"escaped controls", [][]byte{[]byte("\x1b[31merror\r\n")}, `"\x1b[31merror\r\n"`},
		{"bound", [][]byte{bytes.Repeat([]byte("x"), 16<<10)}, fmt.Sprintf("%q", bytes.Repeat([]byte("x"), 16<<10))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := &userWindowsProvisionDiagnostic{}
			for _, chunk := range tt.chunks {
				if n, err := d.Write(chunk); err != nil || n != len(chunk) {
					t.Fatalf("pipe was not drained: %d %v", n, err)
				}
			}
			if got := d.String(); got != tt.want {
				t.Fatalf("whole diagnostic differs: %q", got)
			}
		})
	}
}

func TestUserWindowsProvisionDiagnosticWithholdsUnsafeWhole(t *testing.T) {
	for _, tt := range []struct {
		name   string
		chunks [][]byte
	}{
		{"one oversized write", [][]byte{bytes.Repeat([]byte("x"), (16<<10)+1)}},
		{"overflow after prefix", [][]byte{bytes.Repeat([]byte("x"), 16<<10), []byte("y"), []byte("plausible tail")}},
		{"invalid UTF8", [][]byte{{0xff}}},
		{"NUL", [][]byte{[]byte("error\x00tail")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := &userWindowsProvisionDiagnostic{}
			for _, chunk := range tt.chunks {
				if n, err := d.Write(chunk); err != nil || n != len(chunk) {
					t.Fatal("withheld output blocked pipe drainage")
				}
			}
			if got := d.String(); !strings.HasPrefix(got, "whole diagnostic withheld") || strings.Contains(got, "plausible") {
				t.Fatalf("unsafe prefix/tail admitted: %q", got)
			}
			if d.withheld && len(d.data) != 0 {
				t.Fatal("overflow retained a diagnostic prefix")
			}
		})
	}
}
