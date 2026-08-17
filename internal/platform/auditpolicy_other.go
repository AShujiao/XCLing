//go:build !windows

package platform

// AuditPolicyStatus describes the current state of SRP-relevant audit policies.
// On non-Windows platforms auditing is not applicable.
type AuditPolicyStatus struct {
	Available   bool
	Enabled     bool
	Detail      string
	ErrorDetail string
}

// GetAuditPolicyStatus returns not-available on non-Windows.
func GetAuditPolicyStatus() AuditPolicyStatus {
	return AuditPolicyStatus{Available: false, Enabled: false, Detail: "仅Windows平台支持审核策略"}
}

// EnableAuditPolicy is a no-op on non-Windows.
func EnableAuditPolicy() error {
	return nil
}
