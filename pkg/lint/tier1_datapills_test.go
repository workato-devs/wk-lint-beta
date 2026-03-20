package lint

import (
	"testing"

	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

func TestExtractDatapills_ValidJSON(t *testing.T) {
	input := `=_dp('{"pill_type":"output","provider":"salesforce","line":"step1","path":["body","id"]}')`
	dps := extractDatapills(input)
	if len(dps) != 1 {
		t.Fatalf("expected 1 datapill, got %d", len(dps))
	}
	if dps[0].ParseErr != nil {
		t.Errorf("expected no parse error, got %v", dps[0].ParseErr)
	}
	if dps[0].Payload == nil {
		t.Fatal("expected non-nil payload")
	}
	if dps[0].Payload.PillType != "output" {
		t.Errorf("expected pill_type=output, got %s", dps[0].Payload.PillType)
	}
}

func TestExtractDatapills_InvalidJSON(t *testing.T) {
	input := `=_dp('not valid json')`
	dps := extractDatapills(input)
	if len(dps) != 1 {
		t.Fatalf("expected 1 datapill, got %d", len(dps))
	}
	if dps[0].ParseErr == nil {
		t.Error("expected parse error for invalid JSON")
	}
}

func TestExtractDatapills_Multiple(t *testing.T) {
	input := `=_dp('{"pill_type":"output","provider":"sf","line":"s1","path":[]}') + " " + _dp('{"pill_type":"output","provider":"sf","line":"s2","path":[]}')`
	dps := extractDatapills(input)
	if len(dps) != 2 {
		t.Fatalf("expected 2 datapills, got %d", len(dps))
	}
}

func TestExtractDatapills_None(t *testing.T) {
	input := `just a plain string`
	dps := extractDatapills(input)
	if len(dps) != 0 {
		t.Errorf("expected 0 datapills, got %d", len(dps))
	}
}

func TestDP_VALID_JSON_Error(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("salesforce"),
				Input:    rawJSON(t, map[string]interface{}{"field": "=_dp('bad json')"}),
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	if !hasDiag(diags, "DP_VALID_JSON") {
		t.Error("expected DP_VALID_JSON for invalid datapill JSON")
	}
}

func TestDP_VALID_JSON_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("salesforce"),
				Input:    rawJSON(t, map[string]interface{}{"field": `=_dp('{"pill_type":"output","provider":"sf","line":"s1","path":[]}')`}),
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	if hasDiag(diags, "DP_VALID_JSON") {
		t.Error("unexpected DP_VALID_JSON for valid datapill JSON")
	}
}

func TestDP_LHS_NO_FORMULA_Error(t *testing.T) {
	// Simulate a condition LHS that's a formula
	input := rawJSON(t, map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"lhs": `=_dp('{"pill_type":"output","provider":"sf","line":"s1","path":[]}').present?`,
				"rhs": "true",
				"op":  "equals",
			},
		},
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "if",
				Provider: nil,
				Input:    input,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	if !hasDiag(diags, "DP_LHS_NO_FORMULA") {
		t.Error("expected DP_LHS_NO_FORMULA for formula in condition LHS")
	}
}

func TestDP_INTERPOLATION_SINGLE_Warn(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("salesforce"),
				Input:    rawJSON(t, map[string]interface{}{"field": `=_dp('{"pill_type":"output","provider":"sf","line":"s1","path":[]}')`}),
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	if !hasDiag(diags, "DP_INTERPOLATION_SINGLE") {
		t.Error("expected DP_INTERPOLATION_SINGLE for single datapill in formula mode")
	}
}

func TestDP_INTERPOLATION_SINGLE_MethodChain_Pass(t *testing.T) {
	// =_dp('...').present? is valid — NOT a violation
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("salesforce"),
				Input:    rawJSON(t, map[string]interface{}{"field": `=_dp('{"pill_type":"output","provider":"sf","line":"s1","path":[]}').present?`}),
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	if hasDiag(diags, "DP_INTERPOLATION_SINGLE") {
		t.Error("unexpected DP_INTERPOLATION_SINGLE when datapill has method chain")
	}
}

func TestDP_FORMULA_CONCAT_Warn(t *testing.T) {
	dp1 := `_dp('{"pill_type":"output","provider":"sf","line":"s1","path":[]}')`
	dp2 := `_dp('{"pill_type":"output","provider":"sf","line":"s2","path":[]}')`
	value := "Hello #{" + dp1 + "} and #{" + dp2 + "}"
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("salesforce"),
				Input:    rawJSON(t, map[string]interface{}{"field": value}),
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	if !hasDiag(diags, "DP_FORMULA_CONCAT") {
		t.Error("expected DP_FORMULA_CONCAT for multiple datapills with interpolation")
	}
}

func TestDP_NO_OUTER_PARENS_Info(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("salesforce"),
				Input:    rawJSON(t, map[string]interface{}{"field": `=(_dp('{"pill_type":"output","provider":"sf","line":"s1","path":[]}').to_s)`}),
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	if !hasDiag(diags, "DP_NO_OUTER_PARENS") {
		t.Error("expected DP_NO_OUTER_PARENS for formula wrapped in outer parens")
	}
}

func TestDP_NO_OUTER_PARENS_Ternary_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("salesforce"),
				Input:    rawJSON(t, map[string]interface{}{"field": `=(_dp('{"pill_type":"output","provider":"sf","line":"s1","path":[]}').present? ? "yes" : "no")`}),
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	if hasDiag(diags, "DP_NO_OUTER_PARENS") {
		t.Error("unexpected DP_NO_OUTER_PARENS for ternary expression")
	}
}

func TestDP_NO_BODY_NATIVE_Warn(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("salesforce"),
				Input:    rawJSON(t, map[string]interface{}{"field": `=_dp('{"pill_type":"output","provider":"sf","line":"s1","path":["body","id"]}')`}),
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	if !hasDiag(diags, "DP_NO_BODY_NATIVE") {
		t.Error("expected DP_NO_BODY_NATIVE for body path on non-API connector")
	}
}

func TestDP_NO_BODY_NATIVE_APIPlatform_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("rest"),
				Input:    rawJSON(t, map[string]interface{}{"field": `=_dp('{"pill_type":"output","provider":"rest","line":"s1","path":["body","id"]}')`}),
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	if hasDiag(diags, "DP_NO_BODY_NATIVE") {
		t.Error("unexpected DP_NO_BODY_NATIVE for API platform connector")
	}
}

func TestDP_CATCH_PROVIDER_Warn(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword: "catch",
				As:      "error_msg",
			},
			JSONPointer: "/code/block/0",
		},
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("logger"),
				Input:    rawJSON(t, map[string]interface{}{"field": `=_dp('{"pill_type":"output","provider":null,"line":"error_msg","path":["message"]}')`}),
			},
			JSONPointer: "/code/block/0/block/0",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	if !hasDiag(diags, "DP_CATCH_PROVIDER") {
		t.Error("expected DP_CATCH_PROVIDER for null provider matching catch alias")
	}
}

func TestDP_NoDatapills_NoFalsePositives(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("salesforce"),
				Input:    rawJSON(t, map[string]interface{}{"field": "just a plain string"}),
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("unexpected diagnostic: %s — %s", d.RuleID, d.Message)
		}
	}
}

func TestDP_AllDiagsAreTier1(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("salesforce"),
				Input:    rawJSON(t, map[string]interface{}{"field": "=_dp('bad json')"}),
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	for _, d := range diags {
		if d.Tier != 1 {
			t.Errorf("expected tier 1 for rule %s, got tier %d", d.RuleID, d.Tier)
		}
	}
}
