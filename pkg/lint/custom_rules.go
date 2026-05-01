package lint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CustomRule defines a user-authored lint rule evaluated by the tier engine.
type CustomRule struct {
	RuleID       string        `json:"rule_id"`
	Tier         int           `json:"tier"`
	Level        string        `json:"level"`
	Message      string        `json:"message"`
	SuggestedFix string        `json:"suggested_fix,omitempty"`
	Scope        string        `json:"scope"` // "recipe" or "step"
	Where        *StepSelector `json:"where,omitempty"`
	Assert       Assertion     `json:"assert"`
}

// StepSelector filters which steps a step-scoped rule applies to.
type StepSelector struct {
	Keyword    StringOrArray `json:"keyword,omitempty"`
	Provider   StringOrArray `json:"provider,omitempty"`
	ActionName StringOrArray `json:"action_name,omitempty"`
}

// Assertion is a composable matcher. Exactly one field should be populated.
type Assertion struct {
	FieldExists  *AssertFieldPath    `json:"field_exists,omitempty"`
	FieldAbsent  *AssertFieldPath    `json:"field_absent,omitempty"`
	FieldMatches *AssertFieldMatches `json:"field_matches,omitempty"`
	FieldEquals  *AssertFieldEquals  `json:"field_equals,omitempty"`
	StepCount    *AssertStepCount    `json:"step_count,omitempty"`
	EISEmpty     *bool               `json:"eis_empty,omitempty"`
	EISFieldType *AssertEISFieldType `json:"eis_field_type,omitempty"`
	AllOf        []Assertion         `json:"all_of,omitempty"`
	AnyOf        []Assertion         `json:"any_of,omitempty"`
	Not          *Assertion          `json:"not,omitempty"`
	Builtin      *string             `json:"builtin,omitempty"`
}

// AssertFieldPath asserts a field path exists (or is absent).
type AssertFieldPath struct {
	Path string `json:"path"`
}

// AssertFieldMatches asserts a field value matches a regex pattern.
type AssertFieldMatches struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern"`
}

// AssertFieldEquals asserts a field value equals a literal.
type AssertFieldEquals struct {
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

// AssertStepCount asserts the number of steps matching a selector.
type AssertStepCount struct {
	Where *StepSelector `json:"where,omitempty"`
	Min   *int          `json:"min,omitempty"`
	Max   *int          `json:"max,omitempty"`
	Exact *int          `json:"exact,omitempty"`
}

// AssertEISFieldType asserts an EIS field has the expected type/parse_output.
type AssertEISFieldType struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	ParseOutput string `json:"parse_output,omitempty"`
}

// StringOrArray unmarshals either a single string or an array of strings.
type StringOrArray []string

func (s *StringOrArray) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*s = []string{single}
		return nil
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("expected string or []string: %w", err)
	}
	*s = arr
	return nil
}

func (s StringOrArray) Contains(val string) bool {
	for _, v := range s {
		if v == val {
			return true
		}
	}
	return false
}

// ruleFile is the on-disk format for a rules JSON file (v0.2.0+).
type ruleFile struct {
	Version string       `json:"version"`
	Rules   []CustomRule `json:"rules"`
}

func validateCustomRule(r CustomRule) error {
	if r.RuleID == "" {
		return fmt.Errorf("missing rule_id")
	}
	if r.Scope != "recipe" && r.Scope != "step" {
		return fmt.Errorf("scope must be \"recipe\" or \"step\"")
	}
	if r.Level != LevelError && r.Level != LevelWarn && r.Level != LevelInfo {
		return fmt.Errorf("level must be \"error\", \"warn\", or \"info\"")
	}
	return validateAssertion(r.Assert)
}

func validateAssertion(a Assertion) error {
	count := 0
	if a.FieldExists != nil {
		count++
	}
	if a.FieldAbsent != nil {
		count++
	}
	if a.FieldMatches != nil {
		count++
	}
	if a.FieldEquals != nil {
		count++
	}
	if a.StepCount != nil {
		count++
	}
	if a.EISEmpty != nil {
		count++
	}
	if a.EISFieldType != nil {
		count++
	}
	if len(a.AllOf) > 0 {
		count++
	}
	if len(a.AnyOf) > 0 {
		count++
	}
	if a.Not != nil {
		count++
	}
	if a.Builtin != nil {
		count++
	}

	if count == 0 {
		return fmt.Errorf("no recognized assertion type (check for typos in assertion keys)")
	}
	if count > 1 {
		return fmt.Errorf("assertion must have exactly one matcher, found %d (use all_of to combine)", count)
	}

	if a.FieldMatches != nil {
		if _, err := regexp.Compile(a.FieldMatches.Pattern); err != nil {
			return fmt.Errorf("field_matches pattern %q is not a valid regex: %w", a.FieldMatches.Pattern, err)
		}
	}

	for i, sub := range a.AllOf {
		if err := validateAssertion(sub); err != nil {
			return fmt.Errorf("all_of[%d]: %w", i, err)
		}
	}
	for i, sub := range a.AnyOf {
		if err := validateAssertion(sub); err != nil {
			return fmt.Errorf("any_of[%d]: %w", i, err)
		}
	}
	if a.Not != nil {
		if err := validateAssertion(*a.Not); err != nil {
			return fmt.Errorf("not: %w", err)
		}
	}

	return nil
}

