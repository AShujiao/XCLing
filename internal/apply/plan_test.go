package apply

import (
	"strings"
	"testing"

	"XCLing/internal/model"
)

func testDraft(rules ...model.PathRule) model.WhitelistDraft {
	return model.WhitelistDraft{Policy: model.Policy{Name: "测试策略", DefaultLevel: model.LevelDisallowed, Rules: rules}}
}

func TestBuildPlanRequiresSelfAllow(t *testing.T) {
	draft := testDraft(model.PathRule{ID: "windows", Name: "Windows", Path: `C:\Windows\*`, Action: model.ActionAllow, Enabled: true})
	_, err := BuildPlan(draft, `D:\XCLing\XCLing.exe`)
	if err == nil || !strings.Contains(err.Error(), model.ApplyErrSelfNotAllowed) {
		t.Fatalf("expected self allow error, got %v", err)
	}
}

func TestBuildPlanDoesNotTreatBaseRuleAsExplicitSelfAllow(t *testing.T) {
	draft := testDraft(model.PathRule{ID: "base-windows", Name: "Windows", Path: `C:\Windows\*`, Action: model.ActionAllow, Enabled: true})
	_, err := BuildPlan(draft, `C:\Windows\XCLing.exe`)
	if err == nil || !strings.Contains(err.Error(), model.ApplyErrSelfNotAllowed) {
		t.Fatalf("expected explicit self allow error, got %v", err)
	}
}

func TestFingerprintIsOrderIndependent(t *testing.T) {
	a := model.PathRule{ID: "a", Name: "A", Path: `C:\A\*`, Action: model.ActionAllow, Enabled: true}
	b := model.PathRule{ID: "b", Name: "B", Path: `C:\B\app.exe`, Action: model.ActionAllow, Enabled: true}
	self := model.PathRule{ID: "self", Name: "XCLing", Path: `D:\XCLing\XCLing.exe`, Action: model.ActionAllow, Enabled: true}
	p1, err := BuildPlan(testDraft(a, b, self), self.Path)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := BuildPlan(testDraft(self, b, a), self.Path)
	if err != nil {
		t.Fatal(err)
	}
	if Fingerprint(p1) != Fingerprint(p2) {
		t.Fatal("fingerprint must ignore source order")
	}
}

func TestBuildPlanRejectsDuplicatePath(t *testing.T) {
	self := `D:\XCLing\XCLing.exe`
	draft := testDraft(
		model.PathRule{ID: "a", Path: self, Action: model.ActionAllow, Enabled: true},
		model.PathRule{ID: "b", Path: strings.ToLower(self), Action: model.ActionAllow, Enabled: true},
	)
	if _, err := BuildPlan(draft, self); err == nil {
		t.Fatal("expected duplicate rejection")
	}
}

func TestBuildPlanProtectsAdministratorsByDefault(t *testing.T) {
	self := `D:\XCLing\XCLing.exe`
	draft := testDraft(model.PathRule{ID: "self", Path: self, Action: model.ActionAllow, Enabled: true})
	plan, err := BuildPlan(draft, self)
	if err != nil {
		t.Fatal(err)
	}
	if plan.PolicyScope != PolicyScopeAllUsers {
		t.Fatalf("default policy must apply to administrators, got scope %d", plan.PolicyScope)
	}
}

func TestBuildPlanAllowsExplicitAdministratorBypass(t *testing.T) {
	self := `D:\XCLing\XCLing.exe`
	draft := testDraft(model.PathRule{ID: "self", Path: self, Action: model.ActionAllow, Enabled: true})
	draft.AdminBypass = true
	plan, err := BuildPlan(draft, self)
	if err != nil {
		t.Fatal(err)
	}
	if plan.PolicyScope != PolicyScopeExceptAdmins {
		t.Fatalf("explicit administrator bypass must use scope 1, got %d", plan.PolicyScope)
	}
}

func TestPartialMatchesOnlyAcceptsUnchangedExpectedSubset(t *testing.T) {
	expected := Plan{Rules: []Rule{{ID: "{a}", Path: `C:\A\*`, Description: "A", Level: 262144}, {ID: "{b}", Path: `C:\B\*`, Description: "B", Level: 262144}}}
	if !PartialMatches(Plan{Rules: expected.Rules[:1]}, expected) {
		t.Fatal("expected exact subset")
	}
	changed := Plan{Rules: []Rule{{ID: "{a}", Path: `C:\Changed\*`, Description: "A", Level: 262144}}}
	if PartialMatches(changed, expected) {
		t.Fatal("changed rule must be rejected")
	}
	unknown := Plan{Rules: []Rule{{ID: "{x}", Path: `C:\X\*`, Description: "X", Level: 262144}}}
	if PartialMatches(unknown, expected) {
		t.Fatal("unknown rule must be rejected")
	}
}

func TestProtectionStateRequiresExactManagedPlan(t *testing.T) {
	expected := Plan{
		DefaultLevel:       DefaultLevelDisallowed,
		PolicyScope:        PolicyScopeExceptAdmins,
		TransparentEnabled: TransparentEnabledEXE,
		Rules:              []Rule{{ID: "{a}", Path: `C:\Program Files\*`, Description: "Programs", Level: model.SrpLevelUnrestrictedRaw}},
	}
	if got := DetectProtectionState(expected, expected); got != model.ProtectionStateLocked {
		t.Fatalf("expected locked, got %q", got)
	}
	unlocked := expected
	unlocked.DefaultLevel = model.SrpLevelUnrestrictedRaw
	if got := DetectProtectionState(unlocked, expected); got != model.ProtectionStateUnlocked {
		t.Fatalf("expected unlocked, got %q", got)
	}
	drifted := unlocked
	drifted.Rules = []Rule{{ID: "{a}", Path: `C:\Changed\*`, Description: "Programs", Level: model.SrpLevelUnrestrictedRaw}}
	if got := DetectProtectionState(drifted, expected); got != model.ProtectionStateAttention {
		t.Fatalf("expected attention for drift, got %q", got)
	}
}

func TestAddAndRemoveTrustedRule(t *testing.T) {
	plan := Plan{Rules: []Rule{{ID: "{base}", Path: `C:\Windows\*`, Description: "Windows", Level: model.SrpLevelUnrestrictedRaw}}}
	updated, added, err := AddTrustedRule(plan, `D:\Tools`, true, "便携工具")
	if err != nil {
		t.Fatal(err)
	}
	if added.Path != `D:\Tools\*` || len(updated.Rules) != 2 {
		t.Fatalf("unexpected added rule: %+v plan=%+v", added, updated)
	}
	if _, _, err := AddTrustedRule(updated, `d:\tools\`, true, "重复"); err == nil {
		t.Fatal("duplicate trusted path must be rejected")
	}
	removed, err := RemoveTrustedRule(updated, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.Rules) != 1 || removed.Rules[0].ID != "{base}" {
		t.Fatalf("unexpected removal result: %+v", removed)
	}
}
