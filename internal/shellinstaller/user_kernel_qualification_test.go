package shellinstaller

import (
	"strings"
	"testing"
)

// Table test for the pure kernel-qualification predicate. It must not skip on
// the host: the predicate is pure, so every build gate is exercised on every
// platform and a silent floor regression fails the suite anywhere.
func TestWindowsKernelQualification(t *testing.T) {
	tests := []struct {
		name         string
		goarch       string
		native       uint16
		major        uint32
		build        uint32
		productType  uint8
		wantAccepted bool
		wantFragment string
	}{
		{name: "windows 10 21h2 build 19044 rejected", goarch: "amd64", native: 0x8664, major: 10, build: 19044, productType: 1, wantFragment: "Windows build 19044 is below the qualified floor 19045"},
		{name: "windows 10 22h2 build 19045 accepted", goarch: "amd64", native: 0x8664, major: 10, build: 19045, productType: 1, wantAccepted: true},
		{name: "windows 11 build 22000 accepted", goarch: "amd64", native: 0x8664, major: 10, build: 22000, productType: 1, wantAccepted: true},
		{name: "windows 11 build 26200 accepted", goarch: "amd64", native: 0x8664, major: 10, build: 26200, productType: 1, wantAccepted: true},
		{name: "32-bit host rejected", goarch: "amd64", native: 0x014c, major: 10, build: 26200, productType: 1, wantFragment: "the Windows host is not x64; ARM64 is not a qualified target"},
		{name: "arm64 host rejected", goarch: "amd64", native: 0xaa64, major: 10, build: 26200, productType: 1, wantFragment: "the Windows host is not x64; ARM64 is not a qualified target"},
		{name: "non-amd64 executable rejected", goarch: "arm64", native: 0x8664, major: 10, build: 26200, productType: 1, wantFragment: "this gentle-ai executable is arm64, not amd64"},
		{name: "non-windows major version rejected", goarch: "amd64", native: 0x8664, major: 11, build: 26200, productType: 1, wantFragment: "the Windows host reports major version 11"},
		{name: "windows server rejected", goarch: "amd64", native: 0x8664, major: 10, build: 26200, productType: 3, wantFragment: "Windows Server is not a qualified target"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := windowsKernelQualificationError(test.goarch, test.native, test.major, test.build, test.productType)
			if test.wantAccepted && err != nil {
				t.Fatalf("expected acceptance, got %v", err)
			}
			if !test.wantAccepted {
				if err == nil {
					t.Fatalf("expected refusal, got nil")
				}
				if !strings.Contains(err.Error(), test.wantFragment) {
					t.Fatalf("refusal must name the failed condition %q, got %q", test.wantFragment, err.Error())
				}
			}
		})
	}
}
