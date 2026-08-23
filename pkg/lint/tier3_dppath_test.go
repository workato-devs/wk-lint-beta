package lint

import (
	"encoding/json"
	"testing"

	"github.com/workato-devs/recipe-lint/pkg/recipe"
)

// objectEOS: result{booking_id, status} — a fully-declared object.
const objectEOS = `[{"name":"result","type":"object","properties":[
	{"name":"booking_id","type":"string"},
	{"name":"status","type":"string"}]}]`

// arrayEOS: Contact[] of object{FirstName, Email} — array element fields under properties.
const arrayEOS = `[{"name":"Contact","type":"array","of":"object","properties":[
	{"name":"FirstName","type":"string"},
	{"name":"Email","type":"string"}]}]`

// openObjectEOS: payload is an object with NO declared properties (dynamic/raw JSON).
const openObjectEOS = `[{"name":"payload","type":"object"}]`

func dpPath(line string, path ...interface{}) *DatapillPayload {
	return &DatapillPayload{Line: line, Path: path}
}

func aliasMapWithEOS(alias, eos string) map[string]*recipe.FlatStep {
	step := &recipe.FlatStep{}
	step.Code.As = alias
	if eos != "" {
		step.Code.ExtendedOutputSchema = json.RawMessage(eos)
	}
	return map[string]*recipe.FlatStep{alias: step}
}

func actionAliasMap(alias, provider, action, eos string) map[string]*recipe.FlatStep {
	step := &recipe.FlatStep{}
	step.Code.Keyword = "action"
	step.Code.Provider = strPtr(provider)
	step.Code.Name = action
	step.Code.As = alias
	if eos != "" {
		step.Code.ExtendedOutputSchema = json.RawMessage(eos)
	}
	return map[string]*recipe.FlatStep{alias: step}
}

func connectorOutputRules(actionSchemas string) map[string]*ConnectorRules {
	return map[string]*ConnectorRules{
		"rest": {
			ActionOutputSchemas: map[string]json.RawMessage{
				"make_request_v2": json.RawMessage(actionSchemas),
			},
		},
	}
}

func countDPPath(diags []LintDiagnostic) int {
	n := 0
	for _, d := range diags {
		if d.RuleID == "DP_PATH_RESOLVES" {
			n++
		}
	}
	return n
}

func TestCheckDPPathResolves(t *testing.T) {
	tests := []struct {
		name    string
		alias   string
		eos     string
		payload *DatapillPayload
		wantHit int
	}{
		{"valid leaf resolves", "step", objectEOS, dpPath("step", "result", "booking_id"), 0},
		{"invented field flagged", "step", objectEOS, dpPath("step", "result", "nope"), 1},
		{"array descent resolves", "search", arrayEOS, dpPath("search", "Contact", "FirstName"), 0},
		{"array index skipped, resolves", "search", arrayEOS, dpPath("search", "Contact", float64(0), "Email"), 0},
		{"array index then invented field flagged", "search", arrayEOS, dpPath("search", "Contact", float64(0), "Nope"), 1},
		{"case mismatch flagged (case-sensitive)", "search", arrayEOS, dpPath("search", "Contact", "firstname"), 1},
		{"open container accepts deeper path", "step", openObjectEOS, dpPath("step", "payload", "anything", "deep"), 0},
		{"absent EOS skipped", "step", "", dpPath("step", "result", "booking_id"), 0},
		{"unresolved alias skipped", "other", objectEOS, dpPath("ghost", "result", "booking_id"), 0},
		{"empty line skipped", "step", objectEOS, dpPath("", "result"), 0},
		{"empty path resolves to whole step output", "step", objectEOS, dpPath("step"), 0},
		{"numeric-only path remains conservative", "step", objectEOS, dpPath("step", float64(0)), 0},
		{"scalar leaf rejects deeper field", "step", objectEOS, dpPath("step", "result", "booking_id", "invented"), 1},
		{"top-level invented field flagged", "step", objectEOS, dpPath("step", "missing"), 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aliasToStep := aliasMapWithEOS(tt.alias, tt.eos)
			got := countDPPath(checkDPPathResolves(tt.payload, "/code/0/input/x", aliasToStep, nil))
			if got != tt.wantHit {
				t.Errorf("DP_PATH_RESOLVES hits = %d, want %d", got, tt.wantHit)
			}
		})
	}
}

