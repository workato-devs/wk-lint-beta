package lint

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

// internalProviders are providers that should be skipped for CONFIG_PROVIDER_MATCH.
var internalProviders = map[string]bool{
	"workato_recipe_function": true,
	"logger":                  true,
}

// uuidV4Regex matches standard UUID v4 format.
var uuidV4Regex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// lintTier1Steps runs Tier 1 step-level lint rules on a parsed recipe.
func lintTier1Steps(parsed *recipe.ParsedRecipe, filename string, connRules map[string]*ConnectorRules) []LintDiagnostic {
	var diags []LintDiagnostic

	diags = append(diags, checkStepNumbering(parsed)...)
	diags = append(diags, checkUUIDUnique(parsed)...)
	diags = append(diags, checkUUIDDescriptive(parsed)...)
	diags = append(diags, checkTriggerNumberZero(parsed)...)
	diags = append(diags, checkFilenameMatch(parsed, filename)...)
	diags = append(diags, checkConfigNoWorkato(parsed)...)
	diags = append(diags, checkConfigProviderMatch(parsed, connRules)...)
	diags = append(diags, checkActionNameValid(parsed, connRules)...)
	diags = append(diags, checkControlFlowRules(parsed)...)
	diags = append(diags, checkNoElsif(parsed)...)
	diags = append(diags, checkResponseCodesDefined(parsed)...)
	diags = append(diags, checkFormulaMethods(parsed)...)
	diags = append(diags, checkDatapillsWithCatchAliases(parsed, connRules)...)
	diags = append(diags, checkEIS(parsed, connRules)...)

	return diags
}

// checkStepNumbering verifies steps are numbered 0,1,2,... sequentially.
func checkStepNumbering(parsed *recipe.ParsedRecipe) []LintDiagnostic {
	var diags []LintDiagnostic
	for i, step := range parsed.Steps {
		if step.Code.Number == nil {
			continue
		}
		if *step.Code.Number != i {
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: fmt.Sprintf("Step numbering: expected %d but got %d", i, *step.Code.Number),
				Source:  &SourceRef{JSONPointer: step.JSONPointer + "/number"},
				RuleID:  "STEP_NUMBERING",
				Tier:    1,
			})
		}
	}
	return diags
}

// checkUUIDUnique verifies no duplicate UUIDs.
func checkUUIDUnique(parsed *recipe.ParsedRecipe) []LintDiagnostic {
	var diags []LintDiagnostic
	seen := make(map[string]string) // uuid -> first pointer
	for _, step := range parsed.Steps {
		if step.Code.UUID == "" {
			continue
		}
		if firstPtr, ok := seen[step.Code.UUID]; ok {
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: fmt.Sprintf("Duplicate UUID %q (first seen at %s)", step.Code.UUID, firstPtr),
				Source:  &SourceRef{JSONPointer: step.JSONPointer + "/uuid"},
				RuleID:  "UUID_UNIQUE",
				Tier:    1,
			})
		} else {
			seen[step.Code.UUID] = step.JSONPointer
		}
	}
	return diags
}

// checkUUIDDescriptive warns on UUIDs matching standard UUID v4 format.
func checkUUIDDescriptive(parsed *recipe.ParsedRecipe) []LintDiagnostic {
	var diags []LintDiagnostic
	for _, step := range parsed.Steps {
		if step.Code.UUID == "" {
			continue
		}
		if uuidV4Regex.MatchString(step.Code.UUID) {
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: fmt.Sprintf("UUID %q looks like a standard UUID v4; consider using a descriptive name", step.Code.UUID),
				Source:  &SourceRef{JSONPointer: step.JSONPointer + "/uuid"},
				RuleID:  "UUID_DESCRIPTIVE",
				Tier:    1,
			})
		}
	}
	return diags
}

// checkTriggerNumberZero checks that the first step (trigger) has number 0.
func checkTriggerNumberZero(parsed *recipe.ParsedRecipe) []LintDiagnostic {
	var diags []LintDiagnostic
	if len(parsed.Steps) == 0 {
		return diags
	}
	trigger := parsed.Steps[0]
	if trigger.Code.Keyword != "trigger" {
		return diags
	}
	if trigger.Code.Number == nil || *trigger.Code.Number != 0 {
		num := "<nil>"
		if trigger.Code.Number != nil {
			num = fmt.Sprintf("%d", *trigger.Code.Number)
		}
		diags = append(diags, LintDiagnostic{
			Level:   LevelError,
			Message: fmt.Sprintf("Trigger step must have number 0, got %s", num),
			Source:  &SourceRef{JSONPointer: trigger.JSONPointer + "/number"},
			RuleID:  "TRIGGER_NUMBER_ZERO",
			Tier:    1,
		})
	}
	return diags
}

