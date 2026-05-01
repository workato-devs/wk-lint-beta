package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/workato-devs/wk-lint-beta/pkg/lint"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// RPCRequest represents a JSON-RPC 2.0 request.
type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCResponse represents a JSON-RPC 2.0 response.
type RPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC 2.0 error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- lint.run types ---

type lintRunParams struct {
	Files      []string `json:"files"`
	SkillsPath string   `json:"skills_path"`
	ConfigPath string   `json:"config_path"`
	Tiers      []int    `json:"tiers"`
	Profile    string   `json:"profile"`
	PluginDir  string   `json:"plugin_dir"`
}

type fileDiagnostics struct {
	File        string             `json:"file"`
	Diagnostics []lint.LintDiagnostic `json:"diagnostics"`
	Summary     fileSummary        `json:"summary"`
}

type fileSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
}

type lintRunResult struct {
	ExitCode int               `json:"exit_code"`
	Files    []fileDiagnostics `json:"files"`
}

// --- lint.pre_push types ---

type hookFile struct {
	Path       string `json:"path"`
	Status     string `json:"status"`
	ServerPath string `json:"server_path,omitempty"`
}

type prePushParams struct {
	ProjectRoot string     `json:"project_root"`
	Files       []hookFile `json:"files"`
	Profile     string     `json:"profile"`
	PluginDir   string     `json:"plugin_dir"`
}

type prePushDiagnostic struct {
	File         string `json:"file"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	Rule         string `json:"rule"`
	Path         string `json:"path"`
	SuggestedFix string `json:"suggested_fix,omitempty"`
}

type prePushResult struct {
	Passed      bool                `json:"passed"`
	Diagnostics []prePushDiagnostic `json:"diagnostics"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer for large JSON-RPC requests
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var req RPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp := RPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error: &RPCError{
					Code:    -32700,
					Message: "Parse error: " + err.Error(),
				},
			}
			writeResponse(resp)
			continue
		}

		resp := handleRequest(req)
		writeResponse(resp)

		// Handle shutdown: exit after sending response
		if req.Method == "shutdown" {
			os.Exit(0)
		}
	}
}

// handleRequest processes a single JSON-RPC request and returns a response.
// Extracted for testability.
func handleRequest(req RPCRequest) RPCResponse {
	switch req.Method {
	case "lint.run":
		return handleLintRun(req)
	case "lint.pre_push":
		return handlePrePush(req)
	case "lint.describe_rules":
		return handleDescribeRules(req)
	case "lint.version":
		return RPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"version": version,
				"commit":  commit,
				"date":    date,
			},
		}
	case "shutdown":
		return RPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{"ok": true},
		}
	default:
		return RPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    -32601,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

func handleLintRun(req RPCRequest) RPCResponse {
	var params lintRunParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return RPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &RPCError{
					Code:    -32602,
					Message: "Invalid params: " + err.Error(),
				},
			}
		}
	}

	expandedFiles, dirErrors := expandFiles(params.Files)

	result := lintRunResult{
		Files: make([]fileDiagnostics, 0, len(expandedFiles)),
	}
	result.Files = append(result.Files, dirErrors...)

	for _, file := range expandedFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			result.Files = append(result.Files, fileDiagnostics{
				File: file,
				Diagnostics: []lint.LintDiagnostic{{
					Level:   lint.LevelError,
					Message: "Cannot read file: " + err.Error(),
					RuleID:  "FILE_READ_ERROR",
					Tier:    0,
				}},
				Summary: fileSummary{Errors: 1},
			})
			continue
		}

		opts := lint.LintOptions{
			Tiers:      params.Tiers,
			SkillsPath: params.SkillsPath,
			ConfigPath: params.ConfigPath,
			Filename:   file,
			Profile:    params.Profile,
			PluginDir:  params.PluginDir,
		}

		diags, err := lint.LintRecipe(data, opts)
		if err != nil {
			result.Files = append(result.Files, fileDiagnostics{
				File: file,
				Diagnostics: []lint.LintDiagnostic{{
					Level:   lint.LevelError,
					Message: "Lint error: " + err.Error(),
					RuleID:  "LINT_ERROR",
					Tier:    0,
				}},
				Summary: fileSummary{Errors: 1},
			})
			continue
		}

		if diags == nil {
			diags = []lint.LintDiagnostic{}
		}

		summary := fileSummary{}
		for _, d := range diags {
			switch d.Level {
			case lint.LevelError:
				summary.Errors++
			case lint.LevelWarn:
				summary.Warnings++
			case lint.LevelInfo:
				summary.Info++
			}
		}

		result.Files = append(result.Files, fileDiagnostics{
			File:        file,
			Diagnostics: diags,
			Summary:     summary,
		})
	}

	result.ExitCode = computeExitCode(result.Files)

	return RPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func computeExitCode(files []fileDiagnostics) int {
	hasLintError := false
	for _, f := range files {
		for _, d := range f.Diagnostics {
			switch d.RuleID {
			case "FILE_READ_ERROR", "INVALID_JSON", "LINT_ERROR":
				return 2
			}
			if d.Level == lint.LevelError {
				hasLintError = true
			}
		}
	}
	if hasLintError {
		return 1
	}
	return 0
}