// TestCheckDPPathResolves_RealFixture exercises the rule against a real recipe whose
// Salesforce search step materializes a Contact[...] schema (if_else_branching), confirming
// declared fields resolve and an invented one is flagged.
func TestCheckDPPathResolves_RealFixture(t *testing.T) {
	_, parsed, _ := loadAndBuild(t, "if_else_branching.recipe.json")

	aliasToStep := make(map[string]*recipe.FlatStep, len(parsed.Steps))
	for i := range parsed.Steps {
		if as := parsed.Steps[i].Code.As; as != "" {
			aliasToStep[as] = &parsed.Steps[i]
		}
	}
	if _, ok := aliasToStep["search_contact"]; !ok {
		t.Fatal("expected alias 'search_contact' in fixture")
	}

	// Declared field resolves.
	if n := countDPPath(checkDPPathResolves(dpPath("search_contact", "Contact", "FirstName"), "/p", aliasToStep, nil)); n != 0 {
		t.Errorf("declared field should resolve, got %d hits", n)
	}
	// Invented field is flagged.
	if n := countDPPath(checkDPPathResolves(dpPath("search_contact", "Contact", "EmailAddress"), "/p", aliasToStep, nil)); n != 1 {
		t.Errorf("invented field should be flagged, got %d hits", n)
	}
}

