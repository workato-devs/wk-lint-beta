package lint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

func sfStep(name string, input json.RawMessage) recipe.FlatStep {
	return recipe.FlatStep{
		Code: recipe.Code{
			Keyword:  "action",
			Provider: strPtr("salesforce"),
			Name:     name,
			UUID:     "sf-search-001",
			Input:    input,
		},
		JSONPointer: "/code/block/0",
	}
}

// --- resolveFieldPath ---

func TestResolveFieldPath_TopLevel(t *testing.T) {
	step := &recipe.FlatStep{
		Code: recipe.Code{
			UUID:    "my-uuid",
			Name:    "search",
			Keyword: "action",
			As:      "search_step",
		},
	}
	tests := []struct {
		path string
		want string
	}{
		{"uuid", "my-uuid"},
		{"name", "search"},
		{"keyword", "action"},
		{"as", "search_step"},
	}
	for _, tt := range tests {
		val, ok := resolveFieldPath(step, tt.path)
		if !ok {
			t.Errorf("resolveFieldPath(%q) not found", tt.path)
			continue
		}
		if val != tt.want {
			t.Errorf("resolveFieldPath(%q) = %v, want %v", tt.path, val, tt.want)
		}
	}
}

func TestResolveFieldPath_Provider(t *testing.T) {
	step := &recipe.FlatStep{Code: recipe.Code{Provider: strPtr("salesforce")}}
	val, ok := resolveFieldPath(step, "provider")
	if !ok || val != "salesforce" {
		t.Errorf("expected 'salesforce', got %v (ok=%v)", val, ok)
	}

	step2 := &recipe.FlatStep{Code: recipe.Code{}}
	_, ok2 := resolveFieldPath(step2, "provider")
	if ok2 {
		t.Error("expected false for nil provider")
	}
}

func TestResolveFieldPath_NestedInput(t *testing.T) {
	step := &recipe.FlatStep{
		Code: recipe.Code{
			Input: json.RawMessage(`{"sobject_name":"Account","limit":50}`),
		},
	}
	val, ok := resolveFieldPath(step, "input.sobject_name")
	if !ok || val != "Account" {
		t.Errorf("input.sobject_name = %v (ok=%v)", val, ok)
	}
	val2, ok2 := resolveFieldPath(step, "input.limit")
	if !ok2 {
		t.Error("input.limit not found")
	}
	if val2.(float64) != 50 {
		t.Errorf("input.limit = %v, want 50", val2)
	}
}

func TestResolveFieldPath_Missing(t *testing.T) {
	step := &recipe.FlatStep{
		Code: recipe.Code{Input: json.RawMessage(`{"a":"b"}`)},
	}
	_, ok := resolveFieldPath(step, "input.missing")
	if ok {
		t.Error("expected false for missing nested key")
	}
	_, ok2 := resolveFieldPath(step, "unknown_top_level")
	if ok2 {
		t.Error("expected false for unknown top-level path")
	}
}

func TestResolveFieldPath_NilStep(t *testing.T) {
	_, ok := resolveFieldPath(nil, "uuid")
	if ok {
		t.Error("expected false for nil step")
	}
}

// --- matchesWhere ---

func TestMatchesWhere_NilSelector(t *testing.T) {
	step := &recipe.FlatStep{Code: recipe.Code{Keyword: "action"}}
	if !matchesWhere(step, nil) {
		t.Error("nil selector should match everything")
	}
}

func TestMatchesWhere_Keyword(t *testing.T) {
	step := &recipe.FlatStep{Code: recipe.Code{Keyword: "action"}}
	if !matchesWhere(step, &StepSelector{Keyword: StringOrArray{"action"}}) {
		t.Error("should match action keyword")
	}
	if matchesWhere(step, &StepSelector{Keyword: StringOrArray{"trigger"}}) {
		t.Error("should not match trigger keyword")
	}
}

