package lint

import (
	"encoding/json"
	"testing"

	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

// helper to create *string
func strPtr(s string) *string { return &s }

// helper to create *int
func intPtr(i int) *int { return &i }

// helper to create json.RawMessage from value
func rawJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	return data
}

// buildParsedRecipe is a helper to build a basic ParsedRecipe from steps and config.
func buildParsedRecipe(name string, steps []recipe.FlatStep, config []recipe.ConfigEntry) *recipe.ParsedRecipe {
	providerSet := make(map[string]bool)
	for _, c := range config {
		if c.Provider != "" {
			providerSet[c.Provider] = true
		}
	}
	var providers []string
	for p := range providerSet {
		providers = append(providers, p)
	}
	return &recipe.ParsedRecipe{
		Raw: recipe.Recipe{
			Name: name,
		},
		Steps:     steps,
		Config:    config,
		Providers: providers,
	}
}

func TestTier1_StepNumbering_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "trigger", Number: intPtr(0)}, JSONPointer: "/code"},
		{Code: recipe.Code{Keyword: "action", Number: intPtr(1)}, JSONPointer: "/code/block/0"},
		{Code: recipe.Code{Keyword: "action", Number: intPtr(2)}, JSONPointer: "/code/block/1"},
	}, nil)
	diags := checkStepNumbering(parsed)
	if hasDiag(diags, "STEP_NUMBERING") {
		t.Error("expected no STEP_NUMBERING for sequential numbering")
	}
}

func TestTier1_StepNumbering_Fail(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "trigger", Number: intPtr(0)}, JSONPointer: "/code"},
		{Code: recipe.Code{Keyword: "action", Number: intPtr(5)}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkStepNumbering(parsed)
	if !hasDiag(diags, "STEP_NUMBERING") {
		t.Error("expected STEP_NUMBERING for non-sequential numbering")
	}
}

func TestTier1_UUIDUnique_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{UUID: "step-a"}, JSONPointer: "/code"},
		{Code: recipe.Code{UUID: "step-b"}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkUUIDUnique(parsed)
	if hasDiag(diags, "UUID_UNIQUE") {
		t.Error("expected no UUID_UNIQUE for unique UUIDs")
	}
}

func TestTier1_UUIDUnique_Fail(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{UUID: "same-uuid"}, JSONPointer: "/code"},
		{Code: recipe.Code{UUID: "same-uuid"}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkUUIDUnique(parsed)
	if !hasDiag(diags, "UUID_UNIQUE") {
		t.Error("expected UUID_UNIQUE for duplicate UUIDs")
	}
}

func TestTier1_UUIDDescriptive_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{UUID: "create-salesforce-lead"}, JSONPointer: "/code"},
	}, nil)
	diags := checkUUIDDescriptive(parsed)
	if hasDiag(diags, "UUID_DESCRIPTIVE") {
		t.Error("expected no UUID_DESCRIPTIVE for descriptive UUID")
	}
}

func TestTier1_UUIDDescriptive_Fail(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{UUID: "550e8400-e29b-41d4-a716-446655440000"}, JSONPointer: "/code"},
	}, nil)
	diags := checkUUIDDescriptive(parsed)
	if !hasDiag(diags, "UUID_DESCRIPTIVE") {
		t.Error("expected UUID_DESCRIPTIVE for UUID v4 format")
	}
}

func TestTier1_TriggerNumberZero_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "trigger", Number: intPtr(0)}, JSONPointer: "/code"},
	}, nil)
	diags := checkTriggerNumberZero(parsed)
	if hasDiag(diags, "TRIGGER_NUMBER_ZERO") {
		t.Error("expected no TRIGGER_NUMBER_ZERO when trigger is 0")
	}
}

func TestTier1_TriggerNumberZero_Fail(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "trigger", Number: intPtr(1)}, JSONPointer: "/code"},
	}, nil)
	diags := checkTriggerNumberZero(parsed)
	if !hasDiag(diags, "TRIGGER_NUMBER_ZERO") {
		t.Error("expected TRIGGER_NUMBER_ZERO when trigger number is not 0")
	}
}

