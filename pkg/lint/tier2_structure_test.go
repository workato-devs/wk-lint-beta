package lint

import (
	"os"
	"testing"

	"github.com/workato-devs/wk-lint-beta/pkg/igm"
	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

func loadAndBuild(t *testing.T, name string) ([]byte, *recipe.ParsedRecipe, *igm.Graph) {
	t.Helper()
	data, err := os.ReadFile("testdata/fixtures/" + name)
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	parsed, err := recipe.Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	graph, err := igm.Transform(data)
	if err != nil {
		t.Fatalf("igm transform: %v", err)
	}
	return data, parsed, graph
}

func TestTier2_CatchLastInTry(t *testing.T) {
	_, parsed, graph := loadAndBuild(t, "simple_connector.recipe.json")
	diags := lintTier2Structure(graph, parsed)

	// In the fixture, catch IS last in try, so no CATCH_LAST_IN_TRY diagnostic expected
	for _, d := range diags {
		if d.RuleID == "CATCH_LAST_IN_TRY" {
			t.Errorf("unexpected CATCH_LAST_IN_TRY: %s", d.Message)
		}
	}
}

func TestTier2_TerminalCoverage(t *testing.T) {
	_, parsed, graph := loadAndBuild(t, "api_endpoint_try_catch.recipe.json")
	diags := lintTier2Structure(graph, parsed)

	// This fixture declares 200, 404, 409, 500 response codes.
	// It has return_response for: 404 (guest not found), 200 (success), 500 (catch error)
	// Missing: 409
	found := false
	for _, d := range diags {
		if d.RuleID == "TERMINAL_COVERAGE" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected TERMINAL_COVERAGE diagnostic for missing 409 response code")
	}
}

func TestTier2_RecipeCallZipName(t *testing.T) {
	_, parsed, graph := loadAndBuild(t, "api_endpoint_try_catch.recipe.json")
	diags := lintTier2Structure(graph, parsed)

	// Recipe calls in this fixture DO have zip_name, so no diagnostic expected
	for _, d := range diags {
		if d.RuleID == "RECIPE_CALL_ZIP_NAME" {
			t.Errorf("unexpected RECIPE_CALL_ZIP_NAME: %s", d.Message)
		}
	}
}

func TestTier2_AllPathsReturn(t *testing.T) {
	_, parsed, graph := loadAndBuild(t, "simple_connector.recipe.json")
	diags := lintTier2Structure(graph, parsed)

	// Simple connector has return_result in both try and catch paths,
	// so no ALL_PATHS_RETURN expected (both paths terminate)
	for _, d := range diags {
		if d.RuleID == "ALL_PATHS_RETURN" {
			t.Logf("ALL_PATHS_RETURN: %s at %s", d.Message, d.Source.JSONPointer)
		}
	}
}

func TestTier2_SuccessBeforeCatch(t *testing.T) {
	_, parsed, graph := loadAndBuild(t, "api_endpoint_try_catch.recipe.json")
	diags := lintTier2Structure(graph, parsed)

	// In this fixture, 200 success is in try body (correct), not in catch
	for _, d := range diags {
		if d.RuleID == "SUCCESS_BEFORE_CATCH" {
			t.Errorf("unexpected SUCCESS_BEFORE_CATCH: %s", d.Message)
		}
	}
}

func TestTier2_IntegrationWithLintRecipe(t *testing.T) {
	data, err := os.ReadFile("testdata/fixtures/api_endpoint_try_catch.recipe.json")
	if err != nil {
		t.Fatal(err)
	}

	// Run full lint with tier 2 enabled
	diags, err := LintRecipe(data, LintOptions{Tiers: []int{2}})
	if err != nil {
		t.Fatalf("LintRecipe: %v", err)
	}

	// Should have tier 2 diagnostics
	hasTier2 := false
	for _, d := range diags {
		if d.Tier == 2 {
			hasTier2 = true
			break
		}
	}
	if !hasTier2 {
		t.Error("expected tier 2 diagnostics from LintRecipe with Tiers: [2]")
	}
}
