package lint

import (
	"encoding/json"
	"testing"

	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

func TestEIS_MIRRORS_INPUT_Warn(t *testing.T) {
	eis := json.RawMessage(`[{"name":"name","label":"Name","type":"string"}]`)
	input := rawJSON(t, map[string]interface{}{
		"name":          "Alice",
		"missing_field": "value",
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("salesforce"),
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	if !hasDiag(diags, "EIS_MIRRORS_INPUT") {
		t.Error("expected EIS_MIRRORS_INPUT for input key not in EIS")
	}
}

func TestEIS_MIRRORS_INPUT_Pass(t *testing.T) {
	eis := json.RawMessage(`[{"name":"name","label":"Name","type":"string"},{"name":"email","label":"Email","type":"string"}]`)
	input := rawJSON(t, map[string]interface{}{
		"name":  "Alice",
		"email": "alice@example.com",
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("salesforce"),
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	if hasDiag(diags, "EIS_MIRRORS_INPUT") {
		t.Error("unexpected EIS_MIRRORS_INPUT when all input keys in EIS")
	}
}

func TestEIS_NAME_MATCH_Warn(t *testing.T) {
	eis := json.RawMessage(`[{"name":"extra_field","label":"Extra","type":"string"}]`)
	input := rawJSON(t, map[string]interface{}{
		"other_field": "value",
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("salesforce"),
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	if !hasDiag(diags, "EIS_NAME_MATCH") {
		t.Error("expected EIS_NAME_MATCH for EIS field not in input")
	}
}

func TestEIS_NESTED_MATCH_Warn(t *testing.T) {
	eis := json.RawMessage(`[{"name":"address","label":"Address","type":"object"}]`)
	input := rawJSON(t, map[string]interface{}{
		"address": map[string]interface{}{
			"street": "123 Main St",
			"city":   "NYC",
		},
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("salesforce"),
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	if !hasDiag(diags, "EIS_NESTED_MATCH") {
		t.Error("expected EIS_NESTED_MATCH for nested input without EIS properties")
	}
}

func TestEIS_NESTED_MATCH_Pass(t *testing.T) {
	eis := json.RawMessage(`[{"name":"address","label":"Address","type":"object","properties":[{"name":"street","type":"string"},{"name":"city","type":"string"}]}]`)
	input := rawJSON(t, map[string]interface{}{
		"address": map[string]interface{}{
			"street": "123 Main St",
			"city":   "NYC",
		},
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("salesforce"),
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	if hasDiag(diags, "EIS_NESTED_MATCH") {
		t.Error("unexpected EIS_NESTED_MATCH when EIS has properties")
	}
}

func TestEIS_NO_CONNECTOR_INTERNAL_Warn(t *testing.T) {
	eis := json.RawMessage(`[{"name":"action_name","label":"Action","type":"string"},{"name":"real_field","label":"Real","type":"string"}]`)
	input := rawJSON(t, map[string]interface{}{
		"action_name": "create",
		"real_field":  "value",
	})
	connRules := map[string]*ConnectorRules{
		"salesforce": {
			ConnectorInternals: []string{"action_name"},
		},
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("salesforce"),
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, connRules)
	if !hasDiag(diags, "EIS_NO_CONNECTOR_INTERNAL") {
		t.Error("expected EIS_NO_CONNECTOR_INTERNAL for connector-internal field in EIS")
	}
}

func TestEIS_OUTPUT_MIRRORS_INPUT_Info(t *testing.T) {
	eis := json.RawMessage(`[{"name":"result","label":"Result","type":"string"},{"name":"status","label":"Status","type":"string"}]`)
	eos := json.RawMessage(`[{"name":"result","label":"Result","type":"string"}]`)
	input := rawJSON(t, map[string]interface{}{
		"result": "ok",
		"status": "done",
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:              "action",
				Provider:             strPtr("workato_recipe_function"),
				Name:                 "return_result",
				Input:                input,
				ExtendedInputSchema:  eis,
				ExtendedOutputSchema: eos,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	if !hasDiag(diags, "EIS_OUTPUT_MIRRORS_INPUT") {
		t.Error("expected EIS_OUTPUT_MIRRORS_INPUT when EIS field missing from EOS")
	}
}

func TestEIS_NoEIS_NoFalsePositives(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("salesforce"),
				Input:    rawJSON(t, map[string]interface{}{"field": "value"}),
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("unexpected diagnostic: %s — %s", d.RuleID, d.Message)
		}
	}
}

func TestEIS_AllDiagsAreTier1(t *testing.T) {
	eis := json.RawMessage(`[{"name":"extra","label":"Extra","type":"string"}]`)
	input := rawJSON(t, map[string]interface{}{"other": "value"})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("salesforce"),
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	for _, d := range diags {
		if d.Tier != 1 {
			t.Errorf("expected tier 1 for rule %s, got tier %d", d.RuleID, d.Tier)
		}
	}
}
