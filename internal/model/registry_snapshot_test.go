package model

import "testing"

func TestRegistryTreeSnapshotEqualIgnoresEnumerationOrder(t *testing.T) {
	left := RegistryTreeSnapshot{
		Exists: true,
		Root: RegistryKeySnapshot{
			Values: []RegistryValueSnapshot{
				{Name: "PolicyScope", Type: 4, Data: []byte{1, 0, 0, 0}},
				{Name: "DefaultLevel", Type: 4, Data: []byte{0, 0, 0, 0}},
			},
			Children: []RegistryNamedKeySnapshot{
				{Name: "262144", Key: RegistryKeySnapshot{}},
				{Name: "0", Key: RegistryKeySnapshot{}},
			},
		},
	}
	right := RegistryTreeSnapshot{
		Exists: true,
		Root: RegistryKeySnapshot{
			Values: []RegistryValueSnapshot{
				{Name: "defaultlevel", Type: 4, Data: []byte{0, 0, 0, 0}},
				{Name: "policyscope", Type: 4, Data: []byte{1, 0, 0, 0}},
			},
			Children: []RegistryNamedKeySnapshot{
				{Name: "0", Key: RegistryKeySnapshot{}},
				{Name: "262144", Key: RegistryKeySnapshot{}},
			},
		},
	}

	if !left.Equal(right) {
		t.Fatal("equivalent registry trees should compare equal")
	}

	right.Root.Values[0].Data = []byte{1, 0, 0, 0}
	if left.Equal(right) {
		t.Fatal("changed registry value data must be detected")
	}
}
