package lint

import (
	"encoding/json"
	"testing"

	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

func sfStepWithInput(name string, input json.RawMessage) recipe.FlatStep {
	return recipe.FlatStep{
		Code: recipe.Code{
			Keyword:  "action",
			Provider: strPtr("salesforce"),
			Name:     name,
			Input:    input,
		},
		JSONPointer: "/code/block/0",
	}
}

// --- RequireFields ---

func TestActionRule_RequireFields_Pass(t *testing.T) {
	step := sfStepWithInput("search_sobjects", json.RawMessage(`{"sobject_name":"Account"}`))
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {
			ActionRules: []ActionRule{{
				RuleID:        "SF_REQUIRE",
				ActionNames:   []string{"search_sobjects"},
				RequireFields: []string{"sobject_name"},
				RequireIn:     []string{"input"},
				Message:       "missing {field_name} in {missing_location}",
			}},
		},
	}
	diags := checkActionRules(parsed, connRules)
	if hasDiag(diags, "SF_REQUIRE") {
		t.Error("field is present, should not produce diagnostic")
	}
}

func TestActionRule_RequireFields_MissingInInput(t *testing.T) {
	step := sfStepWithInput("search_sobjects", json.RawMessage(`{"query":"test"}`))
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {
			ActionRules: []ActionRule{{
				RuleID:        "SF_REQUIRE",
				ActionNames:   []string{"search_sobjects"},
				RequireFields: []string{"sobject_name"},
				RequireIn:     []string{"input"},
				Message:       "missing {field_name} in {missing_location}",
			}},
		},
	}
	diags := checkActionRules(parsed, connRules)
	if !hasDiag(diags, "SF_REQUIRE") {
		t.Error("field is missing, should produce diagnostic")
	}
	if diags[0].Message != "missing sobject_name in input" {
		t.Errorf("unexpected message: %s", diags[0].Message)
	}
}

func TestActionRule_RequireFields_NilInput(t *testing.T) {
	step := sfStepWithInput("search_sobjects", nil)
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {
			ActionRules: []ActionRule{{
				RuleID:        "SF_REQUIRE",
				ActionNames:   []string{"search_sobjects"},
				RequireFields: []string{"sobject_name"},
				RequireIn:     []string{"input"},
				Message:       "missing",
			}},
		},
	}
	diags := checkActionRules(parsed, connRules)
	if !hasDiag(diags, "SF_REQUIRE") {
		t.Error("nil input should produce diagnostic")
	}
}

func TestActionRule_RequireFields_DefaultRequireIn(t *testing.T) {
	step := sfStepWithInput("search_sobjects", json.RawMessage(`{"query":"test"}`))
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {
			ActionRules: []ActionRule{{
				RuleID:        "SF_REQUIRE",
				ActionNames:   []string{"search_sobjects"},
				RequireFields: []string{"sobject_name"},
				Message:       "missing",
			}},
		},
	}
	diags := checkActionRules(parsed, connRules)
	if !hasDiag(diags, "SF_REQUIRE") {
		t.Error("should default to checking input")
	}
}

func TestActionRule_RequireFields_NonMatchingAction(t *testing.T) {
	step := sfStepWithInput("upsert_sobject", json.RawMessage(`{}`))
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {
			ActionRules: []ActionRule{{
				RuleID:        "SF_REQUIRE",
				ActionNames:   []string{"search_sobjects"},
				RequireFields: []string{"sobject_name"},
				Message:       "missing",
			}},
		},
	}
	diags := checkActionRules(parsed, connRules)
	if hasDiag(diags, "SF_REQUIRE") {
		t.Error("action does not match, should not produce diagnostic")
	}
}

func TestActionRule_RequireFields_NoProvider(t *testing.T) {
	step := recipe.FlatStep{
		Code:        recipe.Code{Keyword: "if", Name: "conditional"},
		JSONPointer: "/code/block/0",
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {ActionRules: []ActionRule{{
			RuleID: "SF_REQUIRE", ActionNames: []string{"conditional"},
			RequireFields: []string{"x"}, Message: "missing",
		}}},
	}
	diags := checkActionRules(parsed, connRules)
	if hasDiag(diags, "SF_REQUIRE") {
		t.Error("no provider, should skip")
	}
}