// LoadCustomRules loads custom rules from both the skills directory and the
// project-level .wklint/rules/ directory. Skills rules are loaded first,
// project rules are appended after. Validation warnings are returned as
// diagnostics so they flow through the normal severity pipeline.
func LoadCustomRules(skillsPath, projectRoot string) ([]CustomRule, []LintDiagnostic, error) {
	var rules []CustomRule
	var warnings []LintDiagnostic

	// Layer 1: skills directory lint-rules.json files with v0.2.0+ rules array
	if skillsPath != "" {
		skillRules, w, err := loadCustomRulesFromSkills(skillsPath)
		if err != nil {
			return nil, nil, fmt.Errorf("loading skills rules: %w", err)
		}
		rules = append(rules, skillRules...)
		warnings = append(warnings, w...)
	}

	// Layer 2: project-level .wklint/rules/*.json
	if projectRoot != "" {
		rulesDir := filepath.Join(projectRoot, ".wklint", "rules")
		projectRules, w, err := loadCustomRulesFromDir(rulesDir)
		if err != nil {
			return nil, nil, fmt.Errorf("loading project rules: %w", err)
		}
		rules = append(rules, projectRules...)
		warnings = append(warnings, w...)
	}

	return rules, warnings, nil
}

func loadCustomRulesFromSkills(skillsPath string) ([]CustomRule, []LintDiagnostic, error) {
	var rules []CustomRule
	var warnings []LintDiagnostic

	info, err := os.Stat(skillsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return rules, nil, nil
		}
		return nil, nil, err
	}
	if !info.IsDir() {
		return rules, nil, nil
	}

	err = filepath.Walk(skillsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Name() != "lint-rules.json" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			warnings = append(warnings, LintDiagnostic{
				Level:   LevelWarn,
				RuleID:  "CUSTOM_RULE_INVALID",
				Message: fmt.Sprintf("cannot read %s: %v", path, readErr),
			})
			return nil
		}
		fileRules, fileWarnings := parseRuleFile(data, path)
		rules = append(rules, fileRules...)
		warnings = append(warnings, fileWarnings...)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return rules, warnings, nil
}

func loadCustomRulesFromDir(dir string) ([]CustomRule, []LintDiagnostic, error) {
	var rules []CustomRule
	var warnings []LintDiagnostic

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return rules, nil, nil
		}
		return nil, nil, err
	}
	if !info.IsDir() {
		return rules, nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			warnings = append(warnings, LintDiagnostic{
				Level:   LevelWarn,
				RuleID:  "CUSTOM_RULE_INVALID",
				Message: fmt.Sprintf("cannot read %s: %v", path, readErr),
			})
			continue
		}
		fileRules, fileWarnings := parseRuleFile(data, path)
		rules = append(rules, fileRules...)
		warnings = append(warnings, fileWarnings...)
	}

	return rules, warnings, nil
}

func parseRuleFile(data []byte, sourcePath string) ([]CustomRule, []LintDiagnostic) {
	var f ruleFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, []LintDiagnostic{{
			Level:   LevelWarn,
			RuleID:  "CUSTOM_RULE_INVALID",
			Message: fmt.Sprintf("cannot parse %s: %v", sourcePath, err),
		}}
	}
	if len(f.Rules) == 0 {
		return nil, nil
	}

	var valid []CustomRule
	var warnings []LintDiagnostic
	for _, r := range f.Rules {
		if err := validateCustomRule(r); err != nil {
			warnings = append(warnings, LintDiagnostic{
				Level:   LevelWarn,
				RuleID:  "CUSTOM_RULE_INVALID",
				Message: fmt.Sprintf("rule %q in %s: %v", r.RuleID, sourcePath, err),
			})
			continue
		}
		valid = append(valid, r)
	}
	return valid, warnings
}
