package main

import "testing"

func TestRealWorldAxisRemainsUnregistered(t *testing.T) {
	for _, axis := range Axes() {
		if axis.Name == "real-world" {
			t.Fatal("retired real-world axis is registered")
		}
	}
}
