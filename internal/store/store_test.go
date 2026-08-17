package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONFilesRoundTripAndRejectsTraversal(t *testing.T) {
	files := NewJSONFiles(t.TempDir(), ".json")
	value := map[string]string{"name": "first"}
	if err := files.Save("item-1", value); err != nil {
		t.Fatal(err)
	}
	var loaded map[string]string
	if err := files.Load("item-1", &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded["name"] != "first" {
		t.Fatalf("unexpected value: %#v", loaded)
	}
	if err := files.Save("../escape", value); err == nil {
		t.Fatal("path traversal must be rejected")
	}
	if _, err := os.Stat(filepath.Join(files.Dir(), "item-1.json.tmp")); !os.IsNotExist(err) {
		t.Fatal("temporary file must not remain after save")
	}
}

func TestJSONFilesListAndDelete(t *testing.T) {
	files := NewJSONFiles(t.TempDir(), ".record.json")
	if err := files.Save("b", map[string]int{"n": 2}); err != nil {
		t.Fatal(err)
	}
	if err := files.Save("a", map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	ids, err := files.IDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("unexpected ids: %v", ids)
	}
	if err := files.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if files.Exists("a") {
		t.Fatal("deleted record still exists")
	}
}
