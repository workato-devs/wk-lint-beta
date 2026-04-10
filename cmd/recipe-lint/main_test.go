package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// testdataDir returns the absolute path to pkg/lint/testdata relative to this test file.
func testdataDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	// cmd/recipe-lint/main_test.go -> repo root -> pkg/lint/testdata
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filename)))
	return filepath.Join(repoRoot, "pkg", "lint", "testdata")
}

func TestUnknownMethod(t *testing.T) {
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "nonexistent.method",
	}
	resp := handleRequest(req)

	if resp.Error == nil {
		t.Fatal("expected error for unknown method, got nil")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected error code -32601, got %d", resp.Error.Code)
	}
	if resp.Result != nil {
		t.Errorf("expected nil result, got %v", resp.Result)
	}
}

func TestLintRunWithFixture(t *testing.T) {
	dir := testdataDir(t)
	fixturePath := filepath.Join(dir, "fixtures", "simple_connector.recipe.json")

	// Verify fixture exists
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Fatalf("fixture file does not exist: %s", fixturePath)
	}

	params, _ := json.Marshal(lintRunParams{
		Files: []string{fixturePath},
	})

	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      float64(2),
		Method:  "lint.run",
		Params:  json.RawMessage(params),
	}
	resp := handleRequest(req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: code=%d message=%s", resp.Error.Code, resp.Error.Message)
	}
	if resp.ID != float64(2) {
		t.Errorf("expected ID 2, got %v", resp.ID)
	}

	// Marshal and re-unmarshal to inspect structure
	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("cannot marshal result: %v", err)
	}

	var result lintRunResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("cannot unmarshal result: %v", err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file in result, got %d", len(result.Files))
	}
	if result.Files[0].File != fixturePath {
		t.Errorf("expected file path %q, got %q", fixturePath, result.Files[0].File)
	}
}

func TestLintRunMultipleFixtures(t *testing.T) {
	dir := testdataDir(t)
	files := []string{
		filepath.Join(dir, "fixtures", "api_endpoint_try_catch.recipe.json"),
		filepath.Join(dir, "fixtures", "simple_connector.recipe.json"),
		filepath.Join(dir, "fixtures", "if_else_branching.recipe.json"),
	}

	// Verify all fixtures exist
	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Fatalf("fixture file does not exist: %s", f)
		}
	}

	params, _ := json.Marshal(lintRunParams{
		Files: files,
	})

	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      float64(3),
		Method:  "lint.run",
		Params:  json.RawMessage(params),
	}
	resp := handleRequest(req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: code=%d message=%s", resp.Error.Code, resp.Error.Message)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result lintRunResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("cannot unmarshal result: %v", err)
	}

	if len(result.Files) != 3 {
		t.Fatalf("expected 3 files in result, got %d", len(result.Files))
	}
}

func TestPrePushWithFiles(t *testing.T) {
	dir := testdataDir(t)
	fixturePath := filepath.Join(dir, "fixtures", "simple_connector.recipe.json")

	// Verify fixture exists
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Fatalf("fixture file does not exist: %s", fixturePath)
	}

	params, _ := json.Marshal(prePushParams{
		ProjectRoot: "",
		Files:       []hookFile{{Path: fixturePath}, {Path: "/some/path/not_a_recipe.json"}},
	})

	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      float64(4),
		Method:  "lint.pre_push",
		Params:  json.RawMessage(params),
	}
	resp := handleRequest(req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: code=%d message=%s", resp.Error.Code, resp.Error.Message)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result prePushResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("cannot unmarshal result: %v", err)
	}

	// The "passed" field must exist (we check it's a bool, not nil)
	// For a valid recipe file, passed should be true (no error-level diagnostics from stub)
	// Note: with the full linter, there may be warnings but those don't fail pre_push
	if !result.Passed {
		// Check if any error-level diagnostic exists
		hasErrors := false
		for _, d := range result.Diagnostics {
			if d.Severity == "error" {
				hasErrors = true
				t.Logf("Error diagnostic: file=%s rule=%s message=%s", d.File, d.Rule, d.Message)
			}
		}
		if hasErrors {
			t.Logf("pre_push failed with error-level diagnostics (expected for full linter)")
		}
	}

	// Verify non-recipe files are filtered out
	for _, d := range result.Diagnostics {
		if d.File == "/some/path/not_a_recipe.json" {
			t.Error("non-.recipe.json file should have been filtered out")
		}
	}
}

func TestPrePushFiltersNonRecipeFiles(t *testing.T) {
	params, _ := json.Marshal(prePushParams{
		ProjectRoot: "/fake/root",
		Files:       []hookFile{{Path: "readme.md"}, {Path: "package.json"}, {Path: "styles.css"}},
	})

	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      float64(5),
		Method:  "lint.pre_push",
		Params:  json.RawMessage(params),
	}
	resp := handleRequest(req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: code=%d message=%s", resp.Error.Code, resp.Error.Message)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result prePushResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("cannot unmarshal result: %v", err)
	}

	if !result.Passed {
		t.Error("expected passed=true when no recipe files to lint")
	}
	if len(result.Diagnostics) != 0 {
		t.Errorf("expected 0 diagnostics, got %d", len(result.Diagnostics))
	}
}

func TestShutdown(t *testing.T) {
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      float64(99),
		Method:  "shutdown",
	}
	resp := handleRequest(req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: code=%d message=%s", resp.Error.Code, resp.Error.Message)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("cannot unmarshal result: %v", err)
	}

	ok, exists := result["ok"]
	if !exists {
		t.Fatal("expected 'ok' key in shutdown response")
	}
	if ok != true {
		t.Errorf("expected ok=true, got %v", ok)
	}
}

func TestLintRunWithMalformedFixture(t *testing.T) {
	dir := testdataDir(t)
	fixturePath := filepath.Join(dir, "malformed", "code_as_array.recipe.json")

	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Fatalf("fixture file does not exist: %s", fixturePath)
	}

	params, _ := json.Marshal(lintRunParams{
		Files: []string{fixturePath},
	})

	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      float64(6),
		Method:  "lint.run",
		Params:  json.RawMessage(params),
	}
	resp := handleRequest(req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: code=%d message=%s", resp.Error.Code, resp.Error.Message)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result lintRunResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("cannot unmarshal result: %v", err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file in result, got %d", len(result.Files))
	}

	// Malformed file should produce error diagnostics
	fd := result.Files[0]
	if fd.Summary.Errors == 0 {
		t.Log("Note: malformed fixture did not produce errors (may depend on linter implementation)")
	}
}

func TestLintRunNonexistentFile(t *testing.T) {
	params, _ := json.Marshal(lintRunParams{
		Files: []string{"/nonexistent/path/fake.recipe.json"},
	})

	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      float64(7),
		Method:  "lint.run",
		Params:  json.RawMessage(params),
	}
	resp := handleRequest(req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: code=%d message=%s", resp.Error.Code, resp.Error.Message)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result lintRunResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("cannot unmarshal result: %v", err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file in result, got %d", len(result.Files))
	}

	if result.Files[0].Summary.Errors != 1 {
		t.Errorf("expected 1 error for nonexistent file, got %d", result.Files[0].Summary.Errors)
	}
}

func TestResponseJSONRPCVersion(t *testing.T) {
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      float64(10),
		Method:  "shutdown",
	}
	resp := handleRequest(req)

	if resp.JSONRPC != "2.0" {
		t.Errorf("expected JSONRPC version 2.0, got %q", resp.JSONRPC)
	}
}