func TestCheckDPPathResolves_ConnectorOutputSchemas(t *testing.T) {
	static := `{"kind":"static","fields":[{"name":"record","type":"object","properties":[{"name":"id","type":"string"}]}]}`
	staticArray := `{"kind":"static","fields":[{"name":"records","type":"array","properties":[{"name":"id","type":"string"}]}],"future_option":{"mode":"ignored"}}`
	dynamic := `{"kind":"dynamic","intrinsic_fields":[{"name":"status_code","type":"string"},{"name":"headers","type":"object"}]}`
	partialEOS := `[{"name":"response","type":"object","properties":[{"name":"project_build","type":"object","properties":[{"name":"id","type":"string"}]}]}]`

	tests := []struct {
		name    string
		steps   map[string]*recipe.FlatStep
		rules   map[string]*ConnectorRules
		payload *DatapillPayload
		wantHit int
	}{
		{
			name:  "partial recipe EOS augmented by connector intrinsic",
			steps: actionAliasMap("rest_call", "rest", "make_request_v2", partialEOS),
			rules: connectorOutputRules(dynamic), payload: dpPath("rest_call", "status_code"), wantHit: 0,
		},
		{
			name:  "partial recipe EOS still rejects unknown non-intrinsic root",
			steps: actionAliasMap("rest_call", "rest", "make_request_v2", partialEOS),
			rules: connectorOutputRules(dynamic), payload: dpPath("rest_call", "invented"), wantHit: 1,
		},
		{
			name:  "partial recipe EOS validates its own nested fields",
			steps: actionAliasMap("rest_call", "rest", "make_request_v2", partialEOS),
			rules: connectorOutputRules(dynamic), payload: dpPath("rest_call", "response", "project_build", "id"), wantHit: 0,
		},
		{
			name:  "partial recipe EOS rejects invented nested field",
			steps: actionAliasMap("rest_call", "rest", "make_request_v2", partialEOS),
			rules: connectorOutputRules(dynamic), payload: dpPath("rest_call", "response", "project_build", "invented"), wantHit: 1,
		},
		{
			name:  "absent recipe EOS uses static connector schema",
			steps: actionAliasMap("static_call", "rest", "make_request_v2", ""),
			rules: connectorOutputRules(static), payload: dpPath("static_call", "record", "id"), wantHit: 0,
		},
		{
			name:  "absent recipe EOS rejects unknown static field",
			steps: actionAliasMap("static_call", "rest", "make_request_v2", ""),
			rules: connectorOutputRules(static), payload: dpPath("static_call", "record", "invented"), wantHit: 1,
		},
		{
			name:  "empty static connector schema is closed",
			steps: actionAliasMap("static_call", "rest", "make_request_v2", ""),
			rules: connectorOutputRules(`{"kind":"static","fields":[]}`), payload: dpPath("static_call", "invented"), wantHit: 1,
		},
		{
			name:  "static connector schema supports numeric array index and ignores future fields",
			steps: actionAliasMap("static_call", "rest", "make_request_v2", ""),
			rules: connectorOutputRules(staticArray), payload: dpPath("static_call", "records", float64(0), "id"), wantHit: 0,
		},
		{
			name:  "absent recipe EOS validates known dynamic intrinsic",
			steps: actionAliasMap("rest_call", "rest", "make_request_v2", ""),
			rules: connectorOutputRules(dynamic), payload: dpPath("rest_call", "status_code"), wantHit: 0,
		},
		{
			name:  "absent recipe EOS skips unknown dynamic root",
			steps: actionAliasMap("rest_call", "rest", "make_request_v2", ""),
			rules: connectorOutputRules(dynamic), payload: dpPath("rest_call", "runtime_field"), wantHit: 0,
		},
		{
			name:  "intrinsic open object accepts nested runtime path",
			steps: actionAliasMap("rest_call", "rest", "make_request_v2", partialEOS),
			rules: connectorOutputRules(dynamic), payload: dpPath("rest_call", "headers", "x-request-id"), wantHit: 0,
		},
		{
			name:    "missing provider rules remains conservative",
			steps:   actionAliasMap("rest_call", "rest", "make_request_v2", ""),
			payload: dpPath("rest_call", "status_code"), wantHit: 0,
		},
		{
			name:  "missing action schema remains conservative",
			steps: actionAliasMap("rest_call", "rest", "other_action", ""),
			rules: connectorOutputRules(dynamic), payload: dpPath("rest_call", "status_code"), wantHit: 0,
		},
		{
			name:  "unknown schema kind remains conservative",
			steps: actionAliasMap("rest_call", "rest", "make_request_v2", ""),
			rules: connectorOutputRules(`{"kind":"future","fields":[{"name":"status_code"}]}`), payload: dpPath("rest_call", "status_code"), wantHit: 0,
		},
		{
			name:  "malformed action schema remains conservative",
			steps: actionAliasMap("rest_call", "rest", "make_request_v2", ""),
			rules: connectorOutputRules(`{"kind":`), payload: dpPath("rest_call", "status_code"), wantHit: 0,
		},
		{
			name:  "malformed sibling action schema does not disable valid action",
			steps: actionAliasMap("rest_call", "rest", "make_request_v2", ""),
			rules: map[string]*ConnectorRules{"rest": {ActionOutputSchemas: map[string]json.RawMessage{
				"broken":          json.RawMessage(`"not-an-object"`),
				"make_request_v2": json.RawMessage(static),
			}}}, payload: dpPath("rest_call", "record", "id"), wantHit: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countDPPath(checkDPPathResolves(tt.payload, "/code/0/input/x", tt.steps, tt.rules))
			if got != tt.wantHit {
				t.Fatalf("DP_PATH_RESOLVES hits = %d, want %d", got, tt.wantHit)
			}
		})
	}
}

func TestCheckDPPathResolves_ConnectorFallbackDoesNotApplyToTrigger(t *testing.T) {
	steps := actionAliasMap("trigger", "rest", "make_request_v2", "")
	steps["trigger"].Code.Keyword = "trigger"
	rules := connectorOutputRules(`{"kind":"static","fields":[{"name":"status_code","type":"string"}]}`)

	if got := countDPPath(checkDPPathResolves(dpPath("trigger", "invented"), "/p", steps, rules)); got != 0 {
		t.Fatalf("trigger without recipe EOS must remain conservative, got %d hits", got)
	}
}
