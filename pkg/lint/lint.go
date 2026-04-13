package lint

import (
	"path/filepath"

	"github.com/workato-devs/wk-lint-beta/pkg/igm"
	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

// LintOptions configures a lint run.
type LintOptions struct {
	Tiers      []int
	SkillsPath string
	ConfigPath string
	Filename   string
	Profile    string
	PluginDir  string
}

// LintRecipe lints raw recipe JSON bytes and returns diagnostics.
func LintRecipe(data []byte, opts LintOptions) ([]LintDiagnostic, error) {
	// 1. Load config
	cfg, err := LoadConfig(opts.ConfigPath)
	if err != nil {
		return nil, err
	}

	// 2. Resolve profile (CLI flag > config file > none)
	profileName := opts.Profile
	if profileName == "" && cfg != nil {
		profileName = cfg.Profile
	}

	var resolvedProfile *ResolvedProfile
	if profileName != "" {
		projectRoot := ""
		if opts.ConfigPath != "" {
			projectRoot = filepath.Dir(opts.ConfigPath)
		}
		discovered, err := discoverProfiles(projectRoot, opts.PluginDir)
		if err != nil {
			return nil, err
		}
		resolvedProfile, err = resolveProfileChain(profileName, discovered)
		if err != nil {
			return nil, err
		}
	}

	// 3. Check ShouldIgnoreFile
	if cfg != nil && cfg.ShouldIgnoreFile(opts.Filename) {
		return nil, nil
	}

	// Determine which tiers to run (empty = all)
	tierSet := make(map[int]bool)
	if len(opts.Tiers) == 0 {
		tierSet[0] = true
		tierSet[1] = true
		tierSet[2] = true
		tierSet[3] = true
	} else {
		for _, t := range opts.Tiers {
			tierSet[t] = true
		}
	}

	var diags []LintDiagnostic

	// 4. Run Tier 0 if requested
	if tierSet[0] {
		tier0Diags := lintTier0(data)
		diags = append(diags, tier0Diags...)

		// 5. If tier 0 has errors, stop
		hasErrors := false
		for _, d := range tier0Diags {
			if d.Level == LevelError {
				hasErrors = true
				break
			}
		}
		if hasErrors {
			diags = applyOverrides(diags, resolvedProfile, cfg)
			return filterOff(diags), nil
		}
	}

	// 6. Parse recipe
	parsed, err := recipe.Parse(data)
	if err != nil {
		return diags, err
	}

	// 7. Load connector rules
	connRules, err := LoadConnectorRules(opts.SkillsPath)
	if err != nil {
		return diags, err
	}

	// 8. Run Tier 1 if requested
	if tierSet[1] {
		tier1Diags := lintTier1Steps(parsed, opts.Filename, connRules)
		diags = append(diags, tier1Diags...)
	}

	// 9. Build IGM graph for Tier 2-3 if needed
	var graph *igm.Graph
	if tierSet[2] || tierSet[3] {
		var igmErr error
		graph, igmErr = igm.Transform(data)
		if igmErr != nil {
			// IGM build failure is non-fatal; skip Tier 2-3
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: "IGM graph build failed; skipping Tier 2-3 rules: " + igmErr.Error(),
				RuleID:  "IGM_BUILD_FAILED",
				Tier:    2,
			})
		}
	}

	// 10. Run Tier 2 if requested
	if tierSet[2] && graph != nil {
		tier2Diags := lintTier2Structure(graph, parsed)
		diags = append(diags, tier2Diags...)
	}

	// 11. Run Tier 3 if requested
	if tierSet[3] && graph != nil {
		tier3Diags := lintTier3DataFlow(parsed, graph)
		diags = append(diags, tier3Diags...)
	}

	// 12. Apply overrides (profile first, then config)
	diags = applyOverrides(diags, resolvedProfile, cfg)

	// 13. Filter out "off" rules
	diags = filterOff(diags)

	return diags, nil
}

// applyOverrides applies severity overrides in order: profile layer first,
// then .wklintrc.json layer (config always wins over profile).
func applyOverrides(diags []LintDiagnostic, profile *ResolvedProfile, cfg *LintConfig) []LintDiagnostic {
	for i := range diags {
		level := diags[i].Level

		// Layer 1: profile overrides
		if profile != nil {
			if v, ok := profile.Rules[diags[i].RuleID]; ok {
				level = v
			}
		}

		// Layer 2: config overrides (highest priority)
		if cfg != nil {
			if v, ok := cfg.Rules[diags[i].RuleID]; ok {
				level = v
			}
		}

		diags[i].Level = level
	}
	return diags
}

// filterOff removes diagnostics whose level is "off".
func filterOff(diags []LintDiagnostic) []LintDiagnostic {
	var filtered []LintDiagnostic
	for _, d := range diags {
		if d.Level != "off" {
			filtered = append(filtered, d)
		}
	}
	if filtered == nil {
		return diags[:0]
	}
	return filtered
}
