package lint

import (
	"encoding/json"
	"testing"
)

// validRecipeJSON returns a minimal valid recipe JSON for testing.
func validRecipeJSON() map[string]interface{} {
	return map[string]interface{}{
		"name":        "Test Recipe",
		"version":     1,
		"private":     true,
		"concurrency": "1",
		"code": map[string]interface{}{
			"keyword": "trigger",
			"number":  0,
			"uuid":    "trigger-step",
			"block": []interface{}{
				map[string]interface{}{
					"keyword": "action",
					"number":  1,
					"uuid":    "step-one",
				},
			},
		},
		"config": []interface{}{
			map[string]interface{}{
				"keyword":  "application",
				"provider": "salesforce",
				"name":     "salesforce",
			},
		},
	}
}

func toJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	return data
}

func hasDiag(diags []LintDiagnostic, ruleID string) bool {
	for _, d := range diags {
		if d.RuleID == ruleID {
			return true
		}
	}
	return false
}

func countDiag(diags []LintDiagnostic, ruleID string) int {
	n := 0
	for _, d := range diags {
		if d.RuleID == ruleID {
			n++
		}
	}
	return n
}

func TestTier0_ValidRecipe(t *testing.T) {
	data := toJSON(t, validRecipeJSON())
	diags := lintTier0(data)
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("unexpected diagnostic: %s — %s", d.RuleID, d.Message)
		}
	}
}

func TestTier0_MissingTopLevelKeys(t *testing.T) {
	keys := []string{"name", "version", "private", "concurrency", "code", "config"}
	for _, key := range keys {
		t.Run("missing_"+key, func(t *testing.T) {
			recipe := validRecipeJSON()
			delete(recipe, key)
			data := toJSON(t, recipe)
			diags := lintTier0(data)
			if !hasDiag(diags, "MISSING_TOP_LEVEL_KEYS") {
				t.Errorf("expected MISSING_TOP_LEVEL_KEYS for missing %q", key)
			}
		})
	}
}

func TestTier0_CodeNotObject(t *testing.T) {
	recipe := validRecipeJSON()
	recipe["code"] = []interface{}{1, 2, 3}
	data := toJSON(t, recipe)
	diags := lintTier0(data)
	if !hasDiag(diags, "CODE_NOT_OBJECT") {
		t.Error("expected CODE_NOT_OBJECT when code is an array")
	}
}

func TestTier0_CodeWrappedInRecipe(t *testing.T) {
	recipe := validRecipeJSON()
	recipe["recipe"] = map[string]interface{}{"name": "wrapped"}
	data := toJSON(t, recipe)
	diags := lintTier0(data)
	if !hasDiag(diags, "CODE_WRAPPED_IN_RECIPE") {
		t.Error("expected CODE_WRAPPED_IN_RECIPE")
	}
}

func TestTier0_StepMissingKeyword(t *testing.T) {
	recipe := validRecipeJSON()
	code := recipe["code"].(map[string]interface{})
	code["block"] = []interface{}{
		map[string]interface{}{
			"number": 1,
			"uuid":   "step-one",
		},
	}
	data := toJSON(t, recipe)
	diags := lintTier0(data)
	if !hasDiag(diags, "STEP_MISSING_KEYWORD") {
		t.Error("expected STEP_MISSING_KEYWORD")
	}
}

func TestTier0_StepMissingNumber(t *testing.T) {
	recipe := validRecipeJSON()
	code := recipe["code"].(map[string]interface{})
	code["block"] = []interface{}{
		map[string]interface{}{
			"keyword": "action",
			"uuid":    "step-one",
		},
	}
	data := toJSON(t, recipe)
	diags := lintTier0(data)
	if !hasDiag(diags, "STEP_MISSING_NUMBER") {
		t.Error("expected STEP_MISSING_NUMBER")
	}
}

func TestTier0_StepMissingUUID(t *testing.T) {
	recipe := validRecipeJSON()
	code := recipe["code"].(map[string]interface{})
	code["block"] = []interface{}{
		map[string]interface{}{
			"keyword": "action",
			"number":  1,
		},
	}
	data := toJSON(t, recipe)
	diags := lintTier0(data)
	if !hasDiag(diags, "STEP_MISSING_UUID") {
		t.Error("expected STEP_MISSING_UUID")
	}
}

