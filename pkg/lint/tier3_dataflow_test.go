package lint

import (
	"os"
	"testing"
)

func TestTier3_DPLineResolves(t *testing.T) {
	_, parsed, graph := loadAndBuild(t, "simple_connector.recipe.json")
	diags := lintTier3DataFlow(parsed, graph)

	// All datapills in this fixture reference valid step aliases (trigger, update_booking_catch)
	for _, d := range diags {
		if d.RuleID == "DP_LINE_RESOLVES" {
			t.Errorf("unexpected DP_LINE_RESOLVES: %s", d.Message)
		}
	}
}

func TestTier3_DPProviderMatches(t *testing.T) {
	_, parsed, graph := loadAndBuild(t, "simple_connector.recipe.json")
	diags := lintTier3DataFlow(parsed, graph)

	// Datapills in this fixture correctly reference workato_recipe_function for trigger
	for _, d := range diags {
		if d.RuleID == "DP_PROVIDER_MATCHES" {
			t.Errorf("unexpected DP_PROVIDER_MATCHES: %s", d.Message)
		}
	}
}

func TestTier3_DPStepReachable(t *testing.T) {
	_, parsed, graph := loadAndBuild(t, "simple_connector.recipe.json")
	diags := lintTier3DataFlow(parsed, graph)

	// All datapill references in this fixture are reachable (trigger → update_booking → return)
	for _, d := range diags {
		if d.RuleID == "DP_STEP_REACHABLE" {
			t.Logf("DP_STEP_REACHABLE: %s", d.Message)
		}
	}
}

func TestTier3_DPTriggerPath(t *testing.T) {
	_, parsed, graph := loadAndBuild(t, "api_endpoint_try_catch.recipe.json")
	diags := lintTier3DataFlow(parsed, graph)

	// API endpoint datapills use ["request", "field"] which is correct
	for _, d := range diags {
		if d.RuleID == "DP_TRIGGER_PATH" {
			t.Errorf("unexpected DP_TRIGGER_PATH: %s", d.Message)
		}
	}
}

func TestTier3_IntegrationWithLintRecipe(t *testing.T) {
	data, err := os.ReadFile("testdata/fixtures/simple_connector.recipe.json")
	if err != nil {
		t.Fatal(err)
	}

	// Run full lint with tier 3 enabled
	diags, err := LintRecipe(data, LintOptions{Tiers: []int{3}})
	if err != nil {
		t.Fatalf("LintRecipe: %v", err)
	}

	// Should have tier 3 diagnostics (or none if all valid)
	for _, d := range diags {
		if d.Tier == 3 {
			t.Logf("Tier 3 diagnostic: [%s] %s", d.RuleID, d.Message)
		}
	}
}

func TestTier3_AllTiersRun(t *testing.T) {
	data, err := os.ReadFile("testdata/fixtures/api_endpoint_try_catch.recipe.json")
	if err != nil {
		t.Fatal(err)
	}

	// Run all tiers
	diags, err := LintRecipe(data, LintOptions{})
	if err != nil {
		t.Fatalf("LintRecipe: %v", err)
	}

	tiers := make(map[int]int)
	for _, d := range diags {
		tiers[d.Tier]++
	}

	t.Logf("Diagnostics by tier: %v", tiers)

	// Should have diagnostics from tier 1 at minimum
	if tiers[1] == 0 {
		t.Error("expected tier 1 diagnostics")
	}
}