func TestMatchesWhere_Provider(t *testing.T) {
	step := &recipe.FlatStep{Code: recipe.Code{Provider: strPtr("salesforce")}}
	if !matchesWhere(step, &StepSelector{Provider: StringOrArray{"salesforce"}}) {
		t.Error("should match salesforce")
	}
	if matchesWhere(step, &StepSelector{Provider: StringOrArray{"rest"}}) {
		t.Error("should not match rest")
	}
}

func TestMatchesWhere_ActionName(t *testing.T) {
	step := &recipe.FlatStep{Code: recipe.Code{Name: "search_sobjects"}}
	if !matchesWhere(step, &StepSelector{ActionName: StringOrArray{"search_sobjects"}}) {
		t.Error("should match")
	}
}

func TestMatchesWhere_ArrayValues(t *testing.T) {
	step := &recipe.FlatStep{Code: recipe.Code{Keyword: "action", Provider: strPtr("salesforce")}}
	sel := &StepSelector{
		Keyword:  StringOrArray{"action", "trigger"},
		Provider: StringOrArray{"salesforce", "rest"},
	}
	if !matchesWhere(step, sel) {
		t.Error("should match with array selectors")
	}
}

// --- Assertion evaluators ---

func TestEvalAssertion_FieldExists(t *testing.T) {
	step := &recipe.FlatStep{Code: recipe.Code{UUID: "test-001"}}
	a := Assertion{FieldExists: &AssertFieldPath{Path: "uuid"}}
	if !evalAssertion(step, nil, a) {
		t.Error("uuid exists, should pass")
	}
	a2 := Assertion{FieldExists: &AssertFieldPath{Path: "name"}}
	if evalAssertion(step, nil, a2) {
		t.Error("name is empty, should fail")
	}
}

func TestEvalAssertion_FieldAbsent(t *testing.T) {
	step := &recipe.FlatStep{Code: recipe.Code{UUID: "test-001"}}
	a := Assertion{FieldAbsent: &AssertFieldPath{Path: "name"}}
	if !evalAssertion(step, nil, a) {
		t.Error("name is empty, should pass")
	}
	a2 := Assertion{FieldAbsent: &AssertFieldPath{Path: "uuid"}}
	if evalAssertion(step, nil, a2) {
		t.Error("uuid exists, should fail")
	}
}

func TestEvalAssertion_FieldMatches(t *testing.T) {
	step := &recipe.FlatStep{Code: recipe.Code{UUID: "sf-search-001"}}
	a := Assertion{FieldMatches: &AssertFieldMatches{Path: "uuid", Pattern: "^sf-"}}
	if !evalAssertion(step, nil, a) {
		t.Error("uuid starts with sf-, should match")
	}
	a2 := Assertion{FieldMatches: &AssertFieldMatches{Path: "uuid", Pattern: "^rest-"}}
	if evalAssertion(step, nil, a2) {
		t.Error("uuid does not start with rest-, should not match")
	}
}

func TestEvalAssertion_FieldMatches_InvalidRegex(t *testing.T) {
	step := &recipe.FlatStep{Code: recipe.Code{UUID: "test"}}
	a := Assertion{FieldMatches: &AssertFieldMatches{Path: "uuid", Pattern: "[invalid"}}
	if evalAssertion(step, nil, a) {
		t.Error("invalid regex should return false, not panic")
	}
}

func TestEvalAssertion_FieldEquals(t *testing.T) {
	step := &recipe.FlatStep{Code: recipe.Code{Keyword: "action"}}
	a := Assertion{FieldEquals: &AssertFieldEquals{Path: "keyword", Value: "action"}}
	if !evalAssertion(step, nil, a) {
		t.Error("keyword is action, should match")
	}
	a2 := Assertion{FieldEquals: &AssertFieldEquals{Path: "keyword", Value: "trigger"}}
	if evalAssertion(step, nil, a2) {
		t.Error("keyword is action not trigger, should not match")
	}
}

