//go:build windows

package platform

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	pgapply "XCLing/internal/apply"
	"XCLing/internal/model"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const unrestrictedPaths = model.SRPRegistryPath + `\` + "262144" + `\Paths`

// disallowedPaths holds the explicit vendor blocklist. SRP resolves the most
// specific path rule first regardless of level, so a rule here outranks any
// broader allow rule and stays in force during a temporary unlock.
const disallowedPaths = model.SRPRegistryPath + `\` + "0" + `\Paths`

var regSetValueExW = windows.NewLazySystemDLL("advapi32.dll").NewProc("RegSetValueExW")

// 规则 ID 是 deterministicRuleID 生成的带花括号 GUID，直接作为注册表子键名，
// 严格匹配该形态即可同时阻断路径穿越等注入。
var validRuleIDPattern = regexp.MustCompile(`^\{[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\}$`)

func validateRuleID(id string) error {
	if !validRuleIDPattern.MatchString(id) {
		return fmt.Errorf("invalid rule ID format: %q", id)
	}
	return nil
}

func SRPKeyExists() (bool, error) {
	snapshot, err := InspectSRPRoot()
	return snapshot.Exists, err
}

// WriteSRPPlan is the only production function allowed to write SRP registry state.
func WriteSRPPlan(plan pgapply.Plan) error {
	return WriteSRPPlanFrom(plan, SRPRootSnapshot{})
}

// WriteSRPPlanReplacing replaces an arbitrary local SRP tree after verifying
// it still matches the snapshot captured for recovery.
func WriteSRPPlanReplacing(plan pgapply.Plan, before model.RegistryTreeSnapshot) error {
	current, err := SnapshotSRPRegistry()
	if err != nil {
		return err
	}
	if !current.Equal(before) {
		return errors.New(model.ApplyErrLegacySRPDrifted + ": SRP tree changed after backup")
	}
	if err := RestoreSRPRegistrySnapshot(model.RegistryTreeSnapshot{}); err != nil {
		return fmt.Errorf("clear existing SRP tree: %w", err)
	}
	if err := WriteSRPPlanFrom(plan, SRPRootSnapshot{}); err != nil {
		if restoreErr := RestoreSRPRegistrySnapshot(before); restoreErr != nil {
			return fmt.Errorf("write policy: %v; restore original SRP: %w", err, restoreErr)
		}
		return err
	}
	return nil
}

// SetSRPProtectionState switches an owned policy between locked and temporary
// unlock without deleting any rules or the recovery record.
func SetSRPProtectionState(expected pgapply.Plan, locked bool) error {
	actual, err := ReadSRPPlan()
	if err != nil {
		return err
	}
	currentState := pgapply.DetectProtectionState(actual, expected)
	if currentState == model.ProtectionStateAttention {
		return errors.New(model.ApplyErrPolicyDrifted + ": current policy no longer matches the owned plan")
	}
	rootSnapshot, err := InspectSRPRoot()
	if err != nil {
		return err
	}
	if !OwnedRootMatchesLegacyCompatible(rootSnapshot, uint64(actual.DefaultLevel), uint64(expected.PolicyScope), len(expected.BlockRules) > 0) {
		return errors.New(model.ApplyErrPolicyDrifted + ": current SRP root has unexpected values")
	}
	if err := ensureSRPExecutableTypes(); err != nil {
		return err
	}
	if err := ensureSRPRuleValueTypes(expected); err != nil {
		return err
	}
	if err := RefreshSRPPolicy(); err != nil {
		return err
	}
	rootSnapshot, err = InspectSRPRoot()
	if err != nil || !SameSRPRootSnapshot(rootSnapshot, ownedRootSnapshot(uint64(actual.DefaultLevel), uint64(expected.PolicyScope), len(expected.BlockRules) > 0)) {
		if err == nil {
			err = errors.New("SRP executable types verification mismatch")
		}
		return err
	}
	desired := model.SrpLevelUnrestrictedRaw
	desiredState := model.ProtectionStateUnlocked
	if locked {
		desired = pgapply.DefaultLevelDisallowed
		desiredState = model.ProtectionStateLocked
	}
	if actual.DefaultLevel == desired {
		return nil
	}
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, model.SRPRegistryPath, registry.WRITE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	// Ensure SRP event logging is enabled
	setErr := root.SetDWordValue("LogEvent", 1)
	if setErr == nil {
		setErr = root.SetDWordValue("DefaultLevel", uint32(desired))
	}
	closeErr := root.Close()
	if setErr != nil {
		return fmt.Errorf("set SRP protection state: %w", setErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close SRP root: %w", closeErr)
	}
	if err := RefreshSRPPolicy(); err != nil {
		return err
	}
	verified, err := ReadSRPPlan()
	if err == nil && pgapply.DetectProtectionState(verified, expected) == desiredState {
		verifiedRoot, rootErr := InspectSRPRoot()
		if rootErr == nil && OwnedRootMatches(verifiedRoot, uint64(desired), uint64(expected.PolicyScope), len(expected.BlockRules) > 0) {
			return nil
		}
		if rootErr != nil {
			err = rootErr
		} else {
			err = errors.New("SRP root verification mismatch")
		}
	}
	rollbackRoot, openErr := registry.OpenKey(registry.LOCAL_MACHINE, model.SRPRegistryPath, registry.WRITE|registry.SET_VALUE)
	if openErr == nil {
		_ = rollbackRoot.SetDWordValue("DefaultLevel", uint32(actual.DefaultLevel))
		_ = rollbackRoot.Close()
	}
	if err == nil {
		err = errors.New("SRP protection state verification mismatch")
	}
	return err
}

func ensureSRPExecutableTypes() error {
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, model.SRPRegistryPath, registry.WRITE|registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer root.Close()
	values, _, readErr := root.GetStringsValue("ExecutableTypes")
	if readErr == nil {
		if equalStrings(normalizedStrings(values), normalizedStrings(srpExecutableTypes)) {
			return nil
		}
		// The legacy list (with LNK/URL) falls through to be rewritten below.
		if !equalStrings(normalizedStrings(values), normalizedStrings(srpExecutableTypesLegacy)) {
			return errors.New(model.ApplyErrPolicyDrifted + ": ExecutableTypes has unexpected values")
		}
	} else if !errors.Is(readErr, registry.ErrNotExist) && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read SRP ExecutableTypes: %w", readErr)
	}
	if err := root.SetStringsValue("ExecutableTypes", srpExecutableTypes); err != nil {
		return fmt.Errorf("migrate SRP ExecutableTypes: %w", err)
	}
	return nil
}

func ensureSRPRuleValueTypes(expected pgapply.Plan) error {
	for _, rule := range expected.Rules {
		if err := validateRuleID(rule.ID); err != nil {
			return err
		}
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, unrestrictedPaths+`\`+rule.ID, registry.WRITE|registry.SET_VALUE|registry.QUERY_VALUE)
		if err != nil {
			return fmt.Errorf("open SRP rule %s for migration: %w", rule.ID, err)
		}
		defer key.Close()

		path, valueType, readErr := key.GetStringValue("ItemData")
		if readErr != nil {
			return fmt.Errorf("read SRP rule %s ItemData: %w", rule.ID, readErr)
		}
		if path != rule.Path {
			return errors.New(model.ApplyErrPolicyDrifted + ": SRP rule path changed during migration")
		}
		if valueType == registry.SZ {
			err = setSRPPathValue(key, path)
		} else if valueType != registry.EXPAND_SZ {
			err = fmt.Errorf("SRP rule %s ItemData has unexpected type %d", rule.ID, valueType)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func WriteSRPPlanFrom(plan pgapply.Plan, before SRPRootSnapshot) (err error) {
	current, err := InspectSRPRoot()
	if err != nil {
		return err
	}
	if !SameSRPRootSnapshot(current, before) {
		return errors.New(model.ApplyErrLegacySRPDrifted + ": SRP root changed after backup")
	}
	disposition := ClassifySRPRoot(before)
	if disposition != SRPDispositionAbsent && disposition != SRPDispositionInertUnrestricted {
		return errors.New("SRP root is not replaceable")
	}
	defer func() {
		if err != nil {
			_ = RestoreSRPBeforeState(plan, disposition)
		}
	}()
	if disposition == SRPDispositionAbsent {
		root, _, createErr := registry.CreateKey(registry.LOCAL_MACHINE, model.SRPRegistryPath, registry.WRITE|registry.CREATE_SUB_KEY|registry.SET_VALUE)
		if createErr != nil {
			return fmt.Errorf("create SRP root: %w", createErr)
		}
		_ = root.Close()
	}
	paths, _, err := registry.CreateKey(registry.LOCAL_MACHINE, unrestrictedPaths, registry.WRITE|registry.CREATE_SUB_KEY)
	if err != nil {
		return fmt.Errorf("create unrestricted paths: %w", err)
	}
	defer paths.Close()

	for _, rule := range plan.Rules {
		if e := validateRuleID(rule.ID); e != nil {
			return e
		}
		key, _, e := registry.CreateKey(registry.LOCAL_MACHINE, unrestrictedPaths+`\`+rule.ID, registry.WRITE|registry.SET_VALUE)
		if e != nil {
			return fmt.Errorf("create rule %s: %w", rule.ID, e)
		}
		if e = setSRPPathValue(key, rule.Path); e == nil {
			e = key.SetStringValue("Description", rule.Description)
		}
		if e == nil {
			e = key.SetDWordValue("SaferFlags", 0)
		}
		if e == nil {
			e = key.SetQWordValue("LastModified", uint64(time.Now().UTC().UnixNano()/100)+116444736000000000)
		}
		_ = key.Close()
		if e != nil {
			return fmt.Errorf("write rule %s: %w", rule.ID, e)
		}
	}
	// Write explicit disallow (block) rules under level 0.
	if len(plan.BlockRules) > 0 {
		if e := writeBlockRules(plan.BlockRules); e != nil {
			return e
		}
	}
	transition, err := InspectSRPRoot()
	if err != nil || !SafeTransitionRoot(transition, disposition) {
		if err == nil {
			err = errors.New(model.ApplyErrLegacySRPDrifted + ": unexpected SRP root change before activation")
		}
		return err
	}
	readBack, err := ReadSRPPlan()
	if err != nil {
		return fmt.Errorf("verify rules: %w", err)
	}
	readBack.DefaultLevel = plan.DefaultLevel
	readBack.PolicyScope = plan.PolicyScope
	readBack.TransparentEnabled = plan.TransparentEnabled
	if pgapply.Fingerprint(readBack) != pgapply.Fingerprint(plan) {
		return errors.New("rule verification mismatch")
	}
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, model.SRPRegistryPath, registry.WRITE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	if err = root.SetDWordValue("PolicyScope", uint32(plan.PolicyScope)); err == nil {
		err = root.SetStringsValue("ExecutableTypes", srpExecutableTypes)
	}
	if err == nil {
		err = root.SetDWordValue("TransparentEnabled", uint32(plan.TransparentEnabled))
	}
	// Enable SRP event logging (Event ID 865/866 in Application log) so interceptions are recorded
	if err == nil {
		err = root.SetDWordValue("LogEvent", 1)
	}
	// DefaultLevel is the final activation edge. Win7 must see the complete
	// designated-file-type list and unrestricted path tree before disallowing.
	if err == nil {
		err = root.SetDWordValue("DefaultLevel", uint32(plan.DefaultLevel))
	}
	_ = root.Close()
	if err != nil {
		return fmt.Errorf("activate SRP: %w", err)
	}
	if err := RefreshSRPPolicy(); err != nil {
		return err
	}
	appliedRoot, err := InspectSRPRoot()
	if err != nil || !OwnedRootMatches(appliedRoot, uint64(plan.DefaultLevel), uint64(plan.PolicyScope), len(plan.BlockRules) > 0) {
		if err == nil {
			err = errors.New("applied SRP root verification mismatch")
		}
		return err
	}
	return nil
}

// SRP path rules created by the Windows policy editor use REG_EXPAND_SZ for
// ItemData, even when the path contains no environment variable. Win7 is
// stricter about this type than newer Windows versions.
func setSRPPathValue(key registry.Key, path string) error {
	values, err := windows.UTF16FromString(path)
	if err != nil {
		return err
	}
	data := make([]byte, len(values)*2)
	for i, value := range values {
		binary.LittleEndian.PutUint16(data[i*2:], value)
	}
	return setRawRegistryValue(key, "ItemData", registry.EXPAND_SZ, data)
}

// writeBlockRules writes explicit disallow rules under the SRP level-0 Paths key.
// These rules are more specific than the level-262144 allow rules and therefore
// take priority. They survive a temporary unlock (DefaultLevel flip) unchanged.
func writeBlockRules(rules []pgapply.Rule) error {
	blockPaths, _, err := registry.CreateKey(registry.LOCAL_MACHINE, disallowedPaths, registry.WRITE|registry.CREATE_SUB_KEY)
	if err != nil {
		return fmt.Errorf("create disallowed paths: %w", err)
	}
	_ = blockPaths.Close()
	for _, rule := range rules {
		if e := validateRuleID(rule.ID); e != nil {
			return e
		}
		key, _, e := registry.CreateKey(registry.LOCAL_MACHINE, disallowedPaths+`\`+rule.ID, registry.WRITE|registry.SET_VALUE)
		if e != nil {
			return fmt.Errorf("create block rule %s: %w", rule.ID, e)
		}
		if e = setSRPPathValue(key, rule.Path); e == nil {
			e = key.SetStringValue("Description", rule.Description)
		}
		if e == nil {
			e = key.SetDWordValue("SaferFlags", 0)
		}
		if e == nil {
			e = key.SetQWordValue("LastModified", uint64(time.Now().UTC().UnixNano()/100)+116444736000000000)
		}
		_ = key.Close()
		if e != nil {
			return fmt.Errorf("write block rule %s: %w", rule.ID, e)
		}
	}
	return nil
}

func ReadSRPPlan() (pgapply.Plan, error) {
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, model.SRPRegistryPath, registry.READ)
	if err != nil {
		return pgapply.Plan{}, err
	}
	defer root.Close()
	plan := pgapply.Plan{}
	if value, _, e := root.GetIntegerValue("DefaultLevel"); e == nil {
		plan.DefaultLevel = int(value)
	}
	if value, _, e := root.GetIntegerValue("PolicyScope"); e == nil {
		plan.PolicyScope = int(value)
	}
	if value, _, e := root.GetIntegerValue("TransparentEnabled"); e == nil {
		plan.TransparentEnabled = int(value)
	}
	paths, err := registry.OpenKey(registry.LOCAL_MACHINE, unrestrictedPaths, registry.READ|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			return plan, nil
		}
		return pgapply.Plan{}, err
	}
	defer paths.Close()
	names, err := paths.ReadSubKeyNames(-1)
	if err != nil {
		return pgapply.Plan{}, err
	}
	for _, name := range names {
		key, e := registry.OpenKey(registry.LOCAL_MACHINE, unrestrictedPaths+`\`+name, registry.READ)
		if e != nil {
			return pgapply.Plan{}, e
		}
		path, _, pathErr := key.GetStringValue("ItemData")
		description, _, _ := key.GetStringValue("Description")
		_ = key.Close()
		if pathErr != nil {
			return pgapply.Plan{}, pathErr
		}
		plan.Rules = append(plan.Rules, pgapply.Rule{ID: name, Path: path, Description: description, Level: model.SrpLevelUnrestrictedRaw})
	}
	// Read block (explicit disallow) rules from level-0 Paths.
	blockRules, err := readDisallowedRules()
	if err != nil {
		return pgapply.Plan{}, err
	}
	plan.BlockRules = blockRules
	return plan, nil
}

func readDisallowedRules() ([]pgapply.Rule, error) {
	paths, err := registry.OpenKey(registry.LOCAL_MACHINE, disallowedPaths, registry.READ|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer paths.Close()
	names, err := paths.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	var rules []pgapply.Rule
	for _, name := range names {
		key, e := registry.OpenKey(registry.LOCAL_MACHINE, disallowedPaths+`\`+name, registry.READ)
		if e != nil {
			return nil, e
		}
		path, _, pathErr := key.GetStringValue("ItemData")
		description, _, _ := key.GetStringValue("Description")
		_ = key.Close()
		if pathErr != nil {
			return nil, pathErr
		}
		rules = append(rules, pgapply.Rule{ID: name, Path: path, Description: description, Level: model.SrpLevelDisallowedRaw})
	}
	return rules, nil
}

func RemoveSRPPlan(plan pgapply.Plan) error {
	return RestoreSRPBeforeState(plan, SRPDispositionAbsent)
}

func RestoreSRPBeforeState(plan pgapply.Plan, beforeState string) error {
	snapshot, err := InspectSRPRoot()
	if err != nil {
		return err
	}
	if snapshot.Exists {
		actual, readErr := ReadSRPPlan()
		if readErr != nil || !pgapply.PartialMatches(actual, plan) || !SafeTransitionRoot(snapshot, beforeState) {
			return errors.New(model.ApplyErrLegacySRPDrifted + ": current SRP state is not a safe " + model.AppName + " transition")
		}
	}
	var first error
	for _, rule := range plan.Rules {
		if err := registry.DeleteKey(registry.LOCAL_MACHINE, unrestrictedPaths+`\`+rule.ID); err != nil && !errors.Is(err, registry.ErrNotExist) && first == nil {
			first = err
		}
	}
	for _, rule := range plan.BlockRules {
		if err := registry.DeleteKey(registry.LOCAL_MACHINE, disallowedPaths+`\`+rule.ID); err != nil && !errors.Is(err, registry.ErrNotExist) && first == nil {
			first = err
		}
	}
	levelPaths := []string{
		unrestrictedPaths, model.SRPRegistryPath + `\` + strconv.Itoa(model.SrpLevelUnrestrictedRaw),
		disallowedPaths, model.SRPRegistryPath + `\` + strconv.Itoa(model.SrpLevelDisallowedRaw),
	}
	for _, path := range levelPaths {
		if err := registry.DeleteKey(registry.LOCAL_MACHINE, path); err != nil && !errors.Is(err, registry.ErrNotExist) && first == nil {
			first = err
		}
	}
	if first != nil {
		return first
	}
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, model.SRPRegistryPath, registry.WRITE|registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) && beforeState == SRPDispositionAbsent {
			return nil
		}
		return err
	}
	for _, name := range []string{"PolicyScope", "TransparentEnabled", "ExecutableTypes"} {
		if deleteErr := root.DeleteValue(name); deleteErr != nil && !errors.Is(deleteErr, registry.ErrNotExist) && first == nil {
			first = deleteErr
		}
	}
	if beforeState == SRPDispositionInertUnrestricted {
		if setErr := root.SetDWordValue("DefaultLevel", model.SrpLevelUnrestrictedRaw); setErr != nil && first == nil {
			first = setErr
		}
	} else if deleteErr := root.DeleteValue("DefaultLevel"); deleteErr != nil && !errors.Is(deleteErr, registry.ErrNotExist) && first == nil {
		first = deleteErr
	}
	_ = root.Close()
	if first != nil {
		return first
	}
	if beforeState == SRPDispositionAbsent {
		if err := registry.DeleteKey(registry.LOCAL_MACHINE, model.SRPRegistryPath); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
	}
	if err := RefreshSRPPolicy(); err != nil {
		return err
	}
	final, err := InspectSRPRoot()
	if err != nil {
		return err
	}
	if ClassifySRPRoot(final) != beforeState {
		return errors.New("restored SRP state verification mismatch")
	}
	return nil
}

func replaceRegistryTree(root registry.Key, path string, snapshot model.RegistryTreeSnapshot) error {
	if err := deleteRegistryTree(root, path); err != nil {
		return err
	}
	if !snapshot.Exists {
		return nil
	}
	key, _, err := registry.CreateKey(root, path, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("create registry tree %q: %w", path, err)
	}
	defer key.Close()

	writeErr := writeRegistryKey(key, path, snapshot.Root)
	if writeErr != nil {
		return writeErr
	}
	return nil
}

func writeRegistryKey(key registry.Key, path string, snapshot model.RegistryKeySnapshot) error {
	for _, value := range snapshot.Values {
		if err := setRawRegistryValue(key, value.Name, value.Type, value.Data); err != nil {
			return fmt.Errorf("write registry value %q in %q: %w", value.Name, path, err)
		}
	}
	for _, childSnapshot := range snapshot.Children {
		child, _, err := registry.CreateKey(key, childSnapshot.Name, registry.ALL_ACCESS)
		if err != nil {
			return fmt.Errorf("create child %q in %q: %w", childSnapshot.Name, path, err)
		}
		defer child.Close()

		writeErr := writeRegistryKey(child, path+`\`+childSnapshot.Name, childSnapshot.Key)
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func setRawRegistryValue(key registry.Key, name string, valueType uint32, data []byte) error {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	var dataPtr uintptr
	if len(data) > 0 {
		dataPtr = uintptr(unsafe.Pointer(&data[0]))
	}
	result, _, callErr := regSetValueExW.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(namePtr)),
		0,
		uintptr(valueType),
		dataPtr,
		uintptr(len(data)),
	)
	if result != uintptr(windows.ERROR_SUCCESS) {
		if callErr != nil && !errors.Is(callErr, syscall.Errno(0)) {
			return callErr
		}
		return syscall.Errno(result)
	}
	return nil
}

func deleteRegistryTree(root registry.Key, path string) error {
	key, err := registry.OpenKey(root, path, registry.READ|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open registry tree for deletion %q: %w", path, err)
	}
	defer key.Close()

	children, readErr := key.ReadSubKeyNames(-1)
	if readErr != nil {
		return fmt.Errorf("read registry children for deletion %q: %w", path, readErr)
	}
	for _, child := range children {
		if err := deleteRegistryTree(root, path+`\`+child); err != nil {
			return err
		}
	}
	if err := registry.DeleteKey(root, path); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("delete registry key %q: %w", path, err)
	}
	return nil
}

// RestoreSRPRegistrySnapshot replaces the local SRP tree with an earlier complete snapshot.
func RestoreSRPRegistrySnapshot(snapshot model.RegistryTreeSnapshot) error {
	if err := replaceRegistryTree(registry.LOCAL_MACHINE, model.SRPRegistryPath, snapshot); err != nil {
		return err
	}
	return RefreshSRPPolicy()
}
