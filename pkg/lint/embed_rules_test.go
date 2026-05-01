package lint

import (
	"encoding/json"
	"testing"

	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

func evalBuiltinRulesForTest(t *testing.T, parsed *recipe.ParsedRecipe) []LintDiagnostic {
	t.Helper()
	rules, err := loadBuiltinRules()
	if err != nil {
		t.Fatal(err)
	}
	return evalCustomRules(&BuiltinContext{Parsed: parsed}, rules, 1)
}

func TestBuiltinRules_Load(t *testing.T) {
	rules, err := loadBuiltinRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) == 0 {
		t.Fatal("expected at least one built-in rule")
	}
	expectedRules := []string{
		"INVALID_JSON",
		"CODE_WRAPPED_IN_RECIPE",
		"MISSING_TOP_LEVEL_KEYS",
		"CODE_NOT_OBJECT",
		"CONFIG_INVALID",
		"STEP_MISSING_KEYWORD",
		"STEP_MISSING_NUMBER",
		"NUMBER_NOT_INTEGER",
		"STEP_MISSING_UUID",
		"UUID_TOO_LONG",
		"RESPONSE_CODES_DEFINED",
		"IF_NO_PROVIDER",
		"ELSE_NO_PROVIDER",
		"CATCH_PROVIDER_NULL",
		"CATCH_HAS_AS",
		"TRY_NO_AS",
		"CATCH_HAS_RETRY",
		"NO_ELSIF",
		"STEP_NUMBERING",
		"UUID_UNIQUE",
		"UUID_DESCRIPTIVE",
		"TRIGGER_NUMBER_ZERO",
		"FILENAME_MATCH",
		"CONFIG_NO_WORKATO",
		"CONFIG_PROVIDER_MATCH",
		"ACTION_NAME_VALID",
		"ACTION_RULES",
		"DP_VALID_JSON",
		"DP_LHS_NO_FORMULA",
		"DP_INTERPOLATION_SINGLE",
		"DP_FORMULA_CONCAT",
		"DP_NO_OUTER_PARENS",
		"DP_NO_BODY_NATIVE",
		"DP_CATCH_PROVIDER",
		"EIS_MIRRORS_INPUT",
		"EIS_NAME_MATCH",
		"EIS_NESTED_MATCH",
		"EIS_NO_CONNECTOR_INTERNAL",
		"EIS_OUTPUT_MIRRORS_INPUT",
		"FORMULA_METHOD_INVALID",
		"FORMULA_FORBIDDEN_PATTERN",
		"CATCH_LAST_IN_TRY",
		"ELSE_LAST_IN_IF",
		"SUCCESS_BEFORE_CATCH",
		"TERMINAL_COVERAGE",
		"ALL_PATHS_RETURN",
		"CATCH_RETURNS_ALL_FIELDS",
		"RECIPE_CALL_ZIP_NAME",
		"DP_LINE_RESOLVES",
		"DP_PROVIDER_MATCHES",
		"DP_STEP_REACHABLE",
		"DP_TRIGGER_PATH",
	}
	ruleSet := make(map[string]bool)
	for _, r := range rules {
		ruleSet[r.RuleID] = true
	}
	for _, id := range expectedRules {
		if !ruleSet[id] {
			t.Errorf("expected %s in built-in rules", id)
		}
	}
}

func TestBuiltinRule_ResponseCodesDefined_Pass(t *testing.T) {
	provider := "workato_api_platform"
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{
			Keyword:  "trigger",
			Provider: &provider,
			Input: json.RawMessage(`{
				"request": {"content_type": "json"},
				"response": {"content_type": "json", "responses": [{"name": "200 OK"}]}
			}`),
		}, JSONPointer: "/code"},
	}, nil)
	rules, err := loadBuiltinRules()
	if err != nil {
		t.Fatal(err)
	}
	diags := evalCustomRules(&BuiltinContext{Parsed: parsed}, rules, 1)
	if hasDiag(diags, "RESPONSE_CODES_DEFINED") {
		t.Error("expected no RESPONSE_CODES_DEFINED when input.response.responses is present")
	}
}

func TestBuiltinRule_ResponseCodesDefined_Fail_MissingResponses(t *testing.T) {
	provider := "workato_api_platform"
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{
			Keyword:  "trigger",
			Provider: &provider,
			Input:    json.RawMessage(`{"request": {"content_type": "json"}}`),
		}, JSONPointer: "/code"},
	}, nil)
	rules, err := loadBuiltinRules()
	if err != nil {
		t.Fatal(err)
	}
	diags := evalCustomRules(&BuiltinContext{Parsed: parsed}, rules, 1)
	if !hasDiag(diags, "RESPONSE_CODES_DEFINED") {
		t.Error("expected RESPONSE_CODES_DEFINED when input.response.responses is missing")
	}
}

func TestBuiltinRule_ResponseCodesDefined_Fail_NilInput(t *testing.T) {
	provider := "workato_api_platform"
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{
			Keyword:  "trigger",
			Provider: &provider,
		}, JSONPointer: "/code"},
	}, nil)
	rules, err := loadBuiltinRules()
	if err != nil {
		t.Fatal(err)
	}
	diags := evalCustomRules(&BuiltinContext{Parsed: parsed}, rules, 1)
	if !hasDiag(diags, "RESPONSE_CODES_DEFINED") {
		t.Error("expected RESPONSE_CODES_DEFINED when input is nil")
	}
}