func TestEvalAssertion_StepCount(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "trigger"}, JSONPointer: "/code"},
		{Code: recipe.Code{Keyword: "action"}, JSONPointer: "/code/block/0"},
		{Code: recipe.Code{Keyword: "action"}, JSONPointer: "/code/block/1"},
	}, nil)

	max1 := 1
	a := Assertion{StepCount: &AssertStepCount{
		Where: &StepSelector{Keyword: StringOrArray{"action"}},
		Max:   &max1,
	}}
	if evalAssertion(nil, parsed, a) {
		t.Error("2 actions with max 1, should fail")
	}

	max5 := 5
	a2 := Assertion{StepCount: &AssertStepCount{
		Where: &StepSelector{Keyword: StringOrArray{"action"}},
		Max:   &max5,
	}}
	if !evalAssertion(nil, parsed, a2) {
		t.Error("2 actions with max 5, should pass")
	}

	exact2 := 2
	a3 := Assertion{StepCount: &AssertStepCount{
		Where: &StepSelector{Keyword: StringOrArray{"action"}},
		Exact: &exact2,
	}}
	if !evalAssertion(nil, parsed, a3) {
		t.Error("2 actions with exact 2, should pass")
	}
}

func TestEvalAssertion_StepCount_Min(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "trigger"}, JSONPointer: "/code"},
	}, nil)
	min1 := 1
	a := Assertion{StepCount: &AssertStepCount{
		Where: &StepSelector{Keyword: StringOrArray{"action"}},
		Min:   &min1,
	}}
	if evalAssertion(nil, parsed, a) {
		t.Error("0 actions with min 1, should fail")
	}
}

func TestEvalAssertion_EISEmpty(t *testing.T) {
	boolTrue := true
	step := &recipe.FlatStep{Code: recipe.Code{}}
	if !evalAssertion(step, nil, Assertion{EISEmpty: &boolTrue}) {
		t.Error("nil EIS should pass")
	}

	step2 := &recipe.FlatStep{Code: recipe.Code{ExtendedInputSchema: json.RawMessage(`null`)}}
	if !evalAssertion(step2, nil, Assertion{EISEmpty: &boolTrue}) {
		t.Error("null EIS should pass")
	}

	step3 := &recipe.FlatStep{Code: recipe.Code{ExtendedInputSchema: json.RawMessage(`[]`)}}
	if !evalAssertion(step3, nil, Assertion{EISEmpty: &boolTrue}) {
		t.Error("empty array EIS should pass")
	}

	step4 := &recipe.FlatStep{Code: recipe.Code{ExtendedInputSchema: json.RawMessage(`[{"name":"field"}]`)}}
	if evalAssertion(step4, nil, Assertion{EISEmpty: &boolTrue}) {
		t.Error("non-empty EIS should fail")
	}
}

func TestEvalAssertion_EISFieldType(t *testing.T) {
	eis := json.RawMessage(`[{"name":"limit","type":"integer","parse_output":"integer_conversion"}]`)
	step := &recipe.FlatStep{Code: recipe.Code{ExtendedInputSchema: eis}}

	a := Assertion{EISFieldType: &AssertEISFieldType{Name: "limit", Type: "integer"}}
	if !evalAssertion(step, nil, a) {
		t.Error("limit is integer, should pass")
	}

	a2 := Assertion{EISFieldType: &AssertEISFieldType{Name: "limit", Type: "string"}}
	if evalAssertion(step, nil, a2) {
		t.Error("limit is not string, should fail")
	}

	a3 := Assertion{EISFieldType: &AssertEISFieldType{Name: "limit", ParseOutput: "integer_conversion"}}
	if !evalAssertion(step, nil, a3) {
		t.Error("parse_output matches, should pass")
	}

	a4 := Assertion{EISFieldType: &AssertEISFieldType{Name: "missing_field", Type: "string"}}
	if evalAssertion(step, nil, a4) {
		t.Error("field not in EIS, should fail")
	}
}

