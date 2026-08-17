package platform

import (
	"strings"
	"testing"
)

func TestClassifySRPRootAllowsExactInertUnrestrictedState(t *testing.T) {
	snapshot := SRPRootSnapshot{
		Exists: true,
		Values: []SRPRootValue{{Name: "DefaultLevel", Kind: SRPValueDWORD, Integer: 262144}},
	}
	if got := ClassifySRPRoot(snapshot); got != SRPDispositionInertUnrestricted {
		t.Fatalf("ClassifySRPRoot()=%q, want %q", got, SRPDispositionInertUnrestricted)
	}
}

func TestSameSRPRootSnapshotDetectsDriftButIgnoresEnumerationOrder(t *testing.T) {
	left := SRPRootSnapshot{Exists: true, Values: []SRPRootValue{{Name: "DefaultLevel", Kind: SRPValueDWORD, Integer: 262144}}, Children: []string{"B", "A"}}
	right := SRPRootSnapshot{Exists: true, Values: []SRPRootValue{{Name: "defaultlevel", Kind: SRPValueDWORD, Integer: 262144}}, Children: []string{"a", "b"}}
	if !SameSRPRootSnapshot(left, right) {
		t.Fatal("registry enumeration order and name casing must not count as drift")
	}
	right.Values[0].Integer = 0
	if SameSRPRootSnapshot(left, right) {
		t.Fatal("value changes must count as drift")
	}
}

func TestClassifySRPRootRejectsEveryDeviationFromInertShape(t *testing.T) {
	cases := map[string]SRPRootSnapshot{
		"extra value": {Exists: true, Values: []SRPRootValue{{Name: "DefaultLevel", Kind: SRPValueDWORD, Integer: 262144}, {Name: "PolicyScope", Kind: SRPValueDWORD, Integer: 1}}},
		"wrong type":  {Exists: true, Values: []SRPRootValue{{Name: "DefaultLevel", Kind: "string", Integer: 262144}}},
		"wrong value": {Exists: true, Values: []SRPRootValue{{Name: "DefaultLevel", Kind: SRPValueDWORD, Integer: 0}}},
		"child key":   {Exists: true, Values: []SRPRootValue{{Name: "DefaultLevel", Kind: SRPValueDWORD, Integer: 262144}}, Children: []string{"262144"}},
		"empty root":  {Exists: true},
	}
	for name, snapshot := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ClassifySRPRoot(snapshot); got != SRPDispositionManaged {
				t.Fatalf("ClassifySRPRoot()=%q, want %q", got, SRPDispositionManaged)
			}
		})
	}
	if got := ClassifySRPRoot(SRPRootSnapshot{}); got != SRPDispositionAbsent {
		t.Fatalf("absent root classified as %q", got)
	}
}

func TestSafeTransitionRootRejectsUnknownChanges(t *testing.T) {
	valid := SRPRootSnapshot{Exists: true, Values: []SRPRootValue{
		{Name: "DefaultLevel", Kind: SRPValueDWORD, Integer: 0},
		{Name: "PolicyScope", Kind: SRPValueDWORD, Integer: 1},
	}, Children: []string{"262144"}, LevelChildren: []string{"Paths"}}
	if !SafeTransitionRoot(valid, SRPDispositionInertUnrestricted) {
		t.Fatal("expected known partial activation state to be safe")
	}
	withUnknown := valid
	withUnknown.Values = append(append([]SRPRootValue(nil), valid.Values...), SRPRootValue{Name: "Unknown", Kind: SRPValueDWORD, Integer: 1})
	if SafeTransitionRoot(withUnknown, SRPDispositionInertUnrestricted) {
		t.Fatal("unknown root value must make transition unsafe")
	}
	withUnknown = valid
	withUnknown.Children = []string{"262144", "unexpected"}
	if SafeTransitionRoot(withUnknown, SRPDispositionInertUnrestricted) {
		t.Fatal("unknown child key must make transition unsafe")
	}
	withUnknown = valid
	withUnknown.LevelChildren = []string{"Paths", "Hashes"}
	if SafeTransitionRoot(withUnknown, SRPDispositionInertUnrestricted) {
		t.Fatal("unknown security-level child must make transition unsafe")
	}
}

