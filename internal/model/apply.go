package model

const (
	ApplyErrAdminRequired     = "ADMIN_REQUIRED"
	ApplyErrDomainManaged     = "DOMAIN_MANAGED"
	ApplyErrExistingSRP       = "EXISTING_SRP"
	ApplyErrLegacySRPDrifted  = "LEGACY_SRP_DRIFTED"
	ApplyErrAlreadyApplied    = "ALREADY_APPLIED"
	ApplyErrOperationProgress = "OPERATION_IN_PROGRESS"
	ApplyErrPreflightBlocked  = "PREFLIGHT_BLOCKED"
	ApplyErrSelfNotAllowed    = "SELF_NOT_ALLOWED"
	ApplyErrWriteRolledBack   = "WRITE_FAILED_ROLLED_BACK"
	ApplyErrVerifyRolledBack  = "VERIFY_FAILED_ROLLED_BACK"
	ApplyErrPolicyDrifted     = "POLICY_DRIFTED"
	ApplyErrNoRecoveryRecord  = "NO_RECOVERY_RECORD"
	ApplyErrRecoveryFailed    = "RECOVERY_FAILED"
	ApplyErrUnsupported       = "UNSUPPORTED_PLATFORM"
	ApplyErrInvalidState      = "INVALID_STATE"
	ApplyErrRuleNotRemovable  = "RULE_NOT_REMOVABLE"
	RecoveryStatePrepared     = "prepared"
	RecoveryStateApplied      = "applied"
	RecoveryStateRestored     = "restored"
	RecoveryStateFailed       = "recovery_failed"
	BeforeStateAbsent         = "absent"
	BeforeStateInert          = "inert_unrestricted"
	BeforeStateManaged        = "managed"
	ProtectionStateUnmanaged  = "unmanaged"
	ProtectionStateLocked     = "locked"
	ProtectionStateUnlocked   = "unlocked"
	ProtectionStateAttention  = "attention"
	// PolicyMode 区分两种互斥的策略形态：
	//   whitelist —— DefaultLevel=Disallowed + 放行规则（默认全禁，白名单放行）；
	//   blacklist —— DefaultLevel=Unrestricted + level-0 拦截规则（默认全放行，仅拦截名单）。
	// 旧恢复记录没有该字段，缺省按 whitelist 处理。
	PolicyModeWhitelist = "whitelist"
	PolicyModeBlacklist = "blacklist"
)

type ApplyStatus struct {
	Available         bool          `json:"available"`
	CanApply          bool          `json:"canApply"`
	CanRestore        bool          `json:"canRestore"`
	IsAdmin           bool          `json:"isAdmin"`
	DomainJoined      bool          `json:"domainJoined"`
	SrpKeyExists      bool          `json:"srpKeyExists"`
	SrpDisposition    string        `json:"srpDisposition"`
	WillReplaceLegacy bool          `json:"willReplaceLegacy"`
	Active            bool          `json:"active"`
	PolicyName        string        `json:"policyName"`
	Fingerprint       string        `json:"fingerprint"`
	ReasonCode        string        `json:"reasonCode"`
	Reason            string        `json:"reason"`
	Warnings          []string      `json:"warnings"`
	RecoveryPath      string        `json:"recoveryPath"`
	CheckedAt         string        `json:"checkedAt"`
	ProtectionState   string        `json:"protectionState"`
	PolicyMode        string        `json:"policyMode"`
	CanTakeOver       bool          `json:"canTakeOver"`
	CanUnlock         bool          `json:"canUnlock"`
	CanLock           bool          `json:"canLock"`
	ExistingRuleCount int           `json:"existingRuleCount"`
	BackupCreatedAt   string        `json:"backupCreatedAt"`
	ManagedRules      []ManagedRule `json:"managedRules"`
}

type ApplyResult struct {
	Applied     bool   `json:"applied"`
	PolicyName  string `json:"policyName"`
	RuleCount   int    `json:"ruleCount"`
	Fingerprint string `json:"fingerprint"`
	AppliedAt   string `json:"appliedAt"`
	Message     string `json:"message"`
}

type RestoreResult struct {
	Restored   bool   `json:"restored"`
	PolicyName string `json:"policyName"`
	RestoredAt string `json:"restoredAt"`
	Message    string `json:"message"`
}

// ProtectionResult reports a successful lock or temporary-unlock transition.
type ProtectionResult struct {
	ProtectionState string `json:"protectionState"`
	ChangedAt       string `json:"changedAt"`
	Message         string `json:"message"`
}

// ManagedRule is one rule currently owned by XCLing.
type ManagedRule struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Description string `json:"description"`
	Removable   bool   `json:"removable"`
}

// RuleChangeResult reports an immediately applied managed-rule change.
type RuleChangeResult struct {
	ProtectionState string      `json:"protectionState"`
	Rule            ManagedRule `json:"rule"`
	RuleCount       int         `json:"ruleCount"`
	ChangedAt       string      `json:"changedAt"`
	Message         string      `json:"message"`
}

type RecoveryRecord struct {
	SchemaVersion string         `json:"schemaVersion"`
	AppVersion    string         `json:"appVersion"`
	ID            string         `json:"id"`
	PolicyName    string         `json:"policyName"`
	RuleCount     int            `json:"ruleCount"`
	RulesDigest   string         `json:"rulesDigest"`
	DefaultLevel  int            `json:"defaultLevel"`
	PolicyScope   int            `json:"policyScope"`
	Transparent   int            `json:"transparentEnabled"`
	Rules         []RecoveryRule `json:"rules"`
	// BlockRules 是 level-0 显式拦截规则；不持久化会导致重启后期望计划与
	// 注册表实际状态指纹漂移，被误判为外部修改。
	BlockRules []RecoveryRule `json:"blockRules,omitempty"`
	// PolicyMode 见 PolicyModeWhitelist/PolicyModeBlacklist；空值按 whitelist。
	PolicyMode         string               `json:"policyMode,omitempty"`
	Fingerprint        string               `json:"fingerprint"`
	BeforeState        string               `json:"beforeState"`
	BeforeDefaultLevel int                  `json:"beforeDefaultLevel"`
	BeforeSnapshot     RegistryTreeSnapshot `json:"beforeSnapshot"`
	ProtectionState    string               `json:"protectionState"`
	State              string               `json:"state"`
	CreatedAt          string               `json:"createdAt"`
	AppliedAt          string               `json:"appliedAt"`
	RestoredAt         string               `json:"restoredAt"`
	LastErrorCode      string               `json:"lastErrorCode"`
	LastDiagnostic     string               `json:"lastDiagnostic"`
	LastStateChangedAt string               `json:"lastStateChangedAt"`
}

type RecoveryRule struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Description string `json:"description"`
	Level       int    `json:"level"`
}
