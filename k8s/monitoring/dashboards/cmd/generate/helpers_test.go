package main

import "testing"

func TestRefIDs(t *testing.T) {
	for i, want := range map[int]string{0: "A", 1: "B", 25: "Z", 26: "AA", 27: "AB", 52: "BA"} {
		if got := refID(i); got != want {
			t.Errorf("refID(%d) = %q, want %q", i, got, want)
		}
	}
}
