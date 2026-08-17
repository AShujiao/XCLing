package store

import (
	"path/filepath"
	"testing"

	"XCLing/internal/model"
)

func TestProtectionEventStoreListsNewestFirst(t *testing.T) {
	store := NewProtectionEventStoreAt(filepath.Join(t.TempDir(), "events.json"))
	if err := store.Append(model.ProtectionEvent{ID: "1", Action: "enable"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(model.ProtectionEvent{ID: "2", Action: "unlock"}); err != nil {
		t.Fatal(err)
	}
	events, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].ID != "2" || events[1].ID != "1" {
		t.Fatalf("unexpected events: %#v", events)
	}
}