func TestEvalAssertion_AllOf(t *testing.T) {
	step := &recipe.FlatStep{Code: recipe.Code{UUID: "sf-001", Keyword: "action"}}
	a := Assertion{AllOf: []Assertion{
		{FieldExists: &AssertFieldPath{Path: "uuid"}},
		{FieldEquals: &AssertFieldEquals{Path: "keyword", Value: "action"}},
	}}
	if !evalAssertion(step, nil, a) {
		t.Error("both true, all_of should pass")
	}

	a2 := Assertion{AllOf: []Assertion{
		{FieldExists: &AssertFieldPath{Path: "uuid"}},
		{FieldEquals: &AssertFieldEquals{Path: "keyword", Value: "trigger"}},
	}}
	if evalAssertion(step, nil, a2) {
		t.Error("second is false, all_of should fail")
	}
}

func TestEvalAssertion_AnyOf(t *testing.T) {
	step := &recipe.FlatStep{Code: recipe.Code{Keyword: "action"}}
	a := Assertion{AnyOf: []Assertion{
		{FieldEquals: &AssertFieldEquals{Path: "keyword", Value: "trigger"}},
		{FieldEquals: &AssertFieldEquals{Path: "keyword", Value: "action"}},
	}}
	if !evalAssertion(step, nil, a) {
		t.Error("second is true, any_of should pass")
	}

	a2 := Assertion{AnyOf: []Assertion{
		{FieldEquals: &AssertFieldEquals{Path: "keyword", Value: "trigger"}},
		{FieldEquals: &AssertFieldEquals{Path: "keyword", Value: "if"}},
	}}
	if evalAssertion(step, nil, a2) {
		t.Error("none true, any_of should fail")
	}
}

// --- evalCustomRules integration ---

func TestEvalCustomRules_RecipeScope(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "trigger"}, JSONPointer: "/code"},
		{Code: recipe.Code{Keyword: "action"}, JSONPointer: "/code/block/0"},
		{Code: recipe.Code{Keyword: "action"}, JSONPointer: "/code/block/1"},
	}, nil)

	max1 := 1
	rules := []CustomRule{{
		RuleID:  "MAX_ONE_ACTION",
		Tier:    1,
		Level:   "error",
		Message: "Recipe must have at most one action step",
		Scope:   "recipe",
		Assert:  Assertion{StepCount: &AssertStepCount{Where: &StepSelector{Keyword: StringOrArray{"action"}}, Max: &max1}},
	}}

	diags := evalCustomRules(&BuiltinContext{Parsed: parsed}, rules, 1)
	if !hasDiag(diags, "MAX_ONE_ACTION") {
		t.Error("expected MAX_ONE_ACTION diagnostic")
	}
	if len(diags) != 1 {
		t.Errorf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Level != "error" {
		t.Errorf("expected error level, got %s", diags[0].Level)
	}
}

func TestEvalCustomRules_StepScope(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "trigger", Provider: strPtr("workato_api_platform")}, JSONPointer: "/code"},
		{Code: recipe.Code{Keyword: "action", Provider: strPtr("salesforce"), Name: "search_sobjects", Input: json.RawMessage(`{"query":"test"}`)}, JSONPointer: "/code/block/0"},
	}, nil)

	rules := []CustomRule{{
		RuleID:  "SEARCH_NEEDS_LIMIT",
		Tier:    1,
		Level:   "warn",
		Message: "Search action missing limit in input",
		Scope:   "step",
		Where:   &StepSelector{Provider: StringOrArray{"salesforce"}, ActionName: StringOrArray{"search_sobjects"}},
		Assert:  Assertion{FieldExists: &AssertFieldPath{Path: "input.limit"}},
	}}

	diags := evalCustomRules(&BuiltinContext{Parsed: parsed}, rules, 1)
	if !hasDiag(diags, "SEARCH_NEEDS_LIMIT") {
		t.Error("expected SEARCH_NEEDS_LIMIT diagnostic")
	}
	if len(diags) != 1 {
		t.Errorf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Source == nil || diags[0].Source.JSONPointer != "/code/block/0" {
		t.Error("expected source pointer to step")
	}
}

