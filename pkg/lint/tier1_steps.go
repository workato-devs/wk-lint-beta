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

// checkActionRules evaluates legacy v0.1.0 ActionRules from connector rules.
func checkActionRules(parsed *recipe.ParsedRecipe, connRules map[string]*ConnectorRules) []LintDiagnostic {
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
		for _, rule := range cr.ActionRules {
			if !actionRuleApplies(step.Code.Name, rule.ActionNames) {
				continue
			}
			diags = append(diags, evalRequireFields(step, rule)...)
			diags = append(diags, evalActionEISMustBeEmpty(step, rule)...)
			diags = append(diags, evalFieldTypeChecks(step, rule)...)
		}
	}
	return diags
}

func actionRuleApplies(name string, actionNames []string) bool {
	for _, an := range actionNames {
		if an == name {
			return true
		}
	}
	return false
}

func evalRequireFields(step recipe.FlatStep, rule ActionRule) []LintDiagnostic {
	if len(rule.RequireFields) == 0 {
		return nil
	}
	locations := rule.RequireIn
	if len(locations) == 0 {
		locations = []string{"input"}
	}
	var diags []LintDiagnostic
	for _, field := range rule.RequireFields {
		for _, loc := range locations {
			raw := resolveStepLocation(step, loc)
			if len(raw) == 0 {
				msg := expandMessage(rule.Message, map[string]string{
					"missing_location": loc,
					"field_name":       field,
				})
				diags = append(diags, LintDiagnostic{
					Level:   LevelWarn,
					Message: msg,
					Source:  &SourceRef{JSONPointer: step.JSONPointer + "/" + loc},
					RuleID:  rule.RuleID,
					Tier:    1,
				})
				continue
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			if _, ok := m[field]; !ok {
				msg := expandMessage(rule.Message, map[string]string{
					"missing_location": loc,
					"field_name":       field,
				})
				diags = append(diags, LintDiagnostic{
					Level:   LevelWarn,
					Message: msg,
					Source:  &SourceRef{JSONPointer: step.JSONPointer + "/" + loc},
					RuleID:  rule.RuleID,
					Tier:    1,
				})
			}
		}
	}
	return diags
}

func evalActionEISMustBeEmpty(step recipe.FlatStep, rule ActionRule) []LintDiagnostic {
	if !rule.EISMustBeEmpty {
		return nil
	}
	raw := step.Code.ExtendedInputSchema
	if len(raw) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "null" || trimmed == "[]" {
		return nil
	}
	return []LintDiagnostic{{
		Level:   LevelWarn,
		Message: rule.Message,
		Source:  &SourceRef{JSONPointer: step.JSONPointer + "/extended_input_schema"},
		RuleID:  rule.RuleID,
		Tier:    1,
	}}
}

func evalFieldTypeChecks(step recipe.FlatStep, rule ActionRule) []LintDiagnostic {
	if len(rule.FieldTypeChecks) == 0 {
		return nil
	}
	eisFields, err := parseEIS(step.Code.ExtendedInputSchema)
	if err != nil {
		return nil
	}
	eisByName := make(map[string]EISField)
	for _, f := range eisFields {
		eisByName[f.Name] = f
	}
	var diags []LintDiagnostic
	for fieldName, check := range rule.FieldTypeChecks {
		ef, ok := eisByName[fieldName]
		if !ok {
			continue
		}
		mismatch := false
		if check.Type != "" && ef.Type != check.Type {
			mismatch = true
		}
		if check.ParseOutput != "" && ef.ParseOutput != check.ParseOutput {
			mismatch = true
		}
		if mismatch {
			msg := expandMessage(rule.Message, map[string]string{
				"field_name": fieldName,
			})
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: msg,
				Source:  &SourceRef{JSONPointer: step.JSONPointer + "/extended_input_schema"},
				RuleID:  rule.RuleID,
				Tier:    1,
			})
		}
	}
	return diags
}

func resolveStepLocation(step recipe.FlatStep, location string) json.RawMessage {
	switch location {
	case "input":
		return step.Code.Input
	case "dynamicPickListSelection":
		return step.Code.DynamicPickListSelection
	default:
		return nil
	}
}