func TestTier1_TriggerNumberZero_NilNumber(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "trigger", Number: nil}, JSONPointer: "/code"},
	}, nil)
	diags := checkTriggerNumberZero(parsed)
	if !hasDiag(diags, "TRIGGER_NUMBER_ZERO") {
		t.Error("expected TRIGGER_NUMBER_ZERO when trigger number is nil")
	}
}

func TestTier1_FilenameMatch_Pass(t *testing.T) {
	parsed := buildParsedRecipe("My Cool Recipe", nil, nil)
	diags := checkFilenameMatch(parsed, "my_cool_recipe.recipe.json")
	if hasDiag(diags, "FILENAME_MATCH") {
		t.Error("expected no FILENAME_MATCH")
	}
}

func TestTier1_FilenameMatch_Fail(t *testing.T) {
	parsed := buildParsedRecipe("My Cool Recipe", nil, nil)
	diags := checkFilenameMatch(parsed, "wrong_name.recipe.json")
	if !hasDiag(diags, "FILENAME_MATCH") {
		t.Error("expected FILENAME_MATCH")
	}
}

func TestTier1_FilenameMatch_EmptyFilename(t *testing.T) {
	parsed := buildParsedRecipe("My Cool Recipe", nil, nil)
	diags := checkFilenameMatch(parsed, "")
	if hasDiag(diags, "FILENAME_MATCH") {
		t.Error("expected no FILENAME_MATCH for empty filename")
	}
}

func TestTier1_ConfigNoWorkato_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", nil, []recipe.ConfigEntry{
		{Keyword: "application", Provider: "salesforce"},
	})
	diags := checkConfigNoWorkato(parsed)
	if hasDiag(diags, "CONFIG_NO_WORKATO") {
		t.Error("expected no CONFIG_NO_WORKATO")
	}
}

func TestTier1_ConfigNoWorkato_Fail(t *testing.T) {
	parsed := buildParsedRecipe("test", nil, []recipe.ConfigEntry{
		{Keyword: "application", Provider: "workato"},
	})
	diags := checkConfigNoWorkato(parsed)
	if !hasDiag(diags, "CONFIG_NO_WORKATO") {
		t.Error("expected CONFIG_NO_WORKATO")
	}
}

func TestTier1_ConfigProviderMatch_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "trigger", Provider: strPtr("salesforce")}, JSONPointer: "/code"},
	}, []recipe.ConfigEntry{
		{Keyword: "application", Provider: "salesforce"},
	})
	diags := checkConfigProviderMatch(parsed, nil)
	if hasDiag(diags, "CONFIG_PROVIDER_MATCH") {
		t.Error("expected no CONFIG_PROVIDER_MATCH")
	}
}

func TestTier1_ConfigProviderMatch_Fail(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Provider: strPtr("netsuite")}, JSONPointer: "/code/block/0"},
	}, []recipe.ConfigEntry{
		{Keyword: "application", Provider: "salesforce"},
	})
	diags := checkConfigProviderMatch(parsed, nil)
	if !hasDiag(diags, "CONFIG_PROVIDER_MATCH") {
		t.Error("expected CONFIG_PROVIDER_MATCH when provider not in config")
	}
}

func TestTier1_ConfigProviderMatch_InternalSkipped(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Provider: strPtr("workato_recipe_function")}, JSONPointer: "/code/block/0"},
		{Code: recipe.Code{Keyword: "action", Provider: strPtr("logger")}, JSONPointer: "/code/block/1"},
	}, nil)
	diags := checkConfigProviderMatch(parsed, nil)
	if hasDiag(diags, "CONFIG_PROVIDER_MATCH") {
		t.Error("expected no CONFIG_PROVIDER_MATCH for internal providers")
	}
}

func TestTier1_ConfigProviderMatch_NilProviderSkipped(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "if", Provider: nil}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkConfigProviderMatch(parsed, nil)
	if hasDiag(diags, "CONFIG_PROVIDER_MATCH") {
		t.Error("expected no CONFIG_PROVIDER_MATCH for nil provider")
	}
}

func TestTier1_ActionNameValid_Pass(t *testing.T) {
	connRules := map[string]*ConnectorRules{
		"rest": {ValidActionNames: []string{"make_request_v2"}},
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Provider: strPtr("rest"), Name: "make_request_v2"}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkActionNameValid(parsed, connRules)
	if hasDiag(diags, "ACTION_NAME_VALID") {
		t.Error("expected no ACTION_NAME_VALID for valid action name")
	}
}