func TestEvalCustomRules_TierFiltering(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", UUID: "test"}, JSONPointer: "/code/block/0"},
	}, nil)

	rules := []CustomRule{
		{RuleID: "TIER1_RULE", Tier: 1, Level: "warn", Message: "t1", Scope: "step", Assert: Assertion{FieldAbsent: &AssertFieldPath{Path: "uuid"}}},
		{RuleID: "TIER2_RULE", Tier: 2, Level: "warn", Message: "t2", Scope: "step", Assert: Assertion{FieldAbsent: &AssertFieldPath{Path: "uuid"}}},
	}

	diags := evalCustomRules(&BuiltinContext{Parsed: parsed}, rules, 1)
	if !hasDiag(diags, "TIER1_RULE") {
		t.Error("should include tier 1 rule")
	}
	if hasDiag(diags, "TIER2_RULE") {
		t.Error("should not include tier 2 rule when running tier 1")
	}
}

func TestEvalCustomRules_StepScope_WhereFilters(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Provider: strPtr("salesforce"), UUID: "sf-001"}, JSONPointer: "/code/block/0"},
		{Code: recipe.Code{Keyword: "action", Provider: strPtr("rest"), UUID: "rest-001"}, JSONPointer: "/code/block/1"},
	}, nil)

	rules := []CustomRule{{
		RuleID:  "SF_UUID_PREFIX",
		Tier:    1,
		Level:   "warn",
		Message: "SF step UUID must start with sf-",
		Scope:   "step",
		Where:   &StepSelector{Provider: StringOrArray{"salesforce"}},
		Assert:  Assertion{FieldMatches: &AssertFieldMatches{Path: "uuid", Pattern: "^sf-"}},
	}}

	diags := evalCustomRules(&BuiltinContext{Parsed: parsed}, rules, 1)
	if hasDiag(diags, "SF_UUID_PREFIX") {
		t.Error("sf-001 matches ^sf-, should not produce diagnostic")
	}
}

func TestEvalCustomRules_Pass(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{Code: recipe.Code{Keyword: "action", Provider: strPtr("salesforce"), Name: "search_sobjects", Input: json.RawMessage(`{"limit":50}`)}, JSONPointer: "/code/block/0"},
	}, nil)

	rules := []CustomRule{{
		RuleID: "HAS_LIMIT", Tier: 1, Level: "warn", Message: "missing limit", Scope: "step",
		Where:  &StepSelector{ActionName: StringOrArray{"search_sobjects"}},
		Assert: Assertion{FieldExists: &AssertFieldPath{Path: "input.limit"}},
	}}

	diags := evalCustomRules(&BuiltinContext{Parsed: parsed}, rules, 1)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics, got %d", len(diags))
	}
}

// --- StringOrArray ---

func TestStringOrArray_UnmarshalString(t *testing.T) {
	var s StringOrArray
	if err := s.UnmarshalJSON([]byte(`"action"`)); err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 || s[0] != "action" {
		t.Errorf("expected [action], got %v", s)
	}
}

func TestStringOrArray_UnmarshalArray(t *testing.T) {
	var s StringOrArray
	if err := s.UnmarshalJSON([]byte(`["action","trigger"]`)); err != nil {
		t.Fatal(err)
	}
	if len(s) != 2 {
		t.Errorf("expected 2 elements, got %d", len(s))
	}
}

func TestStringOrArray_Contains(t *testing.T) {
	s := StringOrArray{"a", "b", "c"}
	if !s.Contains("b") {
		t.Error("should contain b")
	}
	if s.Contains("d") {
		t.Error("should not contain d")
	}
}

// --- expandMessage ---

