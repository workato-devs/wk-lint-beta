package lint

import (
	"encoding/json"
	"testing"

	"github.com/workato-devs/recipe-lint/pkg/recipe"
)

func TestEIS_MIRRORS_INPUT_Warn(t *testing.T) {
	eis := json.RawMessage(`[{"name":"name","label":"Name","type":"string"}]`)
	input := rawJSON(t, map[string]interface{}{
		"name":          "Alice",
		"missing_field": "value",
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("salesforce"),
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	if !hasDiag(diags, "EIS_MIRRORS_INPUT") {
		t.Error("expected EIS_MIRRORS_INPUT for input key not in EIS")
	}
}

func TestEIS_MIRRORS_INPUT_Pass(t *testing.T) {
	eis := json.RawMessage(`[{"name":"name","label":"Name","type":"string"},{"name":"email","label":"Email","type":"string"}]`)
	input := rawJSON(t, map[string]interface{}{
		"name":  "Alice",
		"email": "alice@example.com",
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("salesforce"),
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	if hasDiag(diags, "EIS_MIRRORS_INPUT") {
		t.Error("unexpected EIS_MIRRORS_INPUT when all input keys in EIS")
	}
}

func TestEIS_NAME_MATCH_Warn(t *testing.T) {
	eis := json.RawMessage(`[{"name":"extra_field","label":"Extra","type":"string"}]`)
	input := rawJSON(t, map[string]interface{}{
		"other_field": "value",
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("salesforce"),
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	if !hasDiag(diags, "EIS_NAME_MATCH") {
		t.Error("expected EIS_NAME_MATCH for EIS field not in input")
	}
}

func TestEIS_NESTED_MATCH_Warn(t *testing.T) {
	eis := json.RawMessage(`[{"name":"address","label":"Address","type":"object"}]`)
	input := rawJSON(t, map[string]interface{}{
		"address": map[string]interface{}{
			"street": "123 Main St",
			"city":   "NYC",
		},
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("salesforce"),
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	if !hasDiag(diags, "EIS_NESTED_MATCH") {
		t.Error("expected EIS_NESTED_MATCH for nested input without EIS properties")
	}
}

func TestEIS_NESTED_MATCH_Pass(t *testing.T) {
	eis := json.RawMessage(`[{"name":"address","label":"Address","type":"object","properties":[{"name":"street","type":"string"},{"name":"city","type":"string"}]}]`)
	input := rawJSON(t, map[string]interface{}{
		"address": map[string]interface{}{
			"street": "123 Main St",
			"city":   "NYC",
		},
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("salesforce"),
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	if hasDiag(diags, "EIS_NESTED_MATCH") {
		t.Error("unexpected EIS_NESTED_MATCH when EIS has properties")
	}
}

func TestEIS_NO_CONNECTOR_INTERNAL_Warn(t *testing.T) {
	eis := json.RawMessage(`[{"name":"action_name","label":"Action","type":"string"},{"name":"real_field","label":"Real","type":"string"}]`)
	input := rawJSON(t, map[string]interface{}{
		"action_name": "create",
		"real_field":  "value",
	})
	connRules := map[string]*ConnectorRules{
		"salesforce": {
			ConnectorInternals: []string{"action_name"},
		},
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("salesforce"),
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, connRules)
	if !hasDiag(diags, "EIS_NO_CONNECTOR_INTERNAL") {
		t.Error("expected EIS_NO_CONNECTOR_INTERNAL for connector-internal field in EIS")
	}
}

func TestEIS_RESTConnectorInternals_NoFalsePositive(t *testing.T) {
	eis := json.RawMessage(`[{"name":"response","type":"object"}]`)
	input := rawJSON(t, map[string]interface{}{
		"response":                     map[string]interface{}{"output_type": "json"},
		"request":                      map[string]interface{}{"method": "GET"},
		"request_name":                 "Get project build",
		"disable_retries":              true,
		"wait_for_response":            false,
		"enable_streaming":             false,
		"completion_threshold_seconds": 120,
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{{
		Code: recipe.Code{
			Keyword:             "action",
			Provider:            strPtr("rest"),
			Name:                "make_request_v2",
			Input:               input,
			ExtendedInputSchema: eis,
		},
		JSONPointer: "/code/block/0",
	}}, nil)
	connRules := map[string]*ConnectorRules{
		"rest": {
			ActionInternals: map[string][]string{
				"make_request_v2": {
					"completion_threshold_seconds",
					"disable_retries",
					"enable_streaming",
					"request",
					"request_name",
					"wait_for_response",
				},
			},
		},
	}

	for _, d := range checkEIS(parsed, connRules) {
		if d.RuleID == "EIS_MIRRORS_INPUT" {
			t.Errorf("unexpected EIS_MIRRORS_INPUT for REST connector field: %s", d.Message)
		}
	}
}

func TestEIS_ActionInternalsDoNotApplyToTrigger(t *testing.T) {
	eis := json.RawMessage(`[{"name":"request","type":"object"}]`)
	parsed := buildParsedRecipe("test", []recipe.FlatStep{{
		Code: recipe.Code{
			Keyword:             "trigger",
			Provider:            strPtr("rest"),
			Name:                "new_poll_event",
			Input:               rawJSON(t, map[string]interface{}{"request": map[string]interface{}{"path": "events"}}),
			ExtendedInputSchema: eis,
		},
		JSONPointer: "/code",
	}}, nil)
	connRules := map[string]*ConnectorRules{
		"rest": {
			ActionInternals: map[string][]string{
				"make_request_v2": {"request"},
			},
		},
	}

	for _, d := range checkEIS(parsed, connRules) {
		if d.RuleID == "EIS_NO_CONNECTOR_INTERNAL" {
			t.Errorf("action-scoped internal must not apply to trigger: %s", d.Message)
		}
	}
}

func TestEIS_OUTPUT_MIRRORS_INPUT_Info(t *testing.T) {
	eis := json.RawMessage(`[{"name":"result","label":"Result","type":"string"},{"name":"status","label":"Status","type":"string"}]`)
	eos := json.RawMessage(`[{"name":"result","label":"Result","type":"string"}]`)
	input := rawJSON(t, map[string]interface{}{
		"result": "ok",
		"status": "done",
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:              "action",
				Provider:             strPtr("workato_recipe_function"),
				Name:                 "return_result",
				Input:                input,
				ExtendedInputSchema:  eis,
				ExtendedOutputSchema: eos,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	if !hasDiag(diags, "EIS_OUTPUT_MIRRORS_INPUT") {
		t.Error("expected EIS_OUTPUT_MIRRORS_INPUT when EIS field missing from EOS")
	}
}

func TestEIS_PyEvalPlatformFields_NoFalsePositive(t *testing.T) {
	eis := json.RawMessage(`[{"name":"code_input","label":"Input fields","type":"object","properties":[{"name":"schema","type":"string"},{"name":"data","type":"object"}]}]`)
	input := rawJSON(t, map[string]interface{}{
		"code":                    "def main(input):\n    return {'result': input['text']}\n",
		"code_input":              map[string]interface{}{"data": map[string]interface{}{"text": "hello"}},
		"code_output_schema_json": `[{"name":"result","type":"string"}]`,
		"name":                    "Reverse text",
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("py_eval"),
				Name:                "invoke_custom_py_code",
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	for _, d := range diags {
		if d.RuleID == "EIS_MIRRORS_INPUT" {
			t.Errorf("unexpected EIS_MIRRORS_INPUT for platform field: %s", d.Message)
		}
		if d.RuleID == "EIS_NAME_MATCH" && (d.Message == `EIS field "code" not found in input` || d.Message == `EIS field "code_output_schema_json" not found in input` || d.Message == `EIS field "name" not found in input`) {
			t.Errorf("unexpected EIS_NAME_MATCH for platform field: %s", d.Message)
		}
	}
}

func TestEIS_LoggerMessage_NoFalsePositive(t *testing.T) {
	eis := json.RawMessage(`[{"name":"user_logs_enabled","label":"Send to Workato log service","type":"string"}]`)
	input := rawJSON(t, map[string]interface{}{
		"message":           "Received event",
		"user_logs_enabled": "false",
	})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("logger"),
				Name:                "log_message",
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	for _, d := range diags {
		if d.RuleID == "EIS_MIRRORS_INPUT" {
			t.Errorf("unexpected EIS_MIRRORS_INPUT for logger message field: %s", d.Message)
		}
	}
}

func TestEIS_PubSubTopicID_ConnectorInternal(t *testing.T) {
	eis := json.RawMessage(`[{"name":"message","label":"Message","type":"object","properties":[{"name":"event_type","type":"string"},{"name":"payload","type":"string"}]}]`)
	input := rawJSON(t, map[string]interface{}{
		"message":  map[string]interface{}{"event_type": "test", "payload": "data"},
		"topic_id": map[string]interface{}{"folder": "", "name": "golden-test-topic"},
	})
	connRules := map[string]*ConnectorRules{
		"workato_pub_sub": {
			ConnectorInternals: []string{"topic_id"},
		},
	}
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("workato_pub_sub"),
				Name:                "publish_to_topic",
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, connRules)
	for _, d := range diags {
		if d.RuleID == "EIS_MIRRORS_INPUT" {
			t.Errorf("unexpected EIS_MIRRORS_INPUT for connector-internal topic_id: %s", d.Message)
		}
	}
}

func TestEIS_NoEIS_NoFalsePositives(t *testing.T) {
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:  "action",
				Provider: strPtr("salesforce"),
				Input:    rawJSON(t, map[string]interface{}{"field": "value"}),
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	if len(diags) != 0 {
		for _, d := range diags {
			t.Errorf("unexpected diagnostic: %s — %s", d.RuleID, d.Message)
		}
	}
}

func TestEIS_AllDiagsAreTier1(t *testing.T) {
	eis := json.RawMessage(`[{"name":"extra","label":"Extra","type":"string"}]`)
	input := rawJSON(t, map[string]interface{}{"other": "value"})
	parsed := buildParsedRecipe("test", []recipe.FlatStep{
		{
			Code: recipe.Code{
				Keyword:             "action",
				Provider:            strPtr("salesforce"),
				Input:               input,
				ExtendedInputSchema: eis,
			},
			JSONPointer: "/code/block/0",
		},
	}, nil)
	diags := checkEIS(parsed, nil)
	for _, d := range diags {
		if d.Tier != 1 {
			t.Errorf("expected tier 1 for rule %s, got tier %d", d.RuleID, d.Tier)
		}
	}
}