// checkFilenameMatch checks that recipe name matches filename.
func checkFilenameMatch(parsed *recipe.ParsedRecipe, filename string) []LintDiagnostic {
	var diags []LintDiagnostic
	if filename == "" {
		return diags
	}
	if parsed.Raw.Name == "" {
		return diags
	}

	// Get filename stem: remove directory and .recipe.json extension
	base := filepath.Base(filename)
	stem := strings.TrimSuffix(base, ".recipe.json")
	if stem == base {
		// Try just .json
		stem = strings.TrimSuffix(base, ".json")
	}

	// Convert recipe name: lowercase and spaces to underscores
	expected := strings.ReplaceAll(strings.ToLower(parsed.Raw.Name), " ", "_")

	if stem != expected {
		diags = append(diags, LintDiagnostic{
			Level:   LevelWarn,
			Message: fmt.Sprintf("Recipe name %q (normalized: %q) does not match filename stem %q", parsed.Raw.Name, expected, stem),
			Source:  &SourceRef{JSONPointer: "/name"},
			RuleID:  "FILENAME_MATCH",
			Tier:    1,
		})
	}
	return diags
}

// checkConfigNoWorkato warns if "workato" is listed as a config provider.
func checkConfigNoWorkato(parsed *recipe.ParsedRecipe) []LintDiagnostic {
	var diags []LintDiagnostic
	for i, cfg := range parsed.Config {
		if cfg.Provider == "workato" {
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: "Config should not list \"workato\" as a provider",
				Source:  &SourceRef{JSONPointer: fmt.Sprintf("/config/%d/provider", i)},
				RuleID:  "CONFIG_NO_WORKATO",
				Tier:    1,
			})
		}
	}
	return diags
}

// checkConfigProviderMatch verifies that step providers appear in config.
func checkConfigProviderMatch(parsed *recipe.ParsedRecipe, connRules map[string]*ConnectorRules) []LintDiagnostic {
	var diags []LintDiagnostic

	// Build set of config providers
	configProviders := make(map[string]bool)
	for _, p := range parsed.Providers {
		configProviders[p] = true
	}

	// Build set of all internal providers (from internal list + connector rules)
	internals := make(map[string]bool)
	for k, v := range internalProviders {
		internals[k] = v
	}
	for _, cr := range connRules {
		for _, ci := range cr.ConnectorInternals {
			internals[ci] = true
		}
	}

	// Check each step
	for _, step := range parsed.Steps {
		if step.Code.Provider == nil {
			continue
		}
		provider := *step.Code.Provider
		if provider == "" {
			continue
		}
		if internals[provider] {
			continue
		}
		if !configProviders[provider] {
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: fmt.Sprintf("Step provider %q not found in config", provider),
				Source:  &SourceRef{JSONPointer: step.JSONPointer + "/provider"},
				RuleID:  "CONFIG_PROVIDER_MATCH",
				Tier:    1,
			})
		}
	}
	return diags
}

// checkControlFlowRules checks if/else/catch/try keyword rules.
func checkControlFlowRules(parsed *recipe.ParsedRecipe) []LintDiagnostic {
	var diags []LintDiagnostic

	for _, step := range parsed.Steps {
		switch step.Code.Keyword {
		case "if":
			// IF_NO_PROVIDER: if keyword -> provider should be nil
			if step.Code.Provider != nil {
				diags = append(diags, LintDiagnostic{
					Level:   LevelWarn,
					Message: "\"if\" step should not have a provider",
					Source:  &SourceRef{JSONPointer: step.JSONPointer + "/provider"},
					RuleID:  "IF_NO_PROVIDER",
					Tier:    1,
				})
			}

		case "else":
			// ELSE_NO_PROVIDER: else keyword -> provider should be nil
			if step.Code.Provider != nil {
				diags = append(diags, LintDiagnostic{
					Level:   LevelWarn,
					Message: "\"else\" step should not have a provider",
					Source:  &SourceRef{JSONPointer: step.JSONPointer + "/provider"},
					RuleID:  "ELSE_NO_PROVIDER",
					Tier:    1,
				})
			}

		case "catch":
			// CATCH_PROVIDER_NULL: catch keyword -> provider should be nil
			if step.Code.Provider != nil {
				diags = append(diags, LintDiagnostic{
					Level:   LevelWarn,
					Message: "\"catch\" step should not have a provider",
					Source:  &SourceRef{JSONPointer: step.JSONPointer + "/provider"},
					RuleID:  "CATCH_PROVIDER_NULL",
					Tier:    1,
				})
			}

			// CATCH_HAS_AS: catch keyword -> "as" should be non-empty
			if step.Code.As == "" {
				diags = append(diags, LintDiagnostic{
					Level:   LevelWarn,
					Message: "\"catch\" step should have a non-empty \"as\" field",
					Source:  &SourceRef{JSONPointer: step.JSONPointer + "/as"},
					RuleID:  "CATCH_HAS_AS",
					Tier:    1,
				})
			}

			// CATCH_HAS_RETRY: catch input should have max_retry_count
			diags = append(diags, checkCatchRetry(step)...)

		case "try":
			// TRY_NO_AS: try keyword -> "as" should be empty
			if step.Code.As != "" {
				diags = append(diags, LintDiagnostic{
					Level:   LevelWarn,
					Message: "\"try\" step should have an empty \"as\" field",
					Source:  &SourceRef{JSONPointer: step.JSONPointer + "/as"},
					RuleID:  "TRY_NO_AS",
					Tier:    1,
				})
			}
		}
	}
	return diags
}

