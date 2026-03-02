package lint

// LintDiagnostic represents a single lint finding.
type LintDiagnostic struct {
	Level   string     `json:"level"`
	Message string     `json:"message"`
	Source  *SourceRef `json:"source,omitempty"`
	RuleID  string     `json:"rule_id"`
	Tier    int        `json:"tier"`
}

// SourceRef points to a location in the recipe JSON.
type SourceRef struct {
	JSONPointer string `json:"json_pointer"`
}

const (
	LevelError = "error"
	LevelWarn  = "warn"
	LevelInfo  = "info"
)
