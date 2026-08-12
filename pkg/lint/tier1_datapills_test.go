package lint

import (
	"testing"

	"github.com/workato-devs/recipe-lint/pkg/recipe"
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

func TestDP_LHS_NO_FORMULA_DatapillWithTransform_Pass(t *testing.T) {
	// A datapill with a formula transform (e.g. .present?) is still datapill-based
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
	if hasDiag(diags, "DP_LHS_NO_FORMULA") {
		t.Error("unexpected DP_LHS_NO_FORMULA for datapill with formula transform")
	}
}

func TestDP_LHS_NO_FORMULA_PureFormula_Error(t *testing.T) {
	input := rawJSON(t, map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"lhs": `="some_value".upcase`,
				"rhs": "SOME_VALUE",
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
		t.Error("expected DP_LHS_NO_FORMULA for pure formula in condition LHS")
	}
}

func TestDP_LHS_NO_FORMULA_TriggerFilter_Error(t *testing.T) {
	filter := rawJSON(t, map[string]interface{}{
		"type":    "compound",
		"operand": "and",
		"conditions": []interface{}{
			map[string]interface{}{
				"lhs":     `="some_value".upcase`,
				"operand": "equals",
				"rhs":     "SOME_VALUE",
				"uuid":    "cond-1",
			},
		},
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "trigger",
				Provider: strPtr("asana"),
				Name:     "new_event",
				Filter:   filter,
			},
			JSONPointer: "/code",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	if !hasDiag(diags, "DP_LHS_NO_FORMULA") {
		t.Error("expected DP_LHS_NO_FORMULA for pure formula in trigger filter condition LHS — code.filter should get the same datapill validation as an if-block's input.conditions")
	}
}

func TestDP_LHS_NO_FORMULA_CaseInsensitiveComparison_Pass(t *testing.T) {
	// =dp('...brand...').upcase is a datapill with .upcase transform — legitimate pattern
	input := rawJSON(t, map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"lhs": `=_dp('{"pill_type":"output","provider":"sf","line":"s1","path":["brand"]}').upcase`,
				"rhs": "ERI",
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
	if hasDiag(diags, "DP_LHS_NO_FORMULA") {
		t.Error("unexpected DP_LHS_NO_FORMULA for case-insensitive comparison pattern")
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

// --- DP_INTERPOLATION_SINGLE: target field type ---

// dpPill is a well-formed single datapill, unwrapped.
const dpPill = `_dp('{"pill_type":"output","provider":"p","line":"expense_call","path":["reports"]}')`

// typedFieldRecipe builds a one-action recipe whose input holds value at the field
// "target", with the step's extended_input_schema declaring that field as
// fieldType. An empty fieldType declares no EIS at all.
func typedFieldRecipe(t *testing.T, value, fieldType string) *recipe.ParsedRecipe {
	t.Helper()
	code := recipe.Code{
		Keyword:  "action",
		Provider: strPtr("salesforce"),
		Input:    rawJSON(t, map[string]interface{}{"target": value}),
	}
	if fieldType != "" {
		code.ExtendedInputSchema = rawJSON(t, []interface{}{
			map[string]interface{}{"name": "target", "type": fieldType},
		})
	}
	return buildParsedRecipe("test", []recipe.FlatStep{
		{Code: code, JSONPointer: "/code/block/0"},
	}, nil)
}

func TestDP_INTERPOLATION_SINGLE_TargetType(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		fieldType string
		wantHit   int
	}{
		{"formula on string field warns", "=" + dpPill, "string", 1},
		{"formula on undeclared field warns", "=" + dpPill, "", 1},
		{"formula on array field skipped", "=" + dpPill, "array", 0},
		{"formula on object field skipped", "=" + dpPill, "object", 0},
		{"formula on number field skipped", "=" + dpPill, "number", 0},
		{"formula on integer field skipped", "=" + dpPill, "integer", 0},
		{"formula on boolean field skipped", "=" + dpPill, "boolean", 0},
		{"interpolation on string field not flagged", "#{" + dpPill + "}", "string", 0},
		{"interpolation on array field not flagged", "#{" + dpPill + "}", "array", 0},
		{"method chain on string field not flagged", "=" + dpPill + ".present?", "string", 0},
		{"concatenation on string field not flagged", "=" + dpPill + ` + "x"`, "string", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := checkDatapillsWithCatchAliases(typedFieldRecipe(t, tt.value, tt.fieldType), nil)
			if got := countDiag(diags, "DP_INTERPOLATION_SINGLE"); got != tt.wantHit {
				t.Errorf("DP_INTERPOLATION_SINGLE hits = %d, want %d", got, tt.wantHit)
			}
		})
	}
}

// TestDP_INTERPOLATION_SINGLE_TriggerResultSchema covers a return step that declares
// no EIS: the field types come from the trigger's result_schema_json, which is what a
// recipe-function/skill trigger declares for the recipe's own output.
func TestDP_INTERPOLATION_SINGLE_TriggerResultSchema(t *testing.T) {
	resultSchema := `[{"name":"reports","label":"Reports","type":"array","of":"object"},` +
		`{"name":"summary","label":"Summary","type":"string"}]`
	build := func(field string) *recipe.ParsedRecipe {
		return buildParsedRecipe("test", []recipe.FlatStep{
			{Code: recipe.Code{
				Keyword:  "trigger",
				Provider: strPtr("workato_recipe_function"),
				Name:     "execute",
				As:       "trigger",
				Input:    rawJSON(t, map[string]interface{}{"result_schema_json": resultSchema}),
			}, JSONPointer: "/code"},
			{Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("workato_recipe_function"),
				Name:     "return_result",
				Input: rawJSON(t, map[string]interface{}{
					"result": map[string]interface{}{field: "=" + dpPill},
				}),
			}, JSONPointer: "/code/block/0"},
		}, nil)
	}

	if diags := checkDatapillsWithCatchAliases(build("reports"), nil); hasDiag(diags, "DP_INTERPOLATION_SINGLE") {
		t.Error("unexpected DP_INTERPOLATION_SINGLE: trigger result_schema_json declares \"reports\" as array — interpolation would stringify it")
	}
	if diags := checkDatapillsWithCatchAliases(build("summary"), nil); !hasDiag(diags, "DP_INTERPOLATION_SINGLE") {
		t.Error("expected DP_INTERPOLATION_SINGLE: \"summary\" is declared string, so interpolation mode is the right advice")
	}
}

// --- DP_BARE_UNWRAPPED ---

func TestDP_BARE_UNWRAPPED(t *testing.T) {
	second := `_dp('{"pill_type":"output","provider":"p","line":"other_call","path":["id"]}')`

	tests := []struct {
		name    string
		value   string
		wantHit int
	}{
		{"bare pill flagged", dpPill, 1},
		{"bare pill in text flagged", "Report: " + dpPill, 1},
		{"interpolated pill not flagged", "#{" + dpPill + "}", 0},
		{"interpolated pill in text not flagged", "Report: #{" + dpPill + "} (end)", 0},
		{"formula pill not flagged", "=" + dpPill, 0},
		{"formula concatenation not flagged", "=" + dpPill + " + " + second, 0},
		{"two interpolated pills not flagged", "#{" + dpPill + "} and #{" + second + "}", 0},
		{"interpolated then bare flagged once", "#{" + dpPill + "} and " + second, 1},
		{"no datapill not flagged", "just a plain string", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := checkDatapillsWithCatchAliases(typedFieldRecipe(t, tt.value, "string"), nil)
			if got := countDiag(diags, "DP_BARE_UNWRAPPED"); got != tt.wantHit {
				t.Errorf("DP_BARE_UNWRAPPED hits = %d, want %d", got, tt.wantHit)
			}
		})
	}
}

// TestDP_BARE_UNWRAPPED_ReturnResult reproduces the shipped failure: a return step
// whose input holds an unwrapped datapill. The recipe pushes, activates and runs
// green — the caller receives the literal characters _dp('{...}').
func TestDP_BARE_UNWRAPPED_ReturnResult(t *testing.T) {
	build := func(value string) *recipe.ParsedRecipe {
		return buildParsedRecipe("test", []recipe.FlatStep{
			{Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("workato_recipe_function"),
				Name:     "return_result",
				Input: rawJSON(t, map[string]interface{}{
					"result": map[string]interface{}{"reports": value},
				}),
				ExtendedInputSchema: rawJSON(t, []interface{}{
					map[string]interface{}{
						"name": "result",
						"type": "object",
						"properties": []interface{}{
							map[string]interface{}{"name": "reports", "type": "array"},
						},
					},
				}),
			}, JSONPointer: "/code/block/0"},
		}, nil)
	}

	broken := checkDatapillsWithCatchAliases(build(dpPill), nil)
	if !hasDiag(broken, "DP_BARE_UNWRAPPED") {
		t.Error("expected DP_BARE_UNWRAPPED for an unwrapped datapill in a return step")
	}

	fixed := checkDatapillsWithCatchAliases(build("="+dpPill), nil)
	if hasDiag(fixed, "DP_BARE_UNWRAPPED") {
		t.Error("unexpected DP_BARE_UNWRAPPED for the formula-mode fix")
	}
	if hasDiag(fixed, "DP_INTERPOLATION_SINGLE") {
		t.Error("unexpected DP_INTERPOLATION_SINGLE: the target field is declared array, so formula mode is what preserves the type")
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

func TestDP_NO_BODY_NATIVE_CrossProvider_Pass(t *testing.T) {
	// Datapill from REST source (uses body paths) consumed by a salesforce step — should NOT fire
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("salesforce"),
				Input:    rawJSON(t, map[string]interface{}{"field": `=_dp('{"pill_type":"output","provider":"rest","line":"make_request","path":["body","id"]}')`}),
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkDatapillsWithCatchAliases(parsed, nil)
	if hasDiag(diags, "DP_NO_BODY_NATIVE") {
		t.Error("unexpected DP_NO_BODY_NATIVE: body path from REST source is valid regardless of consuming step provider")
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
