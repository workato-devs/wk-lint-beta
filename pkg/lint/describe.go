package lint

import "path/filepath"

// RuleDescription describes a single rule in the catalog.
type RuleDescription struct {
	RuleID       string `json:"rule_id"`
	Tier         int    `json:"tier"`
	DefaultLevel string `json:"default_level"`
	Message      string `json:"message"`
	SuggestedFix string `json:"suggested_fix,omitempty"`
	Scope        string `json:"scope"`
	Source       string `json:"source"`
}

// RuleCatalog is the result of DescribeRules.
type RuleCatalog struct {
	Rules []RuleDescription `json:"rules"`
}

// DescribeOptions configures which rule sources to include.
type DescribeOptions struct {
	SkillsPath string
	ConfigPath string
}

// DescribeRules returns the full rule catalog from all configured sources.
func DescribeRules(opts DescribeOptions) (*RuleCatalog, error) {
	catalog := &RuleCatalog{}

	builtinRules, err := loadBuiltinRules()
	if err != nil {
		return nil, err
	}
	for _, r := range builtinRules {
		catalog.Rules = append(catalog.Rules, RuleDescription{
			RuleID:       r.RuleID,
			Tier:         r.Tier,
			DefaultLevel: r.Level,
			Message:      r.Message,
			SuggestedFix: r.SuggestedFix,
			Scope:        r.Scope,
			Source:       "builtin",
		})
	}

	projectRoot := ""
	if opts.ConfigPath != "" {
		projectRoot = filepath.Dir(opts.ConfigPath)
	}

	customRules, _, err := LoadCustomRules(opts.SkillsPath, projectRoot)
	if err != nil {
		return nil, err
	}

	connRules, err := LoadConnectorRules(opts.SkillsPath)
	if err != nil {
		return nil, err
	}
	for _, cr := range connRules {
		customRules = append(customRules, cr.Rules...)
	}

	for _, r := range customRules {
		source := "project"
		if opts.SkillsPath != "" {
			source = "custom"
		}
		catalog.Rules = append(catalog.Rules, RuleDescription{
			RuleID:       r.RuleID,
			Tier:         r.Tier,
			DefaultLevel: r.Level,
			Message:      r.Message,
			SuggestedFix: r.SuggestedFix,
			Scope:        r.Scope,
			Source:       source,
		})
	}

	return catalog, nil
}
