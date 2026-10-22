package app

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestShellInstallEarlyDispatch(t *testing.T) {
	oldOS, oldDetect := ensureCurrentOSSupported, detectSystem
	t.Cleanup(func() { ensureCurrentOSSupported, detectSystem = oldOS, oldDetect })
	ensureCurrentOSSupported = func() error {
		t.Fatal("shell installer reached generic OS gate")
		return errors.New("unexpected OS detection")
	}
	detectSystem = func(context.Context) (system.DetectionResult, error) {
		t.Fatal("shell installer reached generic system detection")
		return system.DetectionResult{}, errors.New("unexpected detection")
	}
	var output bytes.Buffer
	if err := RunArgs([]string{"shell", "install", "--help"}, &output); err != nil {
		t.Fatal(err)
	}
	if output.Len() == 0 {
		t.Fatal("dedicated installer help missing")
	}
}
