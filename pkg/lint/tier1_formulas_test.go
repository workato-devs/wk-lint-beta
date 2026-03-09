package lint

import (
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

// --- Group A: Tokenizer (extractMethods) tests ---

func TestExtractMethods_SimpleChain(t *testing.T) {
	got := extractMethods("=now.to_date")
	want := []string{"to_date"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExtractMethods_MultipleChain(t *testing.T) {
	got := extractMethods("=now.in_time_zone('UTC').strftime('%Y-%m-%d')")
	want := []string{"in_time_zone", "strftime"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExtractMethods_DatapillSkipping(t *testing.T) {
	got := extractMethods("=_dp('{a.b.c}').present? ? _dp('{x.y}') : now.to_date")
	want := []string{"present?", "to_date"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExtractMethods_StringLiteralSkipping(t *testing.T) {
	got := extractMethods("='text.with.dots' + _dp('{x}')")
	if len(got) != 0 {
		t.Errorf("expected no methods, got %v", got)
	}
}

func TestExtractMethods_NullFormula(t *testing.T) {
	got := extractMethods("=null")
	if len(got) != 0 {
		t.Errorf("expected no methods for =null, got %v", got)
	}
}

func TestExtractMethods_QuestionMarkMethod(t *testing.T) {
	got := extractMethods("=_dp('{x}').blank?")
	want := []string{"blank?"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExtractMethods_NoEquals(t *testing.T) {
	got := extractMethods("just plain text")
	if len(got) != 0 {
		t.Errorf("expected no methods for non-formula, got %v", got)
	}
}

func TestExtractMethods_Empty(t *testing.T) {
	got := extractMethods("")
	if len(got) != 0 {
		t.Errorf("expected no methods for empty, got %v", got)
	}
}

func TestExtractMethods_EqualsOnly(t *testing.T) {
	got := extractMethods("=")
	if len(got) != 0 {
		t.Errorf("expected no methods for bare =, got %v", got)
	}
}

func TestExtractMethods_NestedParens(t *testing.T) {
	got := extractMethods("=_dp('{x}').gsub(/[^0-9]/, '').to_i")
	want := []string{"gsub", "to_i"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExtractMethods_TernaryWithMethods(t *testing.T) {
	got := extractMethods("=_dp('{x}').present? ? _dp('{x}').strip : 'default'")
	want := []string{"present?", "strip"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExtractMethods_DatapillOnly(t *testing.T) {
	got := extractMethods("=_dp('{step1.field}')")
	if len(got) != 0 {
		t.Errorf("expected no methods for bare datapill, got %v", got)
	}
}

// --- Group B: Validation (lintFormulaString) tests ---

func TestLintFormulaString_ValidMethods(t *testing.T) {
	diags := lintFormulaString("=now.to_date", "/input/field")
	if len(diags) != 0 {
		t.Errorf("expected no diags for valid methods, got %v", diags)
	}
}

func TestLintFormulaString_InvalidMethod(t *testing.T) {
	diags := lintFormulaString("=_dp('{x}').chomp", "/input/field")
	if !hasDiag(diags, "FORMULA_METHOD_INVALID") {
		t.Error("expected FORMULA_METHOD_INVALID for .chomp")
	}
	if len(diags) != 1 {
		t.Errorf("expected 1 diag, got %d", len(diags))
	}
}

func TestLintFormulaString_ForbiddenPattern_NowUtc(t *testing.T) {
	diags := lintFormulaString("=now.utc", "/input/field")
	if !hasDiag(diags, "FORMULA_FORBIDDEN_PATTERN") {
		t.Error("expected FORMULA_FORBIDDEN_PATTERN for now.utc")
	}
	// utc should be covered by the forbidden pattern, so no FORMULA_METHOD_INVALID
	if hasDiag(diags, "FORMULA_METHOD_INVALID") {
		t.Error("utc should be covered by forbidden pattern, not also reported as FORMULA_METHOD_INVALID")
	}
}

func TestLintFormulaString_ForbiddenPattern_ParseJson(t *testing.T) {
	diags := lintFormulaString("=_dp('{x}').parse_json", "/input/field")
	if !hasDiag(diags, "FORMULA_FORBIDDEN_PATTERN") {
		t.Error("expected FORMULA_FORBIDDEN_PATTERN for .parse_json")
	}
	if hasDiag(diags, "FORMULA_METHOD_INVALID") {
		t.Error("parse_json should be covered by forbidden pattern")
	}
}

func TestLintFormulaString_NonFormula(t *testing.T) {
	diags := lintFormulaString("just a string", "/input/field")
	if len(diags) != 0 {
		t.Errorf("expected no diags for non-formula, got %v", diags)
	}
}

func TestLintFormulaString_MultipleInvalid(t *testing.T) {
	diags := lintFormulaString("=_dp('{x}').chomp.freeze", "/input/field")
	count := countDiag(diags, "FORMULA_METHOD_INVALID")
	if count != 2 {
		t.Errorf("expected 2 FORMULA_METHOD_INVALID, got %d", count)
	}
}

func TestLintFormulaString_MixedValidAndInvalid(t *testing.T) {
	diags := lintFormulaString("=_dp('{x}').strip.chomp", "/input/field")
	if !hasDiag(diags, "FORMULA_METHOD_INVALID") {
		t.Error("expected FORMULA_METHOD_INVALID for .chomp")
	}
	if countDiag(diags, "FORMULA_METHOD_INVALID") != 1 {
		t.Error("expected exactly 1 invalid (chomp), strip is valid")
	}
}

// --- Group C: Integration (checkFormulaMethods) tests ---

func TestCheckFormulaMethods_NestedInput(t *testing.T) {
	input := rawJSON(t, map[string]interface{}{
		"body": "=_dp('{x}').chomp",
		"name": "plain text",
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Input: input}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkFormulaMethods(parsed)
	if !hasDiag(diags, "FORMULA_METHOD_INVALID") {
		t.Error("expected FORMULA_METHOD_INVALID for nested formula with .chomp")
	}
}

func TestCheckFormulaMethods_ValidFormulas(t *testing.T) {
	input := rawJSON(t, map[string]interface{}{
		"date":  "=now.to_date",
		"check": "=_dp('{x}').present?",
		"plain": "not a formula",
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Input: input}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkFormulaMethods(parsed)
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("unexpected: %s — %s at %s", d.RuleID, d.Message, d.Source.JSONPointer)
		}
	}
}

func TestCheckFormulaMethods_NilInput(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action"}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkFormulaMethods(parsed)
	if len(diags) != 0 {
		t.Errorf("expected no diags for nil input, got %d", len(diags))
	}
}

func TestCheckFormulaMethods_HashInterpolation(t *testing.T) {
	// #{} strings are not formulas (they don't start with =)
	input := rawJSON(t, map[string]interface{}{
		"url": "https://example.com/#{_dp('{x}')}",
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Input: input}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkFormulaMethods(parsed)
	if len(diags) != 0 {
		t.Errorf("expected no diags for #{} interpolation, got %d", len(diags))
	}
}

func TestCheckFormulaMethods_DeeplyNested(t *testing.T) {
	input := rawJSON(t, map[string]interface{}{
		"outer": map[string]interface{}{
			"inner": "=_dp('{x}').freeze",
		},
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Input: input}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkFormulaMethods(parsed)
	if !hasDiag(diags, "FORMULA_METHOD_INVALID") {
		t.Error("expected FORMULA_METHOD_INVALID for deeply nested formula")
	}
	if len(diags) > 0 {
		ptr := diags[0].Source.JSONPointer
		if ptr != "/code/block/0/input/outer/inner" {
			t.Errorf("expected pointer /code/block/0/input/outer/inner, got %s", ptr)
		}
	}
}

func TestCheckFormulaMethods_AllDiagsTier1(t *testing.T) {
	input := rawJSON(t, map[string]interface{}{
		"a": "=now.utc",
		"b": "=_dp('{x}').chomp",
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Input: input}, JSONPointer: "/code/block/0"},
	}, nil)
	diags := checkFormulaMethods(parsed)
	for _, d := range diags {
		if d.Tier != 1 {
			t.Errorf("expected tier 1 for %s, got %d", d.RuleID, d.Tier)
		}
	}
}

// Verify the allowlist loaded correctly
func TestFormulaAllowlist_Loaded(t *testing.T) {
	// Spot-check a few methods from different categories
	checks := []string{"present?", "blank?", "strip", "to_i", "abs", "now", "strftime", "first", "join"}
	for _, m := range checks {
		if !formulaAllowlist[m] {
			t.Errorf("expected %q in allowlist", m)
		}
	}
	// Verify known-invalid methods are NOT in allowlist
	invalids := []string{"chomp", "freeze", "utc", "parse_json", "map"}
	for _, m := range invalids {
		if formulaAllowlist[m] {
			t.Errorf("did not expect %q in allowlist", m)
		}
	}
}

func TestForbiddenPatterns_Loaded(t *testing.T) {
	if len(forbiddenPatterns) < 2 {
		t.Errorf("expected at least 2 forbidden patterns, got %d", len(forbiddenPatterns))
	}
	patterns := make([]string, len(forbiddenPatterns))
	for i, fp := range forbiddenPatterns {
		patterns[i] = fp.Pattern
	}
	sort.Strings(patterns)
	if !contains(patterns, "now.utc") {
		t.Error("expected 'now.utc' in forbidden patterns")
	}
	if !contains(patterns, ".parse_json") {
		t.Error("expected '.parse_json' in forbidden patterns")
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// Regression: existing fixtures should produce no formula diagnostics
func TestCheckFormulaMethods_ExistingFixtures(t *testing.T) {
	fixtures := []string{
		"testdata/fixtures/api_endpoint_try_catch.recipe.json",
		"testdata/fixtures/if_else_branching.recipe.json",
		"testdata/fixtures/simple_connector.recipe.json",
	}
	for _, f := range fixtures {
		t.Run(f, func(t *testing.T) {
			diags, err := LintRecipe(mustReadFile(t, f), LintOptions{
				Tiers:    []int{1},
				Filename: f,
			})
			if err != nil {
				t.Fatalf("LintRecipe error: %v", err)
			}
			for _, d := range diags {
				if d.RuleID == "FORMULA_METHOD_INVALID" || d.RuleID == "FORMULA_FORBIDDEN_PATTERN" {
					t.Errorf("unexpected formula diagnostic in fixture %s: %s — %s", f, d.RuleID, d.Message)
				}
			}
		})
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return data
}