func TestActionRule_RequireFields_EmptyConnRules(t *testing.T) {
	step := sfStepWithInput("search_sobjects", json.RawMessage(`{}`))
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	diags := checkActionRules(parsed, nil)
	if len(diags) != 0 {
		t.Error("empty connRules should produce no diagnostics")
	}
}

// --- EISMustBeEmpty ---

func TestActionRule_EISMustBeEmpty_Pass_Nil(t *testing.T) {
	step := sfStepWithInput("search_sobjects_soql", nil)
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {ActionRules: []ActionRule{{
			RuleID: "SF_SOQL", ActionNames: []string{"search_sobjects_soql"},
			EISMustBeEmpty: true, Message: "SOQL should have empty EIS",
		}}},
	}
	diags := checkActionRules(parsed, connRules)
	if hasDiag(diags, "SF_SOQL") {
		t.Error("nil EIS should pass")
	}
}

func TestActionRule_EISMustBeEmpty_Pass_EmptyArray(t *testing.T) {
	step := recipe.FlatStep{
		Code: recipe.Code{
			Keyword: "action", Provider: strPtr("salesforce"),
			Name: "search_sobjects_soql",
			ExtendedInputSchema: json.RawMessage(`[]`),
		},
		JSONPointer: "/code/block/0",
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {ActionRules: []ActionRule{{
			RuleID: "SF_SOQL", ActionNames: []string{"search_sobjects_soql"},
			EISMustBeEmpty: true, Message: "SOQL should have empty EIS",
		}}},
	}
	diags := checkActionRules(parsed, connRules)
	if hasDiag(diags, "SF_SOQL") {
		t.Error("empty array EIS should pass")
	}
}

func TestActionRule_EISMustBeEmpty_Fail(t *testing.T) {
	step := recipe.FlatStep{
		Code: recipe.Code{
			Keyword: "action", Provider: strPtr("salesforce"),
			Name: "search_sobjects_soql",
			ExtendedInputSchema: json.RawMessage(`[{"name":"field"}]`),
		},
		JSONPointer: "/code/block/0",
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {ActionRules: []ActionRule{{
			RuleID: "SF_SOQL", ActionNames: []string{"search_sobjects_soql"},
			EISMustBeEmpty: true, Message: "SOQL should have empty EIS",
		}}},
	}
	diags := checkActionRules(parsed, connRules)
	if !hasDiag(diags, "SF_SOQL") {
		t.Error("non-empty EIS should produce diagnostic")
	}
}

func TestActionRule_EISMustBeEmpty_FlagFalse(t *testing.T) {
	step := recipe.FlatStep{
		Code: recipe.Code{
			Keyword: "action", Provider: strPtr("salesforce"),
			Name: "search_sobjects_soql",
			ExtendedInputSchema: json.RawMessage(`[{"name":"field"}]`),
		},
		JSONPointer: "/code/block/0",
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {ActionRules: []ActionRule{{
			RuleID: "SF_SOQL", ActionNames: []string{"search_sobjects_soql"},
			EISMustBeEmpty: false, Message: "SOQL should have empty EIS",
		}}},
	}
	diags := checkActionRules(parsed, connRules)
	if hasDiag(diags, "SF_SOQL") {
		t.Error("eis_must_be_empty is false, should skip")
	}
}

// --- FieldTypeChecks ---

func TestActionRule_FieldTypeCheck_Pass(t *testing.T) {
	step := recipe.FlatStep{
		Code: recipe.Code{
			Keyword: "action", Provider: strPtr("salesforce"),
			Name:                "search_sobjects",
			ExtendedInputSchema: json.RawMessage(`[{"name":"limit","type":"integer","parse_output":"integer_conversion"}]`),
		},
		JSONPointer: "/code/block/0",
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {ActionRules: []ActionRule{{
			RuleID: "SF_LIMIT", ActionNames: []string{"search_sobjects"},
			FieldTypeChecks: map[string]FieldCheck{
				"limit": {Type: "integer", ParseOutput: "integer_conversion"},
			},
			Message: "limit type mismatch",
		}}},
	}
	diags := checkActionRules(parsed, connRules)
	if hasDiag(diags, "SF_LIMIT") {
		t.Error("types match, should not produce diagnostic")
	}
}