func TestBuiltinRule_ResponseCodesDefined_NonAPIPlatform(t *testing.T) {
	provider := "salesforce"
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{
			Keyword:  "trigger",
			Provider: &provider,
		}, JSONPointer: "/code"},
	}, nil)
	rules, err := loadBuiltinRules()
	if err != nil {
		t.Fatal(err)
	}
	diags := evalCustomRules(&BuiltinContext{Parsed: parsed}, rules, 1)
	if hasDiag(diags, "RESPONSE_CODES_DEFINED") {
		t.Error("expected no RESPONSE_CODES_DEFINED for non-API platform trigger")
	}
}

// --- Control flow rules ---

func TestBuiltinRule_IfNoProvider_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "if"}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if hasDiag(diags, "IF_NO_PROVIDER") {
		t.Error("expected no IF_NO_PROVIDER when provider is nil")
	}
}

func TestBuiltinRule_IfNoProvider_Fail(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "if", Provider: strPtr("salesforce")}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if !hasDiag(diags, "IF_NO_PROVIDER") {
		t.Error("expected IF_NO_PROVIDER when if has provider")
	}
}

func TestBuiltinRule_ElseNoProvider_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "else"}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if hasDiag(diags, "ELSE_NO_PROVIDER") {
		t.Error("expected no ELSE_NO_PROVIDER when provider is nil")
	}
}

func TestBuiltinRule_ElseNoProvider_Fail(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "else", Provider: strPtr("salesforce")}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if !hasDiag(diags, "ELSE_NO_PROVIDER") {
		t.Error("expected ELSE_NO_PROVIDER when else has provider")
	}
}

func TestBuiltinRule_CatchProviderNull_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "catch", As: "error_msg", Input: json.RawMessage(`{"max_retry_count":3}`)}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if hasDiag(diags, "CATCH_PROVIDER_NULL") {
		t.Error("expected no CATCH_PROVIDER_NULL when provider is nil")
	}
}

func TestBuiltinRule_CatchProviderNull_Fail(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "catch", Provider: strPtr("something"), As: "error_msg", Input: json.RawMessage(`{"max_retry_count":3}`)}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if !hasDiag(diags, "CATCH_PROVIDER_NULL") {
		t.Error("expected CATCH_PROVIDER_NULL when catch has provider")
	}
}

func TestBuiltinRule_CatchHasAs_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "catch", As: "error_msg", Input: json.RawMessage(`{"max_retry_count":3}`)}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if hasDiag(diags, "CATCH_HAS_AS") {
		t.Error("expected no CATCH_HAS_AS when as is non-empty")
	}
}

func TestBuiltinRule_CatchHasAs_Fail(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "catch", As: "", Input: json.RawMessage(`{"max_retry_count":3}`)}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if !hasDiag(diags, "CATCH_HAS_AS") {
		t.Error("expected CATCH_HAS_AS when as is empty")
	}
}

func TestBuiltinRule_TryNoAs_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "try", As: ""}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if hasDiag(diags, "TRY_NO_AS") {
		t.Error("expected no TRY_NO_AS when as is empty")
	}
}

func TestBuiltinRule_TryNoAs_Fail(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "try", As: "something"}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if !hasDiag(diags, "TRY_NO_AS") {
		t.Error("expected TRY_NO_AS when try has non-empty as")
	}
}

func TestBuiltinRule_CatchHasRetry_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "catch", As: "err", Input: json.RawMessage(`{"max_retry_count":3}`)}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if hasDiag(diags, "CATCH_HAS_RETRY") {
		t.Error("expected no CATCH_HAS_RETRY when max_retry_count is present")
	}
}

func TestBuiltinRule_CatchHasRetry_Fail(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "catch", As: "err", Input: json.RawMessage(`{"other_field":"value"}`)}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if !hasDiag(diags, "CATCH_HAS_RETRY") {
		t.Error("expected CATCH_HAS_RETRY when max_retry_count is missing")
	}
}

func TestBuiltinRule_CatchHasRetry_NilInput(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "catch", As: "err"}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if !hasDiag(diags, "CATCH_HAS_RETRY") {
		t.Error("expected CATCH_HAS_RETRY when input is nil")
	}
}

func TestBuiltinRule_NoElsif_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "if"}, JSONPointer: "/code/block/0"},
		{Code: recipe.Code{Keyword: "else"}, JSONPointer: "/code/block/1"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if hasDiag(diags, "NO_ELSIF") {
		t.Error("expected no NO_ELSIF")
	}
}

func TestBuiltinRule_NoElsif_Fail(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "elsif"}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := evalBuiltinRulesForTest(t, parsed)
	if !hasDiag(diags, "NO_ELSIF") {
		t.Error("expected NO_ELSIF for elsif keyword")
	}
}

func TestBuiltinRules_ProfileCompleteness(t *testing.T) {
	rules, err := loadBuiltinRules()
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := loadEmbeddedProfiles()
	if err != nil {
		t.Fatal(err)
	}
	standard, ok := profiles["standard"]
	if !ok {
		t.Fatal("standard profile not found")
	}
	for _, r := range rules {
		if r.RuleID == "ACTION_RULES" {
			continue
		}
		if _, ok := standard.Rules[r.RuleID]; !ok {
			t.Errorf("rule %s not found in standard profile", r.RuleID)
		}
	}
}

func TestBuiltinRegistry_Completeness(t *testing.T) {
	rules, err := loadBuiltinRules()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		if r.Assert.Builtin == nil {
			continue
		}
		name := *r.Assert.Builtin
		if _, ok := builtinRegistry[name]; !ok {
			t.Errorf("rule %s references builtin %q which is not registered", r.RuleID, name)
		}
	}
}
