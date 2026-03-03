package lint

import (
	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

// LintOptions configures a lint run.
type LintOptions struct {
	Tiers      []int
	SkillsPath string
	ConfigPath string
	Filename   string
}

// LintRecipe lints raw recipe JSON bytes and returns diagnostics.
func LintRecipe(data []byte, opts LintOptions) ([]LintDiagnostic, error) {
	// 1. Load config
	cfg, err := LoadConfig(opts.ConfigPath)
	if err != nil {
		return nil, err
	}

	// 2. Check ShouldIgnoreFile
	if cfg != nil && cfg.ShouldIgnoreFile(opts.Filename) {
		return nil, nil
	}

	// Determine which tiers to run (empty = all)
	tierSet := make(map[int]bool)
	if len(opts.Tiers) == 0 {
		tierSet[0] = true
		tierSet[1] = true
	} else {
		for _, t := range opts.Tiers {
			tierSet[t] = true
		}
	}

	var diags []LintDiagnostic

	// 3. Run Tier 0 if requested
	if tierSet[0] {
		tier0Diags := lintTier0(data)
		diags = append(diags, tier0Diags...)

		// 4. If tier 0 has errors, stop
		hasErrors := false
		for _, d := range tier0Diags {
			if d.Level == LevelError {
				hasErrors = true
				break
			}
		}
		if hasErrors {
			diags = applyCfgOverrides(diags, cfg)
			return filterOff(diags, cfg), nil
		}
	}

	// 5. Parse recipe
	parsed, err := recipe.Parse(data)
	if err != nil {
		return diags, err
	}

	// 6. Load connector rules
	connRules, err := LoadConnectorRules(opts.SkillsPath)
	if err != nil {
		return diags, err
	}

	// 7. Run Tier 1 if requested
	if tierSet[1] {
		tier1Diags := lintTier1Steps(parsed, opts.Filename, connRules)
		diags = append(diags, tier1Diags...)
	}

	// 8. Apply config severity overrides
	diags = applyCfgOverrides(diags, cfg)

	// 9. Filter out "off" rules
	diags = filterOff(diags, cfg)

	// 10. Return
	return diags, nil
}

// applyCfgOverrides applies config severity overrides to diagnostics.
func applyCfgOverrides(diags []LintDiagnostic, cfg *LintConfig) []LintDiagnostic {
	if cfg == nil {
		return diags
	}
	for i := range diags {
		eff := cfg.EffectiveSeverity(diags[i].RuleID, diags[i].Level)
		diags[i].Level = eff
	}
	return diags
}

// filterOff removes diagnostics whose level is "off".
func filterOff(diags []LintDiagnostic, cfg *LintConfig) []LintDiagnostic {
	if cfg == nil {
		return diags
	}
	var filtered []LintDiagnostic
	for _, d := range diags {
		if d.Level != "off" {
			filtered = append(filtered, d)
		}
	}
	return filtered
}