func TestAppliedRootMatchesRequiresExactActivationValues(t *testing.T) {
	root := SRPRootSnapshot{Exists: true, Values: []SRPRootValue{
		{Name: "DefaultLevel", Kind: SRPValueDWORD, Integer: 0},
		{Name: "PolicyScope", Kind: SRPValueDWORD, Integer: 1},
		{Name: "TransparentEnabled", Kind: SRPValueDWORD, Integer: 1},
		{Name: "ExecutableTypes", Kind: SRPValueMULTISZ, Strings: append([]string(nil), srpExecutableTypes...)},
	}, Children: []string{"262144"}, LevelChildren: []string{"Paths"}}
	if !AppliedRootMatches(root) {
		t.Fatal("expected exact applied root")
	}
	root.Values = append(root.Values, SRPRootValue{Name: "Unknown", Kind: SRPValueDWORD, Integer: 1})
	if AppliedRootMatches(root) {
		t.Fatal("extra values must reject applied root")
	}
}

func TestOwnedRootLegacyCompatibilityIsExplicit(t *testing.T) {
	legacy := SRPRootSnapshot{Exists: true, Values: []SRPRootValue{
		{Name: "DefaultLevel", Kind: SRPValueDWORD, Integer: 0},
		{Name: "PolicyScope", Kind: SRPValueDWORD, Integer: 1},
		{Name: "TransparentEnabled", Kind: SRPValueDWORD, Integer: 1},
	}, Children: []string{"262144"}, LevelChildren: []string{"Paths"}}
	if OwnedRootMatches(legacy, 0, 1, false) {
		t.Fatal("strict root matching must require ExecutableTypes")
	}
	if !OwnedRootMatchesLegacyCompatible(legacy, 0, 1, false) {
		t.Fatal("the exact pre-fix root must remain eligible for migration")
	}
	legacy.Values = append(legacy.Values, SRPRootValue{Name: "Unknown", Kind: SRPValueDWORD, Integer: 1})
	if OwnedRootMatchesLegacyCompatible(legacy, 0, 1, false) {
		t.Fatal("legacy migration must reject unknown root values")
	}
}

func TestSafeTransitionRootValidatesExecutableTypes(t *testing.T) {
	root := SRPRootSnapshot{Exists: true, Values: []SRPRootValue{
		{Name: "DefaultLevel", Kind: SRPValueDWORD, Integer: 0},
		{Name: "PolicyScope", Kind: SRPValueDWORD, Integer: 1},
		{Name: "TransparentEnabled", Kind: SRPValueDWORD, Integer: 1},
		{Name: "ExecutableTypes", Kind: SRPValueMULTISZ, Strings: append([]string(nil), srpExecutableTypes...)},
	}, Children: []string{"262144"}, LevelChildren: []string{"Paths"}}
	if !SafeTransitionRoot(root, SRPDispositionAbsent) {
		t.Fatal("expected exact executable types to be safe")
	}

	wrongType := root
	wrongType.Values = append([]SRPRootValue(nil), root.Values...)
	wrongType.Values[3].Kind = SRPValueDWORD
	if SafeTransitionRoot(wrongType, SRPDispositionAbsent) {
		t.Fatal("wrong ExecutableTypes registry type must be rejected")
	}

	wrongList := root
	wrongList.Values = append([]SRPRootValue(nil), root.Values...)
	wrongList.Values[3].Strings = append([]string(nil), srpExecutableTypes...)
	wrongList.Values[3].Strings[0] = "DLL"
	if SafeTransitionRoot(wrongList, SRPDispositionAbsent) {
		t.Fatal("unexpected executable type must be rejected")
	}
}

