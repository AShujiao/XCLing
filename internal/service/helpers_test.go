package service

import (
	"regexp"
	"testing"
)

func TestNewUUID(t *testing.T) {
	first, second := newUUID(), newUUID()
	if first == second {
		t.Fatal("UUIDs must be unique")
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(first) {
		t.Fatalf("invalid UUID: %q", first)
	}
}
