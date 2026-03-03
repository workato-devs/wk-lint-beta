package lint

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// lintTier0 performs Tier 0 schema validation on raw recipe JSON bytes.
func lintTier0(raw []byte) []LintDiagnostic {
	var diags []LintDiagnostic

	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		diags = append(diags, LintDiagnostic{
			Level:   LevelError,
			Message: "Recipe is not valid JSON: " + err.Error(),
			RuleID:  "INVALID_JSON",
			Tier:    0,
		})
		return diags
	}

	// CODE_WRAPPED_IN_RECIPE — top-level should not have a "recipe" wrapper
	if _, ok := top["recipe"]; ok {
		diags = append(diags, LintDiagnostic{
			Level:   LevelError,
			Message: "Top-level should not have a \"recipe\" key wrapping everything",
			Source:  &SourceRef{JSONPointer: "/recipe"},
			RuleID:  "CODE_WRAPPED_IN_RECIPE",
			Tier:    0,
		})
	}

	// MISSING_TOP_LEVEL_KEYS
	required := []string{"name", "version", "private", "concurrency", "code", "config"}
	for _, key := range required {
		if _, ok := top[key]; !ok {
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: fmt.Sprintf("Missing required top-level key: %q", key),
				Source:  &SourceRef{JSONPointer: "/"},
				RuleID:  "MISSING_TOP_LEVEL_KEYS",
				Tier:    0,
			})
		}
	}

	// CODE_NOT_OBJECT — code value must start with '{'
	if codeRaw, ok := top["code"]; ok {
		trimmed := bytes.TrimSpace(codeRaw)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: "\"code\" must be a JSON object",
				Source:  &SourceRef{JSONPointer: "/code"},
				RuleID:  "CODE_NOT_OBJECT",
				Tier:    0,
			})
		}
	}

	// CONFIG_INVALID — config must be an array of objects with keyword "application"
	if configRaw, ok := top["config"]; ok {
		diags = append(diags, validateConfig(configRaw)...)
	}

	// Walk steps inside code for step-level tier 0 checks
	if codeRaw, ok := top["code"]; ok {
		trimmed := bytes.TrimSpace(codeRaw)
		if len(trimmed) > 0 && trimmed[0] == '{' {
			diags = append(diags, walkStepsTier0(codeRaw, "/code")...)
		}
	}

	return diags
}

// validateConfig checks that config is an array of objects with keyword "application".
func validateConfig(raw json.RawMessage) []LintDiagnostic {
	var diags []LintDiagnostic

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		diags = append(diags, LintDiagnostic{
			Level:   LevelError,
			Message: "\"config\" must be a JSON array",
			Source:  &SourceRef{JSONPointer: "/config"},
			RuleID:  "CONFIG_INVALID",
			Tier:    0,
		})
		return diags
	}

	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		diags = append(diags, LintDiagnostic{
			Level:   LevelError,
			Message: "\"config\" is not a valid JSON array: " + err.Error(),
			Source:  &SourceRef{JSONPointer: "/config"},
			RuleID:  "CONFIG_INVALID",
			Tier:    0,
		})
		return diags
	}

	for i, entry := range entries {
		entryTrimmed := bytes.TrimSpace(entry)
		if len(entryTrimmed) == 0 || entryTrimmed[0] != '{' {
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: fmt.Sprintf("config[%d] must be a JSON object", i),
				Source:  &SourceRef{JSONPointer: fmt.Sprintf("/config/%d", i)},
				RuleID:  "CONFIG_INVALID",
				Tier:    0,
			})
			continue
		}

		var obj map[string]json.RawMessage
		if err := json.Unmarshal(entry, &obj); err != nil {
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: fmt.Sprintf("config[%d] is not valid JSON: %s", i, err.Error()),
				Source:  &SourceRef{JSONPointer: fmt.Sprintf("/config/%d", i)},
				RuleID:  "CONFIG_INVALID",
				Tier:    0,
			})
			continue
		}

		kwRaw, ok := obj["keyword"]
		if !ok {
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: fmt.Sprintf("config[%d] missing \"keyword\" field", i),
				Source:  &SourceRef{JSONPointer: fmt.Sprintf("/config/%d", i)},
				RuleID:  "CONFIG_INVALID",
				Tier:    0,
			})
			continue
		}

		var kw string
		if err := json.Unmarshal(kwRaw, &kw); err != nil || kw != "application" {
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: fmt.Sprintf("config[%d] keyword must be \"application\"", i),
				Source:  &SourceRef{JSONPointer: fmt.Sprintf("/config/%d/keyword", i)},
				RuleID:  "CONFIG_INVALID",
				Tier:    0,
			})
		}
	}

	return diags
}