func TestActionRule_FieldTypeCheck_WrongType(t *testing.T) {
	step := recipe.FlatStep{
		Code: recipe.Code{
			Keyword: "action", Provider: strPtr("salesforce"),
			Name:                "search_sobjects",
			ExtendedInputSchema: json.RawMessage(`[{"name":"limit","type":"number"}]`),
		},
		JSONPointer: "/code/block/0",
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {ActionRules: []ActionRule{{
			RuleID: "SF_LIMIT", ActionNames: []string{"search_sobjects"},
			FieldTypeChecks: map[string]FieldCheck{
				"limit": {Type: "integer"},
			},
			Message: "limit must be integer",
		}}},
	}
	diags := checkActionRules(parsed, connRules)
	if !hasDiag(diags, "SF_LIMIT") {
		t.Error("wrong type, should produce diagnostic")
	}
}

func TestActionRule_FieldTypeCheck_FieldMissing(t *testing.T) {
	step := recipe.FlatStep{
		Code: recipe.Code{
			Keyword: "action", Provider: strPtr("salesforce"),
			Name:                "search_sobjects",
			ExtendedInputSchema: json.RawMessage(`[{"name":"other","type":"string"}]`),
		},
		JSONPointer: "/code/block/0",
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {ActionRules: []ActionRule{{
			RuleID: "SF_LIMIT", ActionNames: []string{"search_sobjects"},
			FieldTypeChecks: map[string]FieldCheck{
				"limit": {Type: "integer"},
			},
			Message: "limit must be integer",
		}}},
	}
	diags := checkActionRules(parsed, connRules)
	if hasDiag(diags, "SF_LIMIT") {
		t.Error("field not in EIS, should skip silently")
	}
}

func TestActionRule_FieldTypeCheck_NoEIS(t *testing.T) {
	step := sfStepWithInput("search_sobjects", json.RawMessage(`{"limit":50}`))
	parsed := buildParsedRecipe("test", []recipe.FlatStep{step}, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {ActionRules: []ActionRule{{
			RuleID: "SF_LIMIT", ActionNames: []string{"search_sobjects"},
			FieldTypeChecks: map[string]FieldCheck{
				"limit": {Type: "integer"},
			},
			Message: "limit must be integer",
		}}},
	}
	diags := checkActionRules(parsed, connRules)
	if hasDiag(diags, "SF_LIMIT") {
		t.Error("no EIS, should skip silently")
	}
}

// --- Integration ---

func TestActionRule_MultipleRules(t *testing.T) {
	steps := []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword: "action", Provider: strPtr("salesforce"),
				Name:  "search_sobjects",
				Input: json.RawMessage(`{"query":"test"}`),
			},
			JSONPointer: "/code/block/0",
		},
		{
			Code: recipe.Code{
				Keyword: "action", Provider: strPtr("salesforce"),
				Name:                "search_sobjects_soql",
				ExtendedInputSchema: json.RawMessage(`[{"name":"field"}]`),
			},
			JSONPointer: "/code/block/1",
		},
	}
	parsed := buildParsedRecipe("test", steps, nil)
	connRules := map[string]*ConnectorRules{
		"salesforce": {ActionRules: []ActionRule{
			{
				RuleID: "SF_REQUIRE", ActionNames: []string{"search_sobjects"},
				RequireFields: []string{"sobject_name"}, Message: "missing sobject_name",
			},
			{
				RuleID: "SF_SOQL_EIS", ActionNames: []string{"search_sobjects_soql"},
				EISMustBeEmpty: true, Message: "SOQL EIS should be empty",
			},
		}},
	}
	diags := checkActionRules(parsed, connRules)
	if !hasDiag(diags, "SF_REQUIRE") {
		t.Error("expected SF_REQUIRE diagnostic")
	}
	if !hasDiag(diags, "SF_SOQL_EIS") {
		t.Error("expected SF_SOQL_EIS diagnostic")
	}
	for _, d := range diags {
		if d.Tier != 1 {
			t.Errorf("expected tier 1, got %d for %s", d.Tier, d.RuleID)
		}
	}
}
