package platform

import (
	"sort"
	"strconv"
	"strings"
)

const (
	SRPDispositionAbsent            = "absent"
	SRPDispositionInertUnrestricted = "inert_unrestricted"
	SRPDispositionManaged           = "managed"
	SRPValueDWORD                   = "dword"
	SRPValueMULTISZ                 = "multi_sz"
)

type SRPRootValue struct {
	Name    string
	Kind    string
	Integer uint64
	Strings []string
}

type SRPRootSnapshot struct {
	Exists        bool
	Values        []SRPRootValue
	Children      []string
	LevelChildren []string
	// BlockChildren 是 level-0（Disallowed）子键下的子键名。拦截规则以
	// GUID 子键形式存放在 0\Paths 下，是 XCLing 自有形态的一部分。
	BlockChildren []string
}

func ClassifySRPRoot(snapshot SRPRootSnapshot) string {
	if !snapshot.Exists {
		return SRPDispositionAbsent
	}
	if len(snapshot.Children) != 0 || len(snapshot.Values) != 1 {
		return SRPDispositionManaged
	}
	value := snapshot.Values[0]
	if strings.EqualFold(value.Name, "DefaultLevel") && value.Kind == SRPValueDWORD && value.Integer == 262144 {
		return SRPDispositionInertUnrestricted
	}
	return SRPDispositionManaged
}

