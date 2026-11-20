//go:build linux

package shellinstaller

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// writeInterpreterELF writes a minimal ELF64 executable whose only program
// header is PT_INTERP naming interpreter, the field the libc probe reads.
func writeInterpreterELF(t *testing.T, path, interpreter string) {
	t.Helper()
	type header struct {
		Ident                      [16]byte
		Type, Machine              uint16
		Version                    uint32
		Entry, Phoff, Shoff        uint64
		Flags                      uint32
		Ehsize, Phentsize, Phnum   uint16
		Shentsize, Shnum, Shstrndx uint16
	}
	type program struct {
		Type, Flags                                uint32
		Offset, Vaddr, Paddr, Filesz, Memsz, Align uint64
	}
	payload := append([]byte(interpreter), 0)
	var ident [16]byte
	copy(ident[:], []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	var buffer bytes.Buffer
	for _, value := range []any{
		header{Ident: ident, Type: 2, Machine: 62, Version: 1, Phoff: 64, Ehsize: 64, Phentsize: 56, Phnum: 1, Shentsize: 64},
		program{Type: 3, Flags: 4, Offset: 120, Filesz: uint64(len(payload)), Memsz: uint64(len(payload)), Align: 1},
	} {
		if err := binary.Write(&buffer, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	buffer.Write(payload)
	if err := os.WriteFile(path, buffer.Bytes(), 0700); err != nil {
		t.Fatal(err)
	}
}

func TestUserHostPlatformRefusesUnsupportedHostsByName(t *testing.T) {
	dir := t.TempDir()
	glibc := filepath.Join(dir, "glibc-sh")
	musl := filepath.Join(dir, "musl-sh")
	writeInterpreterELF(t, glibc, "/lib64/ld-linux-x86-64.so.2")
	writeInterpreterELF(t, musl, "/lib/ld-musl-x86_64.so.1")

	if err := userHostPlatform("amd64", glibc); err != nil {
		t.Fatalf("Linux amd64 with glibc refused: %v", err)
	}
	for _, tc := range []struct{ arch, shell, host string }{
		{"arm64", glibc, "Linux arm64"},
		{"amd64", musl, "Linux amd64/musl"},
		{"arm64", musl, "Linux arm64/musl"},
	} {
		err := userHostPlatform(tc.arch, tc.shell)
		var private *PrivateRuntimeError
		if err == nil || !errors.As(err, &private) || private.Kind != "unavailable" {
			t.Fatalf("%s admitted or untyped: %v", tc.host, err)
		}
		if want := "unsupported host: " + tc.host + "; supported: Linux amd64 with glibc"; !strings.Contains(err.Error(), want) {
			t.Fatalf("%s refusal does not name the condition: %v", tc.host, err)
		}
	}
	// An unreadable or interpreter-less shell is no evidence of musl: the
	// diagnosis never refuses a host it cannot classify.
	static := filepath.Join(dir, "static")
	if err := os.WriteFile(static, []byte("#!/bin/true\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, shell := range []string{static, filepath.Join(dir, "missing")} {
		if err := userHostPlatform("amd64", shell); err != nil {
			t.Fatalf("unclassified shell %s refused: %v", shell, err)
		}
	}
}

func TestUserHostLibcReadsTheRealShellInterpreter(t *testing.T) {
	if _, err := os.Stat("/etc/alpine-release"); err == nil {
		t.Skip("musl test host")
	}
	if got := userHostLibc("/bin/sh"); got != "glibc" {
		t.Fatalf("glibc test host classified as %q", got)
	}
}

func TestUserExecutableMountRefusesNoexec(t *testing.T) {
	parent := t.TempDir()
	noexec := func(path string, st *unix.Statfs_t) error {
		if path != parent {
			t.Fatalf("statfs probed %q, want the target parent %q", path, parent)
		}
		st.Flags = unix.ST_NOEXEC | unix.ST_NOSUID
		return nil
	}
	err := userExecutableMount(parent, noexec)
	var private *PrivateRuntimeError
	if err == nil || !errors.As(err, &private) || private.Kind != "refused" {
		t.Fatalf("noexec parent admitted or untyped: %v", err)
	}
	for _, want := range []string{parent, "noexec mount", "remount"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("noexec refusal lacks %q: %v", want, err)
		}
	}
	if err := userExecutableMount(parent, func(string, *unix.Statfs_t) error { return unix.EACCES }); err == nil || !strings.Contains(err.Error(), parent) {
		t.Fatalf("statfs failure admitted or unnamed: %v", err)
	}
	if err := userExecutableMount(parent, func(_ string, st *unix.Statfs_t) error { st.Flags = unix.ST_NOSUID; return nil }); err != nil {
		t.Fatalf("exec-capable mount refused: %v", err)
	}
	if err := userExecutableMount(parent, unix.Statfs); err != nil {
		t.Fatalf("real test temp mount refused: %v", err)
	}
}

// The preflight runs inside ValidateUserInstall, which the TUI and the
// install flow call through InspectUserInstall before any download; an
// unchanged valid selection still validates (existing behavior).
func TestValidateUserInstallKeepsAcceptingSupportedHost(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(parent)
	if err != nil {
		t.Fatal(err)
	}
	if userHostLibc("/bin/sh") == "musl" {
		t.Skip("test host is musl; refusal is covered by TestUserHostPlatformRefusesUnsupportedHostsByName")
	}
	if err := ValidateUserInstall(UserInstallRequest{Mode: "separate", Destination: filepath.Join(canonical, "shell")}); err != nil {
		t.Fatalf("supported host and exec-capable parent refused: %v", err)
	}
}
