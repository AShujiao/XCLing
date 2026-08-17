package model

const (
	LevelDisallowed   = "disallowed"
	LevelUnrestricted = "unrestricted"
)

const (
	ActionAllow    = "allow"
	ActionDisallow = "disallow"
)

type PathRule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Action      string `json:"action"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type Policy struct {
	Version      string     `json:"version"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	DefaultLevel string     `json:"defaultLevel"`
	Rules        []PathRule `json:"rules"`
	UpdatedAt    string     `json:"updatedAt"`
	AdminBypass  bool       `json:"adminBypass"`
}

type ValidationIssue struct {
	RuleID  string `json:"ruleId"`
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ValidationResult struct {
	Valid     bool              `json:"valid"`
	RuleCount int               `json:"ruleCount"`
	Errors    []ValidationIssue `json:"errors"`
	Warnings  []ValidationIssue `json:"warnings"`
}

type SimulationResult struct {
	Path           string `json:"path"`
	Decision       string `json:"decision"`
	MatchedRuleID  string `json:"matchedRuleId"`
	MatchedRule    string `json:"matchedRule"`
	MatchedPattern string `json:"matchedPattern"`
	Specificity    int    `json:"specificity"`
	DefaultUsed    bool   `json:"defaultUsed"`
	Reason         string `json:"reason"`
}

type ConflictWarning struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

const (
	WarnInfo    = "info"
	WarnWarning = "warning"
	WarnDanger  = "danger"
)