func TestTier0_UUIDTooLong(t *testing.T) {
	recipe := validRecipeJSON()
	code := recipe["code"].(map[string]interface{})
	code["uuid"] = "this-uuid-is-way-too-long-and-exceeds-thirty-six-characters-total"
	data := toJSON(t, recipe)
	diags := lintTier0(data)
	if !hasDiag(diags, "UUID_TOO_LONG") {
		t.Error("expected UUID_TOO_LONG")
	}
}

func TestTier0_NumberNotInteger(t *testing.T) {
	recipe := validRecipeJSON()
	code := recipe["code"].(map[string]interface{})
	code["block"] = []interface{}{
		map[string]interface{}{
			"keyword": "action",
			"number":  "1",
			"uuid":    "step-one",
		},
	}
	data := toJSON(t, recipe)
	diags := lintTier0(data)
	if !hasDiag(diags, "NUMBER_NOT_INTEGER") {
		t.Error("expected NUMBER_NOT_INTEGER when number is a string")
	}
}

func TestTier0_ConfigInvalid_NotArray(t *testing.T) {
	recipe := validRecipeJSON()
	recipe["config"] = "not-an-array"
	data := toJSON(t, recipe)
	diags := lintTier0(data)
	if !hasDiag(diags, "CONFIG_INVALID") {
		t.Error("expected CONFIG_INVALID when config is not an array")
	}
}

func TestTier0_ConfigInvalid_BadKeyword(t *testing.T) {
	recipe := validRecipeJSON()
	recipe["config"] = []interface{}{
		map[string]interface{}{
			"keyword":  "not_application",
			"provider": "salesforce",
		},
	}
	data := toJSON(t, recipe)
	diags := lintTier0(data)
	if !hasDiag(diags, "CONFIG_INVALID") {
		t.Error("expected CONFIG_INVALID when config keyword is not 'application'")
	}
}

func TestTier0_ConfigInvalid_MissingKeyword(t *testing.T) {
	recipe := validRecipeJSON()
	recipe["config"] = []interface{}{
		map[string]interface{}{
			"provider": "salesforce",
		},
	}
	data := toJSON(t, recipe)
	diags := lintTier0(data)
	if !hasDiag(diags, "CONFIG_INVALID") {
		t.Error("expected CONFIG_INVALID when config entry missing keyword")
	}
}

func TestTier0_AllDiagsAreTier0(t *testing.T) {
	// Create a recipe with multiple issues
	recipe := map[string]interface{}{
		"name":    "test",
		"version": 1,
		"private": true,
		"code":    []interface{}{1},
		"config":  "bad",
		"recipe":  map[string]interface{}{},
	}
	data := toJSON(t, recipe)
	diags := lintTier0(data)
	for _, d := range diags {
		if d.Tier != 0 {
			t.Errorf("expected tier 0 for rule %s, got tier %d", d.RuleID, d.Tier)
		}
	}
}

func TestTier0_NestedStepValidation(t *testing.T) {
	recipe := validRecipeJSON()
	code := recipe["code"].(map[string]interface{})
	code["block"] = []interface{}{
		map[string]interface{}{
			"keyword": "if",
			"number":  1,
			"uuid":    "if-step",
			"block": []interface{}{
				map[string]interface{}{
					// Missing keyword
					"number": 2,
					"uuid":   "nested-step",
				},
			},
		},
	}
	data := toJSON(t, recipe)
	diags := lintTier0(data)
	if !hasDiag(diags, "STEP_MISSING_KEYWORD") {
		t.Error("expected STEP_MISSING_KEYWORD for nested step")
	}
}

func TestTier0_TriggerStepAlsoChecked(t *testing.T) {
	recipe := validRecipeJSON()
	// Remove keyword from the trigger (code root)
	code := recipe["code"].(map[string]interface{})
	delete(code, "keyword")
	data := toJSON(t, recipe)
	diags := lintTier0(data)
	if !hasDiag(diags, "STEP_MISSING_KEYWORD") {
		t.Error("expected STEP_MISSING_KEYWORD for trigger step itself")
	}
}

func TestTier0_MultipleStepsMissingUUID(t *testing.T) {
	recipe := validRecipeJSON()
	code := recipe["code"].(map[string]interface{})
	code["block"] = []interface{}{
		map[string]interface{}{
			"keyword": "action",
			"number":  1,
		},
		map[string]interface{}{
			"keyword": "action",
			"number":  2,
		},
	}
	data := toJSON(t, recipe)
	diags := lintTier0(data)
	count := countDiag(diags, "STEP_MISSING_UUID")
	if count != 2 {
		t.Errorf("expected 2 STEP_MISSING_UUID diagnostics, got %d", count)
	}
}