func TestTier1_ActionNameValid_Fail(t *testing.T) {
	connRules := map[string]*ConnectorRules{
		"rest": {ValidActionNames: []string{"make_request_v2"}},
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Provider: strPtr("rest"), Name: "__adhoc_http_action"}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkActionNameValid(parsed, connRules)
	if !hasDiag(diags, "ACTION_NAME_VALID") {
		t.Error("expected ACTION_NAME_VALID for invalid action name")
	}
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Level != LevelError {
		t.Errorf("expected error level, got %s", diags[0].Level)
	}
}

func TestTier1_ActionNameValid_NoRulesForProvider(t *testing.T) {
	connRules := map[string]*ConnectorRules{
		"rest": {ValidActionNames: []string{"make_request_v2"}},
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Provider: strPtr("salesforce"), Name: "search_sobjects"}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkActionNameValid(parsed, connRules)
	if hasDiag(diags, "ACTION_NAME_VALID") {
		t.Error("expected no ACTION_NAME_VALID when provider has no rules")
	}
}

func TestTier1_ActionNameValid_EmptyValidList(t *testing.T) {
	connRules := map[string]*ConnectorRules{
		"rest": {ValidActionNames: []string{}},
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Provider: strPtr("rest"), Name: "anything"}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkActionNameValid(parsed, connRules)
	if hasDiag(diags, "ACTION_NAME_VALID") {
		t.Error("expected no ACTION_NAME_VALID when valid_action_names is empty")
	}
}

func TestTier1_ActionNameValid_NilConnRules(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Provider: strPtr("rest"), Name: "__adhoc_http_action"}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkActionNameValid(parsed, nil)
	if hasDiag(diags, "ACTION_NAME_VALID") {
		t.Error("expected no ACTION_NAME_VALID when connRules is nil")
	}
}

func TestTier1_ActionNameValid_NoNameField(t *testing.T) {
	connRules := map[string]*ConnectorRules{
		"rest": {ValidActionNames: []string{"make_request_v2"}},
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Provider: strPtr("rest")}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkActionNameValid(parsed, connRules)
	if hasDiag(diags, "ACTION_NAME_VALID") {
		t.Error("expected no ACTION_NAME_VALID when step has no name")
	}
}

func evalTier1BuiltinForTest(t *testing.T, parsed *recipe.ParsedRecipe, filename string, connRules map[string]*ConnectorRules) []LintDiagnostic {
	t.Helper()
	rules, err := loadBuiltinRules()
	if err != nil {
		t.Fatal(err)
	}
	ctx := &BuiltinContext{Parsed: parsed, ConnRules: connRules, Filename: filename}
	return evalCustomRules(ctx, rules, 1)
}

func TestTier1_FullIntegration(t *testing.T) {
	provider := "salesforce"
	parsed := buildParsedRecipe("Test Recipe", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "trigger", Number: intPtr(0), UUID: "trigger-step", Provider: &provider}, JSONPointer: "/code"},
		{Code: recipe.Code{Keyword: "action", Number: intPtr(1), UUID: "action-step", Provider: &provider}, JSONPointer: "/code/block/0"},
	}, []recipe.ConfigEntry{
		{Keyword: "application", Provider: "salesforce"},
	})
	diags := evalTier1BuiltinForTest(t, parsed, "test_recipe.recipe.json", nil)
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("unexpected diagnostic: %s — %s", d.RuleID, d.Message)
		}
	}
}

func TestTier1_AllDiagsAreTier1(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "trigger", Number: intPtr(5), UUID: "550e8400-e29b-41d4-a716-446655440000"}, JSONPointer: "/code"},
		{Code: recipe.Code{Keyword: "elsif", Number: intPtr(1), UUID: "550e8400-e29b-41d4-a716-446655440000"}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalTier1BuiltinForTest(t, parsed, "wrong.recipe.json", nil)
	for _, d := range diags {
		if d.Tier != 1 {
			t.Errorf("expected tier 1 for rule %s, got tier %d", d.RuleID, d.Tier)
		}
	}
}
