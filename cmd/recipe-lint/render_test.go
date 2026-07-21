package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/workato-devs/recipe-lint/pkg/lint"
)

func callLintRender(t *testing.T, result lintRunResult) RPCResponse {
	t.Helper()

	params, err := json.Marshal(map[string]interface{}{
		"result": result,
		"context": map[string]interface{}{
			"format":       "text",
			"command_path": "wk lint",
		},
	})
	if err != nil {
		t.Fatalf("marshal renderer params: %v", err)
	}

	return handleRequest(RPCRequest{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "lint.render",
		Params:  params,
	})
}

func renderTextFromResponse(t *testing.T, resp RPCResponse) string {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: code=%d message=%s", resp.Error.Code, resp.Error.Message)
	}

	data, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal renderer result: %v", err)
	}

	var result lintRenderResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal renderer result: %v", err)
	}
	return result.Text
}

func TestLintRenderCleanResult(t *testing.T) {
	resp := callLintRender(t, lintRunResult{
		ExitCode: 0,
		Files: []fileDiagnostics{{
			File:        "clean.recipe.json",
			Diagnostics: []lint.LintDiagnostic{},
			Summary:     fileSummary{},
		}},
	})

	want := "clean.recipe.json\n" +
		"  No issues found.\n" +
		"  Summary: 0 errors, 0 warnings, 0 info\n\n" +
		"Lint summary: 1 file, 0 errors, 0 warnings, 0 info"
	if got := renderTextFromResponse(t, resp); got != want {
		t.Errorf("rendered text =\n%s\n\nwant:\n%s", got, want)
	}
}

func TestLintRenderDiagnosticsAndLocations(t *testing.T) {
	resp := callLintRender(t, lintRunResult{
		ExitCode: 1,
		Files: []fileDiagnostics{{
			File: "findings.recipe.json",
			Diagnostics: []lint.LintDiagnostic{
				{
					Level:        lint.LevelError,
					Message:      "Required response is missing",
					Source:       &lint.SourceRef{JSONPointer: "/code/2"},
					RuleID:       "RESPONSE_REQUIRED",
					Tier:         2,
					SuggestedFix: "Add a return response\nbefore the recipe ends.",
				},
				{
					Level:   lint.LevelWarn,
					Message: "Formula uses interpolation",
					RuleID:  "DP_FORMULA_CONCAT",
					Tier:    1,
				},
				{
					Level:   lint.LevelInfo,
					Message: "Consider a descriptive UUID",
					Source:  &lint.SourceRef{JSONPointer: "/code/3/uuid"},
					RuleID:  "UUID_DESCRIPTIVE",
					Tier:    1,
				},
			},
			Summary: fileSummary{Errors: 1, Warnings: 1, Info: 1},
		}},
	})

	want := "findings.recipe.json\n" +
		"  /code/2 [ERROR] RESPONSE_REQUIRED: Required response is missing\n" +
		"    Suggested fix: Add a return response\n" +
		"    before the recipe ends.\n" +
		"  [WARN] DP_FORMULA_CONCAT: Formula uses interpolation\n" +
		"  /code/3/uuid [INFO] UUID_DESCRIPTIVE: Consider a descriptive UUID\n" +
		"  Summary: 1 error, 1 warning, 1 info\n\n" +
		"Lint summary: 1 file, 1 error, 1 warning, 1 info"
	if got := renderTextFromResponse(t, resp); got != want {
		t.Errorf("rendered text =\n%s\n\nwant:\n%s", got, want)
	}
}

func TestLintRenderMultipleFiles(t *testing.T) {
	resp := callLintRender(t, lintRunResult{
		ExitCode: 0,
		Files: []fileDiagnostics{
			{
				File:        "clean.recipe.json",
				Diagnostics: []lint.LintDiagnostic{},
				Summary:     fileSummary{},
			},
			{
				File: "warning.recipe.json",
				Diagnostics: []lint.LintDiagnostic{{
					Level:   lint.LevelWarn,
					Message: "A warning",
					RuleID:  "WARN_RULE",
					Tier:    1,
				}},
				Summary: fileSummary{Warnings: 1},
			},
		},
	})

	text := renderTextFromResponse(t, resp)
	for _, want := range []string{
		"clean.recipe.json\n  No issues found.",
		"warning.recipe.json\n  [WARN] WARN_RULE: A warning",
		"Lint summary: 2 files, 0 errors, 1 warning, 0 info",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered text missing %q:\n%s", want, text)
		}
	}
}

func TestLintRenderEmptyFileList(t *testing.T) {
	resp := callLintRender(t, lintRunResult{ExitCode: 0, Files: []fileDiagnostics{}})
	want := "No recipe files were linted.\n\n" +
		"Lint summary: 0 files, 0 errors, 0 warnings, 0 info"
	if got := renderTextFromResponse(t, resp); got != want {
		t.Errorf("rendered text = %q, want %q", got, want)
	}
}

func TestLintRenderConsumesExistingResultWithoutRerunningLint(t *testing.T) {
	resp := callLintRender(t, lintRunResult{
		ExitCode: 0,
		Files: []fileDiagnostics{{
			File:        "/definitely/not/a/real.recipe.json",
			Diagnostics: []lint.LintDiagnostic{},
			Summary:     fileSummary{},
		}},
	})

	text := renderTextFromResponse(t, resp)
	if !strings.Contains(text, "No issues found.") || strings.Contains(text, "FILE_READ_ERROR") {
		t.Errorf("renderer appears not to have consumed the supplied result:\n%s", text)
	}

	data, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "exit_code") {
		t.Errorf("renderer response must contain presentation text only: %s", data)
	}
}

func TestLintRenderIgnoresUnknownContextFields(t *testing.T) {
	params := json.RawMessage(`{
		"result":{"exit_code":0,"files":[]},
		"context":{"format":"text","command_path":"wk lint","future_option":true}
	}`)
	resp := handleRequest(RPCRequest{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "lint.render",
		Params:  params,
	})
	if resp.Error != nil {
		t.Fatalf("unknown context field should be ignored: %v", resp.Error)
	}
}

func TestLintRenderRejectsMalformedParams(t *testing.T) {
	tests := []struct {
		name   string
		params json.RawMessage
	}{
		{name: "missing params"},
		{name: "invalid JSON", params: json.RawMessage(`{`)},
		{name: "missing context", params: json.RawMessage(`{"result":{"exit_code":0,"files":[]}}`)},
		{name: "wrong format", params: json.RawMessage(`{"result":{"exit_code":0,"files":[]},"context":{"format":"json"}}`)},
		{name: "missing result", params: json.RawMessage(`{"context":{"format":"text"}}`)},
		{name: "null result", params: json.RawMessage(`{"result":null,"context":{"format":"text"}}`)},
		{name: "missing exit code", params: json.RawMessage(`{"result":{"files":[]},"context":{"format":"text"}}`)},
		{name: "missing files", params: json.RawMessage(`{"result":{"exit_code":0},"context":{"format":"text"}}`)},
		{name: "wrong files type", params: json.RawMessage(`{"result":{"exit_code":0,"files":"bad"},"context":{"format":"text"}}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := handleRequest(RPCRequest{
				JSONRPC: "2.0",
				ID:      float64(1),
				Method:  "lint.render",
				Params:  tt.params,
			})
			if resp.Error == nil {
				t.Fatal("expected invalid params error, got nil")
			}
			if resp.Error.Code != -32602 {
				t.Errorf("error code = %d, want -32602", resp.Error.Code)
			}
			if resp.Result != nil {
				t.Errorf("result = %#v, want nil", resp.Result)
			}
		})
	}
}