// checkCatchRetry checks that a catch step's input contains max_retry_count.
func checkCatchRetry(step recipe.FlatStep) []LintDiagnostic {
	var diags []LintDiagnostic
	if step.Code.Input == nil {
		diags = append(diags, LintDiagnostic{
			Level:   LevelInfo,
			Message: "\"catch\" step input should have \"max_retry_count\"",
			Source:  &SourceRef{JSONPointer: step.JSONPointer + "/input"},
			RuleID:  "CATCH_HAS_RETRY",
			Tier:    1,
		})
		return diags
	}

	var inputMap map[string]json.RawMessage
	if err := json.Unmarshal(step.Code.Input, &inputMap); err != nil {
		return diags
	}
	if _, ok := inputMap["max_retry_count"]; !ok {
		diags = append(diags, LintDiagnostic{
			Level:   LevelInfo,
			Message: "\"catch\" step input should have \"max_retry_count\"",
			Source:  &SourceRef{JSONPointer: step.JSONPointer + "/input"},
			RuleID:  "CATCH_HAS_RETRY",
			Tier:    1,
		})
	}
	return diags
}

// checkNoElsif verifies no step uses the "elsif" keyword.
func checkNoElsif(parsed *recipe.ParsedRecipe) []LintDiagnostic {
	var diags []LintDiagnostic
	for _, step := range parsed.Steps {
		if step.Code.Keyword == "elsif" {
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: "\"elsif\" keyword is not allowed; use nested if/else instead",
				Source:  &SourceRef{JSONPointer: step.JSONPointer + "/keyword"},
				RuleID:  "NO_ELSIF",
				Tier:    1,
			})
		}
	}
	return diags
}

// checkActionNameValid verifies that action names are valid for their provider.
func checkActionNameValid(parsed *recipe.ParsedRecipe, connRules map[string]*ConnectorRules) []LintDiagnostic {
	var diags []LintDiagnostic
	if len(connRules) == 0 {
		return diags
	}
	for _, step := range parsed.Steps {
		if step.Code.Provider == nil || *step.Code.Provider == "" {
			continue
		}
		if step.Code.Name == "" {
			continue
		}
		provider := *step.Code.Provider
		cr, ok := connRules[provider]
		if !ok {
			continue
		}
		if len(cr.ValidActionNames) == 0 {
			continue
		}
		valid := false
		for _, name := range cr.ValidActionNames {
			if step.Code.Name == name {
				valid = true
				break
			}
		}
		if !valid {
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: fmt.Sprintf("Action name %q is not valid for provider %q; expected one of %v", step.Code.Name, provider, cr.ValidActionNames),
				Source:  &SourceRef{JSONPointer: step.JSONPointer + "/name"},
				RuleID:  "ACTION_NAME_VALID",
				Tier:    1,
			})
		}
	}
	return diags
}

// checkResponseCodesDefined checks that API platform triggers have response codes.
func checkResponseCodesDefined(parsed *recipe.ParsedRecipe) []LintDiagnostic {
	var diags []LintDiagnostic
	for _, step := range parsed.Steps {
		if step.Code.Keyword != "trigger" {
			continue
		}
		if step.Code.Provider == nil || *step.Code.Provider != "workato_api_platform" {
			continue
		}
		// Check input for response codes
		if step.Code.Input == nil {
			diags = append(diags, LintDiagnostic{
				Level:   LevelInfo,
				Message: "API platform trigger should define response codes in input",
				Source:  &SourceRef{JSONPointer: step.JSONPointer + "/input"},
				RuleID:  "RESPONSE_CODES_DEFINED",
				Tier:    1,
			})
			continue
		}
		var inputMap map[string]json.RawMessage
		if err := json.Unmarshal(step.Code.Input, &inputMap); err != nil {
			continue
		}
		if _, ok := inputMap["response_codes"]; !ok {
			diags = append(diags, LintDiagnostic{
				Level:   LevelInfo,
				Message: "API platform trigger should define response codes in input",
				Source:  &SourceRef{JSONPointer: step.JSONPointer + "/input"},
				RuleID:  "RESPONSE_CODES_DEFINED",
				Tier:    1,
			})
		}
	}
	return diags
}
