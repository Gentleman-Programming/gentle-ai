package main

import "testing"

func TestDamagedStoreAxisRemainsUnregistered(t *testing.T) {
	for _, axis := range Axes() {
		if axis.Name == "damaged-store" {
			t.Fatal("retired damaged-store axis is registered")
		}
	}
}
