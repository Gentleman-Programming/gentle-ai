//go:build !unix

package opencode

import "errors"

func mkfifo(string) error { return errors.New("FIFOs are not supported") }
