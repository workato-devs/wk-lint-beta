package lint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConnectorRules_ActionOutputSchemasBackwardCompatible(t *testing.T) {
	skillsDir := t.TempDir()
	legacyDir := filepath.Join(skillsDir, "legacy")
	restDir := filepath.Join(skillsDir, "rest")
	for _, dir := range []string{legacyDir, restDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	legacy := `{
		"version":"0.2.0",
		"connector":"legacy",
		"connector_internals":[],
		"action_rules":[]
	}`
	rest := `{
		"version":"0.1.0",
		"connector":"rest",
		"connector_internals":[],
		"action_internals":{"make_request_v2":["request"]},
		"action_rules":[],
		"action_output_schemas":{
			"make_request_v2":{
				"kind":"dynamic",
				"intrinsic_fields":[{"name":"status_code","type":"string"}]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(legacyDir, "lint-rules.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(restDir, "lint-rules.json"), []byte(rest), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, err := LoadConnectorRules(skillsDir)
	if err != nil {
		t.Fatal(err)
	}
	if rules["legacy"] == nil {
		t.Fatal("legacy connector rules must continue to load")
	}
	if got := rules["rest"].ActionInternals["make_request_v2"]; len(got) != 1 || got[0] != "request" {
		t.Fatalf("unexpected action internals: %#v", got)
	}
	raw, ok := rules["rest"].ActionOutputSchemas["make_request_v2"]
	if !ok {
		t.Fatal("expected make_request_v2 output schema")
	}
	var decoded ActionOutputSchema
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Kind != "dynamic" || len(decoded.IntrinsicFields) != 1 || decoded.IntrinsicFields[0].Name != "status_code" {
		t.Fatalf("unexpected decoded action output schema: %#v", decoded)
	}
}

func TestLoadConnectorRules_MalformedActionSchemaDoesNotDropSiblingAction(t *testing.T) {
	skillsDir := t.TempDir()
	restDir := filepath.Join(skillsDir, "rest")
	if err := os.MkdirAll(restDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{
		"version":"0.3.0",
		"connector":"rest",
		"connector_internals":["request"],
		"action_rules":[],
		"action_output_schemas":{
			"broken":"not-an-object",
			"make_request_v2":{"kind":"dynamic","intrinsic_fields":[{"name":"status_code"}]}
		}
	}`
	if err := os.WriteFile(filepath.Join(restDir, "lint-rules.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rules, err := LoadConnectorRules(skillsDir)
	if err != nil {
		t.Fatal(err)
	}
	if rules["rest"] == nil {
		t.Fatal("one malformed action schema must not drop the connector")
	}
	var good ActionOutputSchema
	if err := json.Unmarshal(rules["rest"].ActionOutputSchemas["make_request_v2"], &good); err != nil {
		t.Fatal(err)
	}
	if good.Kind != "dynamic" {
		t.Fatalf("sibling action schema was not retained: %#v", good)
	}
}