// walkStepsTier0 validates step-level schema using raw JSON traversal.
func walkStepsTier0(raw json.RawMessage, pointer string) []LintDiagnostic {
	var diags []LintDiagnostic

	var step map[string]json.RawMessage
	if err := json.Unmarshal(raw, &step); err != nil {
		return diags
	}

	// Check this step's fields
	diags = append(diags, checkStepFields(step, pointer)...)

	// Recurse into block
	if blockRaw, ok := step["block"]; ok {
		var block []json.RawMessage
		if err := json.Unmarshal(blockRaw, &block); err == nil {
			for i, child := range block {
				childPointer := fmt.Sprintf("%s/block/%d", pointer, i)
				diags = append(diags, walkStepsTier0(child, childPointer)...)
			}
		}
	}

	return diags
}

// checkStepFields validates individual step fields for tier 0.
func checkStepFields(step map[string]json.RawMessage, pointer string) []LintDiagnostic {
	var diags []LintDiagnostic

	// STEP_MISSING_KEYWORD
	if _, ok := step["keyword"]; !ok {
		diags = append(diags, LintDiagnostic{
			Level:   LevelError,
			Message: "Step is missing \"keyword\"",
			Source:  &SourceRef{JSONPointer: pointer},
			RuleID:  "STEP_MISSING_KEYWORD",
			Tier:    0,
		})
	}

	// STEP_MISSING_NUMBER
	numRaw, hasNumber := step["number"]
	if !hasNumber {
		diags = append(diags, LintDiagnostic{
			Level:   LevelError,
			Message: "Step is missing \"number\"",
			Source:  &SourceRef{JSONPointer: pointer},
			RuleID:  "STEP_MISSING_NUMBER",
			Tier:    0,
		})
	} else {
		// NUMBER_NOT_INTEGER — number must be a JSON number, not string
		trimmed := bytes.TrimSpace(numRaw)
		if len(trimmed) > 0 && trimmed[0] == '"' {
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: "Step \"number\" must be a JSON number, not a string",
				Source:  &SourceRef{JSONPointer: pointer + "/number"},
				RuleID:  "NUMBER_NOT_INTEGER",
				Tier:    0,
			})
		}
	}

	// STEP_MISSING_UUID
	uuidRaw, hasUUID := step["uuid"]
	if !hasUUID {
		diags = append(diags, LintDiagnostic{
			Level:   LevelError,
			Message: "Step is missing \"uuid\"",
			Source:  &SourceRef{JSONPointer: pointer},
			RuleID:  "STEP_MISSING_UUID",
			Tier:    0,
		})
	} else {
		// UUID_TOO_LONG
		var uuid string
		if err := json.Unmarshal(uuidRaw, &uuid); err == nil {
			if len(uuid) > 36 {
				diags = append(diags, LintDiagnostic{
					Level:   LevelError,
					Message: fmt.Sprintf("UUID exceeds 36 characters (length: %d)", len(uuid)),
					Source:  &SourceRef{JSONPointer: pointer + "/uuid"},
					RuleID:  "UUID_TOO_LONG",
					Tier:    0,
				})
			}
		}
	}

	return diags
}
