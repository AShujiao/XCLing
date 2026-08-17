package store

import (
	"path/filepath"
	"testing"

	"XCLing/internal/model"
)

func TestRecoveryStoreRoundTrip(t *testing.T) {
	st := NewRecoveryStoreAt(filepath.Join(t.TempDir(), "recovery"))
	want := model.RecoveryRecord{SchemaVersion: "1", ID: "r1", PolicyName: "测试", State: model.RecoveryStatePrepared, BeforeState: model.BeforeStateInert, BeforeDefaultLevel: model.SrpLevelUnrestrictedRaw}
	if err := st.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.PolicyName != want.PolicyName || got.BeforeState != want.BeforeState || got.BeforeDefaultLevel != want.BeforeDefaultLevel {
		t.Fatalf("got %#v", got)
	}
}

func TestRecoveryStoreRejectsUnknownSchema(t *testing.T) {
	st := NewRecoveryStoreAt(filepath.Join(t.TempDir(), "recovery"))
	if err := st.Save(model.RecoveryRecord{SchemaVersion: "3", ID: "future"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Load(); err == nil {
		t.Fatal("expected schema rejection")
	}
}

func TestRecoveryStoreRoundTripsManagedSRPSnapshot(t *testing.T) {
	st := NewRecoveryStoreAt(filepath.Join(t.TempDir(), "recovery"))
	want := model.RecoveryRecord{
		SchemaVersion: "2",
		ID:            "managed-r1",
		PolicyName:    "XCLing",
		State:         model.RecoveryStateApplied,
		BeforeState:   model.BeforeStateManaged,
		BeforeSnapshot: model.RegistryTreeSnapshot{
			Exists: true,
			Root: model.RegistryKeySnapshot{
				Values: []model.RegistryValueSnapshot{{Name: "DefaultLevel", Type: 4, Data: []byte{0, 0, 4, 0}}},
			},
		},
	}
	if err := st.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !want.BeforeSnapshot.Equal(got.BeforeSnapshot) {
		t.Fatalf("snapshot changed: %#v", got.BeforeSnapshot)
	}
}

func TestRecoveryStoreRejectsInvalidInertBeforeState(t *testing.T) {
	st := NewRecoveryStoreAt(filepath.Join(t.TempDir(), "recovery"))
	record := model.RecoveryRecord{SchemaVersion: "1", ID: "bad-before", BeforeState: model.BeforeStateInert, BeforeDefaultLevel: 0}
	if err := st.Save(record); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Load(); err == nil {
		t.Fatal("expected invalid inert snapshot rejection")
	}
}