func handlePrePush(req RPCRequest) RPCResponse {
	var params prePushParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return RPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &RPCError{
					Code:    -32602,
					Message: "Invalid params: " + err.Error(),
				},
			}
		}
	}

	// Filter to .recipe.json files only
	var recipeFiles []string
	for _, f := range params.Files {
		if strings.HasSuffix(f.Path, ".recipe.json") {
			recipeFiles = append(recipeFiles, f.Path)
		}
	}

	// Resolve files relative to project_root if needed
	resolvedFiles := make([]string, 0, len(recipeFiles))
	for _, f := range recipeFiles {
		if filepath.IsAbs(f) {
			resolvedFiles = append(resolvedFiles, f)
		} else if params.ProjectRoot != "" {
			resolvedFiles = append(resolvedFiles, filepath.Join(params.ProjectRoot, f))
		} else {
			resolvedFiles = append(resolvedFiles, f)
		}
	}

	result := prePushResult{
		Passed:      true,
		Diagnostics: make([]prePushDiagnostic, 0),
	}

	for _, file := range resolvedFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			result.Passed = false
			result.Diagnostics = append(result.Diagnostics, prePushDiagnostic{
				File:     file,
				Severity: lint.LevelError,
				Message:  "Cannot read file: " + err.Error(),
				Rule:     "FILE_READ_ERROR",
				Path:     "/",
			})
			continue
		}

		opts := lint.LintOptions{
			Filename:  file,
			Profile:   params.Profile,
			PluginDir: params.PluginDir,
		}

		diags, err := lint.LintRecipe(data, opts)
		if err != nil {
			result.Passed = false
			result.Diagnostics = append(result.Diagnostics, prePushDiagnostic{
				File:     file,
				Severity: lint.LevelError,
				Message:  "Lint error: " + err.Error(),
				Rule:     "LINT_ERROR",
				Path:     "/",
			})
			continue
		}

		for _, d := range diags {
			path := "/"
			if d.Source != nil {
				path = d.Source.JSONPointer
			}
			result.Diagnostics = append(result.Diagnostics, prePushDiagnostic{
				File:         file,
				Severity:     d.Level,
				Message:      d.Message,
				Rule:         d.RuleID,
				Path:         path,
				SuggestedFix: d.SuggestedFix,
			})
			if d.Level == lint.LevelError {
				result.Passed = false
			}
		}
	}

	return RPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

// --- lint.describe_rules types ---

type describeRulesParams struct {
	SkillsPath string `json:"skills_path"`
	ConfigPath string `json:"config_path"`
}

func handleDescribeRules(req RPCRequest) RPCResponse {
	var params describeRulesParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return RPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &RPCError{
					Code:    -32602,
					Message: "Invalid params: " + err.Error(),
				},
			}
		}
	}

	catalog, err := lint.DescribeRules(lint.DescribeOptions{
		SkillsPath: params.SkillsPath,
		ConfigPath: params.ConfigPath,
	})
	if err != nil {
		return RPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    -32603,
				Message: "Failed to load rules: " + err.Error(),
			},
		}
	}

	return RPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  catalog,
	}
}

func expandFiles(paths []string) ([]string, []fileDiagnostics) {
	var result []string
	var dirErrors []fileDiagnostics

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			result = append(result, p)
			continue
		}

		if !info.IsDir() {
			result = append(result, p)
			continue
		}

		walkErr := filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".recipe.json") {
				result = append(result, path)
			}
			return nil
		})
		if walkErr != nil {
			dirErrors = append(dirErrors, fileDiagnostics{
				File: p,
				Diagnostics: []lint.LintDiagnostic{{
					Level:   lint.LevelError,
					Message: "Cannot read directory: " + walkErr.Error(),
					RuleID:  "FILE_READ_ERROR",
					Tier:    0,
				}},
				Summary: fileSummary{Errors: 1},
			})
		}
	}
	return result, dirErrors
}

func writeResponse(resp RPCResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		// Fallback error response
		fmt.Fprintf(os.Stdout, `{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"Internal error: %s"}}`+"\n", err.Error())
		return
	}
	fmt.Fprintln(os.Stdout, string(data))
}
