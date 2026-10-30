package main

// The canonical path-free projection of recorded strings (issue #1866).
//
// A results file must not carry machine-specific paths: two runs of the same
// journey against the same binary, in different sandboxes, must record the
// same strings. These tests pin the closed token vocabulary and its
// longest-path-first, whole-path replacement.

import (
	"reflect"
	"testing"
)

func TestPathNormalizerAppliesLongestSpecificTokensFirst(t *testing.T) {
	normalizer := newPathNormalizer("/opt/binaries/gentle-ai", "/tmp/prov-root")
	normalizer.setSandbox("/tmp/prov-root/home", "/tmp/prov-root/home/demo")
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{"repository beats home and run root", "/tmp/prov-root/home/demo/.git/config", RepositoryToken + "/.git/config"},
		{"home beats run root", "/tmp/prov-root/home/.config/x", HomeToken + "/.config/x"},
		{"run root catches the rest", "/tmp/prov-root/tmp/scratch", RunRootToken + "/tmp/scratch"},
		{"target binary exact path", "/opt/binaries/gentle-ai", TargetBinaryToken},
		{"target binary inside an argv string", "exec /opt/binaries/gentle-ai --json", "exec " + TargetBinaryToken + " --json"},
		{"untouched string", "review start --json", "review start --json"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizer.Normalize(tt.in); got != tt.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
	var nilNormalizer *PathNormalizer
	if got := nilNormalizer.Normalize("/tmp/x"); got != "/tmp/x" {
		t.Fatalf("nil normalizer changed %q into %q", "/tmp/x", got)
	}
	if got := nilNormalizer.NormalizeAll([]string{"/tmp/x"}); !reflect.DeepEqual(got, []string{"/tmp/x"}) {
		t.Fatalf("nil normalizer changed args: %q", got)
	}
	if got := normalizer.NormalizeAll(nil); got != nil {
		t.Fatalf("NormalizeAll(nil) = %#v, want nil", got)
	}
	if got := normalizer.NormalizeAll([]string{"/tmp/prov-root/home/demo", "plain"}); !reflect.DeepEqual(got, []string{RepositoryToken, "plain"}) {
		t.Fatalf("NormalizeAll = %#v", got)
	}
}

func TestPathNormalizerRequiresPathBoundary(t *testing.T) {
	normalizer := newPathNormalizer("/opt/bin/gentle-ai", "/tmp/run")
	normalizer.setSandbox("/srv/home", "/srv/repo")
	for _, tt := range []struct{ in, want string }{
		{"/tmp/run2 /tmp/runner", "/tmp/run2 /tmp/runner"},
		{"/srv/homework /srv/repository /opt/bin/gentle-ai2", "/srv/homework /srv/repository /opt/bin/gentle-ai2"},
		{"/tmp/run", RunRootToken},
		{"/tmp/run/child", RunRootToken + "/child"},
		{`/tmp/run\child`, RunRootToken + `\child`},
		{"/tmp/run next", RunRootToken + " next"},
		{"/tmp/run\tnext", RunRootToken + "\tnext"},
		{"/tmp/run\nnext", RunRootToken + "\nnext"},
		{"/tmp/run\u00a0next", RunRootToken + "\u00a0next"},
		{"/tmp/run.log /tmp/run-2 /tmp/run_old", "/tmp/run.log /tmp/run-2 /tmp/run_old"},
		{"chdir /srv/repo: no such file or directory", "chdir " + RepositoryToken + ": no such file or directory"},
		{`fixture "/tmp/run" missing`, `fixture "` + RunRootToken + `" missing`},
		{"root is /tmp/run.", "root is " + RunRootToken + "."},
		{"/var/tmp/run /var/tmp/run/child", "/var/tmp/run /var/tmp/run/child"},
		{"./tmp/run x/tmp/run", "./tmp/run x/tmp/run"},
		{"--dir=/tmp/run", "--dir=" + RunRootToken},
		{"/var/tmp/run /tmp/run", "/var/tmp/run " + RunRootToken},
		{"/tmp/run2 /tmp/run/child /tmp/runner /tmp/run", "/tmp/run2 " + RunRootToken + "/child /tmp/runner " + RunRootToken},
	} {
		t.Run(tt.in, func(t *testing.T) {
			if got := normalizer.Normalize(tt.in); got != tt.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestPathNormalizerProjectsTargetBinaryUnderHome pins the CI layout: the
// target binary is built under $RUNNER_TEMP, which lives inside the runner's
// home, so the exact binary path must win over the home that contains it.
func TestPathNormalizerProjectsTargetBinaryUnderHome(t *testing.T) {
	normalizer := newPathNormalizer("/home/runner/work/_temp/gentle-ai", "")
	normalizer.setSandbox("/home/runner", "")
	for _, tt := range []struct{ in, want string }{
		{"/home/runner/work/_temp/gentle-ai", TargetBinaryToken},
		{"exec /home/runner/work/_temp/gentle-ai --version", "exec " + TargetBinaryToken + " --version"},
		{"/home/runner/work/_temp/other", HomeToken + "/work/_temp/other"},
	} {
		if got := normalizer.Normalize(tt.in); got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
