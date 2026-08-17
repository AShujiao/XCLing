//go:build windows

package platform

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestRegistryTreeRoundTripPreservesValuesAndChildren(t *testing.T) {
	path := fmt.Sprintf(`Software\XCLingTests\RegistryTree-%d`, os.Getpid())
	cleanupRegistryTree(t, path)
	t.Cleanup(func() { cleanupRegistryTree(t, path) })

	root, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.SetDWordValue("DefaultLevel", 0); err != nil {
		t.Fatal(err)
	}
	if err := root.SetStringValue("ExecutableTypes", "EXE"); err != nil {
		t.Fatal(err)
	}
	if err := root.SetBinaryValue("Opaque", []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	child, _, err := registry.CreateKey(registry.CURRENT_USER, path+`\262144\Paths\{rule}`, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	if err := child.SetStringValue("ItemData", `C:\Program Files\*`); err != nil {
		t.Fatal(err)
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}

	want, err := readRegistryTree(registry.CURRENT_USER, path)
	if err != nil {
		t.Fatal(err)
	}
	cleanupRegistryTree(t, path)
	if err := replaceRegistryTree(registry.CURRENT_USER, path, want); err != nil {
		t.Fatal(err)
	}
	got, err := readRegistryTree(registry.CURRENT_USER, path)
	if err != nil {
		t.Fatal(err)
	}
	if !want.Equal(got) {
		t.Fatalf("restored registry tree differs\nwant: %#v\n got: %#v", want, got)
	}
}

func cleanupRegistryTree(t *testing.T, path string) {
	t.Helper()
	if err := deleteRegistryTree(registry.CURRENT_USER, path); err != nil {
		t.Fatalf("cleanup registry fixture: %v", err)
	}
}
