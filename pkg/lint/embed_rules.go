package lint

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed builtin_rules.json
var builtinRulesJSON []byte

func loadBuiltinRules() ([]CustomRule, error) {
	var f ruleFile
	if err := json.Unmarshal(builtinRulesJSON, &f); err != nil {
		return nil, fmt.Errorf("parsing builtin rules: %w", err)
	}
	for _, r := range f.Rules {
		if err := validateCustomRule(r); err != nil {
			return nil, fmt.Errorf("builtin rule %q: %w", r.RuleID, err)
		}
	}
	return f.Rules, nil
}