func TestExpandMessage(t *testing.T) {
	got := expandMessage("missing {field_name} in {missing_location}", map[string]string{
		"field_name":       "sobject_name",
		"missing_location": "input",
	})
	want := "missing sobject_name in input"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpandMessage_NoVars(t *testing.T) {
	msg := "static message"
	got := expandMessage(msg, nil)
	if got != msg {
		t.Errorf("got %q, want %q", got, msg)
	}
}

// --- LoadCustomRules ---

func TestLoadCustomRules_FromProjectDir(t *testing.T) {
	tmpDir := t.TempDir()
	rulesDir := filepath.Join(tmpDir, ".wklint", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ruleJSON := `{
		"version": "0.2.0",
		"rules": [{
			"rule_id": "TEST_RULE",
			"tier": 1,
			"level": "warn",
			"message": "test message",
			"scope": "step",
			"assert": { "field_exists": { "path": "input.x" } }
		}]
	}`
	if err := os.WriteFile(filepath.Join(rulesDir, "test.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, warnings, err := LoadCustomRules("", tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].RuleID != "TEST_RULE" {
		t.Errorf("expected TEST_RULE, got %s", rules[0].RuleID)
	}
}

func TestLoadCustomRules_FromSkillsDir(t *testing.T) {
	tmpDir := t.TempDir()
	connDir := filepath.Join(tmpDir, "salesforce-recipes")
	if err := os.MkdirAll(connDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ruleJSON := `{
		"version": "0.2.0",
		"connector": "salesforce",
		"rules": [{
			"rule_id": "SF_CUSTOM",
			"tier": 1,
			"level": "warn",
			"message": "sf custom",
			"scope": "step",
			"assert": { "field_exists": { "path": "input.sobject_name" } }
		}]
	}`
	if err := os.WriteFile(filepath.Join(connDir, "lint-rules.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, _, err := LoadCustomRules(tmpDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].RuleID != "SF_CUSTOM" {
		t.Errorf("expected SF_CUSTOM, got %s", rules[0].RuleID)
	}
}

func TestLoadCustomRules_EmptyPaths(t *testing.T) {
	rules, _, err := LoadCustomRules("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

func TestLoadCustomRules_V010FileIgnored(t *testing.T) {
	tmpDir := t.TempDir()
	connDir := filepath.Join(tmpDir, "salesforce-recipes")
	if err := os.MkdirAll(connDir, 0o755); err != nil {
		t.Fatal(err)
	}

	oldJSON := `{
		"version": "0.1.0",
		"connector": "salesforce",
		"connector_internals": ["sobject_name"],
		"valid_action_names": ["search_sobjects"],
		"action_rules": []
	}`
	if err := os.WriteFile(filepath.Join(connDir, "lint-rules.json"), []byte(oldJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, _, err := LoadCustomRules(tmpDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("v0.1.0 files should yield 0 custom rules, got %d", len(rules))
	}
}

func TestLoadCustomRules_MalformedFileWarns(t *testing.T) {
	tmpDir := t.TempDir()
	rulesDir := filepath.Join(tmpDir, ".wklint", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "bad.json"), []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, warnings, err := LoadCustomRules("", tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 valid rules, got %d", len(rules))
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].RuleID != "CUSTOM_RULE_INVALID" {
		t.Errorf("expected CUSTOM_RULE_INVALID, got %s", warnings[0].RuleID)
	}
}

func TestLoadCustomRules_InvalidRuleWarns(t *testing.T) {
	tmpDir := t.TempDir()
	rulesDir := filepath.Join(tmpDir, ".wklint", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ruleJSON := `{
		"version": "0.2.0",
		"rules": [{
			"rule_id": "TYPO_RULE",
			"tier": 1,
			"level": "warn",
			"message": "has typo",
			"scope": "step",
			"assert": { "feild_exists": { "path": "input.x" } }
		}]
	}`
	if err := os.WriteFile(filepath.Join(rulesDir, "typo.json"), []byte(ruleJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, warnings, err := LoadCustomRules("", tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 valid rules (typo rule rejected), got %d", len(rules))
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for invalid assertion, got %d", len(warnings))
	}
	if warnings[0].RuleID != "CUSTOM_RULE_INVALID" {
		t.Errorf("expected CUSTOM_RULE_INVALID, got %s", warnings[0].RuleID)
	}
}

func TestEvalAssertion_EmptyAssertionFails(t *testing.T) {
	step := &recipe.FlatStep{Code: recipe.Code{UUID: "test"}}
	if evalAssertion(step, nil, Assertion{}) {
		t.Error("empty assertion should return false, not silently pass")
	}
}
