package main

import "testing"

// Retiring the OpenCode SDD transport axis must not remove the generic axis seam.
func TestRetiredSDDTaskResultAxisIsNotRegistered(t *testing.T) {
	for _, axis := range Axes() {
		if axis.Name == "sdd-task-result" {
			t.Fatal("retired SDD task-result transport axis is still registered")
		}
	}
	if _, err := selectAxes("sdd-task-result"); err == nil {
		t.Fatal("retired SDD task-result axis should be rejected as unknown")
	}
}