// 拦截规则（blocklist）位于 level-0 键（0\Paths）。含拦截规则的自有策略根下
// 存在 "0" 子键，形态校验必须认识它——此前不认识导致任何拦截规则写入都被
// SafeTransitionRoot 误判为"根被外部改动"而中止回滚。
func TestRootShapesRecognizeBlockRulesLevelKey(t *testing.T) {
	withBlock := ownedRootSnapshot(0, 1, true)
	if !OwnedRootMatches(withBlock, 0, 1, true) {
		t.Fatal("owned root with block-rule level key must match when block rules are expected")
	}
	if OwnedRootMatches(withBlock, 0, 1, false) {
		t.Fatal("a level-0 key must count as drift when no block rules are expected")
	}
	withoutBlock := ownedRootSnapshot(0, 1, false)
	if OwnedRootMatches(withoutBlock, 0, 1, true) {
		t.Fatal("expected block rules but level-0 key missing must count as drift")
	}
	if !SafeTransitionRoot(withBlock, SRPDispositionAbsent) {
		t.Fatal("mid-transition root with block-rule level key must be safe")
	}

	hashTampered := withBlock
	hashTampered.BlockChildren = []string{"Paths", "Hashes"}
	if SafeTransitionRoot(hashTampered, SRPDispositionAbsent) {
		t.Fatal("foreign Hashes under level 0 must make transition unsafe")
	}
	if OwnedRootMatches(hashTampered, 0, 1, true) {
		t.Fatal("foreign Hashes under level 0 must count as drift")
	}

	unknownChild := withBlock
	unknownChild.Children = []string{"0", "262144", "131072"}
	if SafeTransitionRoot(unknownChild, SRPDispositionAbsent) {
		t.Fatal("unknown security-level child must make transition unsafe")
	}

	// 旧版本从未写过拦截规则：legacy 兼容匹配在期望含拦截规则时必须失败。
	legacy := SRPRootSnapshot{Exists: true, Values: []SRPRootValue{
		{Name: "DefaultLevel", Kind: SRPValueDWORD, Integer: 0},
		{Name: "PolicyScope", Kind: SRPValueDWORD, Integer: 1},
		{Name: "TransparentEnabled", Kind: SRPValueDWORD, Integer: 1},
	}, Children: []string{"262144"}, LevelChildren: []string{"Paths"}}
	if OwnedRootMatchesLegacyCompatible(legacy, 0, 1, true) {
		t.Fatal("legacy shapes must not satisfy a plan that expects block rules")
	}
}

func TestExecutableTypesExcludeShortcutFileTypes(t *testing.T) {
	for _, banned := range []string{"LNK", "URL"} {
		for _, value := range srpExecutableTypes {
			if strings.EqualFold(value, banned) {
				t.Fatalf("designated file types must not contain %s", banned)
			}
		}
	}
	legacy := map[string]bool{}
	for _, value := range srpExecutableTypesLegacy {
		legacy[strings.ToUpper(value)] = true
	}
	if !legacy["LNK"] || !legacy["URL"] {
		t.Fatal("legacy list must keep LNK/URL so pre-fix policies stay recognizable for migration")
	}
}

func TestOwnedRootAcceptsLegacyExecutableTypesForMigration(t *testing.T) {
	legacy := SRPRootSnapshot{Exists: true, Values: []SRPRootValue{
		{Name: "DefaultLevel", Kind: SRPValueDWORD, Integer: 0},
		{Name: "PolicyScope", Kind: SRPValueDWORD, Integer: 1},
		{Name: "TransparentEnabled", Kind: SRPValueDWORD, Integer: 1},
		{Name: "ExecutableTypes", Kind: SRPValueMULTISZ, Strings: append([]string(nil), srpExecutableTypesLegacy...)},
	}, Children: []string{"262144"}, LevelChildren: []string{"Paths"}}
	if OwnedRootMatches(legacy, 0, 1, false) {
		t.Fatal("strict matching must require the current ExecutableTypes list")
	}
	if !OwnedRootMatchesLegacyCompatible(legacy, 0, 1, false) {
		t.Fatal("pre-LNK/URL-removal root must remain eligible for migration")
	}
	if !SafeTransitionRoot(legacy, SRPDispositionAbsent) {
		t.Fatal("removing a pre-fix policy must remain a safe transition")
	}
	tampered := legacy
	tampered.Values = append([]SRPRootValue(nil), legacy.Values...)
	tampered.Values[3].Strings = append([]string(nil), srpExecutableTypesLegacy...)
	tampered.Values[3].Strings[0] = "DLL"
	if OwnedRootMatchesLegacyCompatible(tampered, 0, 1, false) {
		t.Fatal("a tampered ExecutableTypes list must not pass the legacy migration gate")
	}
	if SafeTransitionRoot(tampered, SRPDispositionAbsent) {
		t.Fatal("a tampered ExecutableTypes list must not be a safe transition")
	}
}
