package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestGentleAICompatWrapper_DeprecationNotice(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"--version"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() devolvió error inesperado: %v", err)
	}

	errStr := stderr.String()
	if !strings.Contains(errStr, deprecationNotice) {
		t.Fatalf("stderr no contiene el aviso de deprecación esperado:\n%s", errStr)
	}

	outStr := stdout.String()
	if !strings.Contains(outStr, "axiom") && !strings.Contains(outStr, "gentle-ai") {
		t.Fatalf("stdout no contiene la salida esperada del comando:\n%s", outStr)
	}
}

func TestGentleAICompatWrapper_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"--help"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() con --help devolvió error: %v", err)
	}

	if !strings.Contains(stderr.String(), deprecationNotice) {
		t.Fatalf("stderr no contiene el aviso de deprecación con --help")
	}

	if !strings.Contains(stdout.String(), "USAGE") && !strings.Contains(stdout.String(), "axiom") && !strings.Contains(stdout.String(), "gentle-ai") {
		t.Fatalf("stdout no contiene la ayuda esperada")
	}
}