func SameSRPRootSnapshot(left, right SRPRootSnapshot) bool {
	if left.Exists != right.Exists || len(left.Values) != len(right.Values) || len(left.Children) != len(right.Children) || len(left.LevelChildren) != len(right.LevelChildren) || len(left.BlockChildren) != len(right.BlockChildren) {
		return false
	}
	values := func(items []SRPRootValue) []string {
		result := make([]string, 0, len(items))
		for _, item := range items {
			values := make([]string, len(item.Strings))
			for i, value := range item.Strings {
				values[i] = strings.ToLower(value)
			}
			sort.Strings(values)
			result = append(result, strings.ToLower(item.Name)+"\x00"+item.Kind+"\x00"+strconv.FormatUint(item.Integer, 10)+"\x00"+strings.Join(values, "\x01"))
		}
		sort.Strings(result)
		return result
	}
	children := func(items []string) []string {
		result := make([]string, len(items))
		for i, item := range items {
			result[i] = strings.ToLower(item)
		}
		sort.Strings(result)
		return result
	}
	return equalStrings(values(left.Values), values(right.Values)) && equalStrings(children(left.Children), children(right.Children)) && equalStrings(children(left.LevelChildren), children(right.LevelChildren)) && equalStrings(children(left.BlockChildren), children(right.BlockChildren))
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
func SafeTransitionRoot(snapshot SRPRootSnapshot, beforeDisposition string) bool {
	if !snapshot.Exists || (beforeDisposition != SRPDispositionAbsent && beforeDisposition != SRPDispositionInertUnrestricted) {
		return false
	}
	// 自有形态最多包含两个级别子键：262144（Unrestricted 放行）与 0（Disallowed
	// 拦截）。任何其它子键都视为外部改动。
	hasUnrestricted, hasDisallowed := false, false
	for _, child := range snapshot.Children {
		switch {
		case strings.EqualFold(child, "262144"):
			if hasUnrestricted {
				return false
			}
			hasUnrestricted = true
		case strings.EqualFold(child, "0"):
			if hasDisallowed {
				return false
			}
			hasDisallowed = true
		default:
			return false
		}
	}
	if len(snapshot.LevelChildren) > 1 || (len(snapshot.LevelChildren) == 1 && !strings.EqualFold(snapshot.LevelChildren[0], "Paths")) {
		return false
	}
	if !hasUnrestricted && len(snapshot.LevelChildren) != 0 {
		return false
	}
	if len(snapshot.BlockChildren) > 1 || (len(snapshot.BlockChildren) == 1 && !strings.EqualFold(snapshot.BlockChildren[0], "Paths")) {
		return false
	}
	if !hasDisallowed && len(snapshot.BlockChildren) != 0 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range snapshot.Values {
		name := strings.ToLower(value.Name)
		if seen[name] {
			return false
		}
		seen[name] = true
		switch name {
		case "defaultlevel":
			if value.Kind != SRPValueDWORD {
				return false
			}
			// 0（白名单锁定）和 262144（临时解锁 / 仅拦截模式）都是本程序会写入的
			// 级别；仅拦截模式接管前根键可能不存在，因此不再要求 262144 必须
			// 来自 inert_unrestricted 接管。
			if value.Integer != 0 && value.Integer != 262144 {
				return false
			}
		case "policyscope":
			if value.Kind != SRPValueDWORD {
				return false
			}
			if value.Integer != 0 && value.Integer != 1 {
				return false
			}
		case "transparentenabled":
			if value.Kind != SRPValueDWORD {
				return false
			}
			if value.Integer != 1 {
				return false
			}
		case "logevent":
			if value.Kind != SRPValueDWORD {
				return false
			}
			if value.Integer != 1 {
				return false
			}
		case "executabletypes":
			if value.Kind != SRPValueMULTISZ || !isOwnedExecutableTypesList(value.Strings) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func AppliedRootMatches(snapshot SRPRootSnapshot) bool {
	return OwnedRootMatches(snapshot, 0, 1, false)
}

// ownedRootSnapshot 是 XCLing 自有策略激活后的精确根形态。含拦截规则时
// 额外存在 level-0 子键（0\Paths 存放拦截 GUID 子键）。
func ownedRootSnapshot(defaultLevel, policyScope uint64, hasBlockRules bool) SRPRootSnapshot {
	snapshot := SRPRootSnapshot{Exists: true, Values: []SRPRootValue{
		{Name: "DefaultLevel", Kind: SRPValueDWORD, Integer: defaultLevel},
		{Name: "PolicyScope", Kind: SRPValueDWORD, Integer: policyScope},
		{Name: "TransparentEnabled", Kind: SRPValueDWORD, Integer: 1},
		{Name: "LogEvent", Kind: SRPValueDWORD, Integer: 1},
		{Name: "ExecutableTypes", Kind: SRPValueMULTISZ, Strings: srpExecutableTypes},
	}, Children: []string{"262144"}, LevelChildren: []string{"Paths"}}
	if hasBlockRules {
		snapshot.Children = []string{"0", "262144"}
		snapshot.BlockChildren = []string{"Paths"}
	}
	return snapshot
}

// OwnedRootMatches verifies the root shape owned by XCLing for a locked
// or temporarily unlocked policy. hasBlockRules 表示自有计划是否包含
// level-0 拦截规则，决定根下是否应存在 "0" 子键。
func OwnedRootMatches(snapshot SRPRootSnapshot, defaultLevel, policyScope uint64, hasBlockRules bool) bool {
	want := ownedRootSnapshot(defaultLevel, policyScope, hasBlockRules)
	if SameSRPRootSnapshot(snapshot, want) {
		return true
	}
	// Also allow old shape without LogEvent for migration compatibility
	withoutLog := want
	withoutLog.Values = make([]SRPRootValue, 0, len(want.Values)-1)
	for _, value := range want.Values {
		if !strings.EqualFold(value.Name, "LogEvent") {
			withoutLog.Values = append(withoutLog.Values, value)
		}
	}
	return SameSRPRootSnapshot(snapshot, withoutLog)
}

// OwnedRootMatchesLegacyCompatible also accepts the exact root shapes emitted
// by earlier versions: before ExecutableTypes was added, and with the legacy
// ExecutableTypes list that still contained LNK/URL. Callers that can write the
// registry use this only as a migration gate, then verify the strict shape.
// 旧版本从未写过 level-0 拦截规则，因此含拦截规则的计划只可能匹配严格形态。
func OwnedRootMatchesLegacyCompatible(snapshot SRPRootSnapshot, defaultLevel, policyScope uint64, hasBlockRules bool) bool {
	if OwnedRootMatches(snapshot, defaultLevel, policyScope, hasBlockRules) {
		return true
	}
	if hasBlockRules {
		return false
	}
	legacyShapes := []SRPRootSnapshot{
		{Exists: true, Values: []SRPRootValue{
			{Name: "DefaultLevel", Kind: SRPValueDWORD, Integer: defaultLevel},
			{Name: "PolicyScope", Kind: SRPValueDWORD, Integer: policyScope},
			{Name: "TransparentEnabled", Kind: SRPValueDWORD, Integer: 1},
		}, Children: []string{"262144"}, LevelChildren: []string{"Paths"}},
		{Exists: true, Values: []SRPRootValue{
			{Name: "DefaultLevel", Kind: SRPValueDWORD, Integer: defaultLevel},
			{Name: "PolicyScope", Kind: SRPValueDWORD, Integer: policyScope},
			{Name: "TransparentEnabled", Kind: SRPValueDWORD, Integer: 1},
			{Name: "ExecutableTypes", Kind: SRPValueMULTISZ, Strings: srpExecutableTypesLegacy},
		}, Children: []string{"262144"}, LevelChildren: []string{"Paths"}},
	}
	for _, legacy := range legacyShapes {
		if SameSRPRootSnapshot(snapshot, legacy) {
			return true
		}
	}
	return false
}

func normalizedStrings(values []string) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = strings.ToLower(value)
	}
	sort.Strings(result)
	return result
}

// srpExecutableTypes is the designated-file-type list XCLing writes: the
// Windows default list minus LNK and URL. Shortcut files live in user-writable
// locations (Desktop, Start Menu, taskbar) that a whitelist policy blocks, and
// the Windows 7 shell checks the shortcut file itself against this list, so
// keeping LNK/URL blocks every shortcut launch under DefaultLevel=Disallowed.
// Shortcut targets are still SAFER-checked at process creation.
var srpExecutableTypes = []string{
	"ADE", "ADP", "BAS", "BAT", "CHM", "CMD", "COM", "CPL", "CRT", "EXE",
	"HLP", "HTA", "INF", "INS", "ISP", "MDB", "MDE", "MSC", "MSI", "MSP",
	"MST", "OCX", "PCD", "PIF", "REG", "SCR", "SHS", "VB", "WSC",
}

// srpExecutableTypesLegacy is the full Windows default list written by versions
// before LNK/URL were removed. Recognized only so owned policies written by
// those versions stay manageable and get migrated; never written again.
var srpExecutableTypesLegacy = []string{
	"ADE", "ADP", "BAS", "BAT", "CHM", "CMD", "COM", "CPL", "CRT", "EXE",
	"HLP", "HTA", "INF", "INS", "ISP", "LNK", "MDB", "MDE", "MSC", "MSI",
	"MSP", "MST", "OCX", "PCD", "PIF", "REG", "SCR", "SHS", "URL", "VB",
	"WSC",
}

// isOwnedExecutableTypesList reports whether values match the current or the
// legacy list written by XCLing, ignoring order and case.
func isOwnedExecutableTypesList(values []string) bool {
	normalized := normalizedStrings(values)
	return equalStrings(normalized, normalizedStrings(srpExecutableTypes)) ||
		equalStrings(normalized, normalizedStrings(srpExecutableTypesLegacy))
}
