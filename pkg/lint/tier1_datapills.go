package lint

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

// datapillMatch represents a single _dp() occurrence found in a string.
type datapillMatch struct {
	RawJSON  string           // raw captured JSON string inside _dp('...')
	Payload  *DatapillPayload // nil if parse failed
	ParseErr error
	Start    int // position in source string
	End      int
}

// DatapillPayload is the parsed JSON inside a _dp('...') call.
type DatapillPayload struct {
	PillType string        `json:"pill_type"`
	Provider interface{}   `json:"provider"` // can be string or null
	Line     string        `json:"line"`
	Path     []interface{} `json:"path"`
}

// dpRegex extracts _dp('...') payloads. The inner content is a single-quoted
// JSON string that may contain escaped characters.
var dpRegex = regexp.MustCompile(`_dp\('((?:[^'\\]|\\.)*)'\)`)

// ternaryPattern detects ternary expressions (value ? a : b).
var ternaryPattern = regexp.MustCompile(`\?`)

// extractDatapills finds all _dp('...') occurrences in a string and parses their JSON payloads.
func extractDatapills(value string) []datapillMatch {
	matches := dpRegex.FindAllStringSubmatchIndex(value, -1)
	if len(matches) == 0 {
		return nil
	}

	var results []datapillMatch
	for _, m := range matches {
		// m[0],m[1] = full match; m[2],m[3] = capture group 1
		rawJSON := value[m[2]:m[3]]
		// Unescape the single-quote escaped JSON
		unescaped := strings.ReplaceAll(rawJSON, `\'`, `'`)
		unescaped = strings.ReplaceAll(unescaped, `\\`, `\`)

		dm := datapillMatch{
			RawJSON: rawJSON,
			Start:   m[0],
			End:     m[1],
		}

		var payload DatapillPayload
		if err := json.Unmarshal([]byte(unescaped), &payload); err != nil {
			dm.ParseErr = err
		} else {
			dm.Payload = &payload
		}
		results = append(results, dm)
	}
	return results
}

// checkDatapillsWithCatchAliases walks all step inputs and validates datapill patterns.
func checkDatapillsWithCatchAliases(parsed *recipe.ParsedRecipe, connRules map[string]*ConnectorRules) []LintDiagnostic {
	var diags []LintDiagnostic

	// Pre-build catch alias set from all steps
	catchAliases := make(map[string]bool)
	for _, step := range parsed.Steps {
		if step.Code.Keyword == "catch" && step.Code.As != "" {
			catchAliases[step.Code.As] = true
		}
	}

	for i := range parsed.Steps {
		step := &parsed.Steps[i]
		if step.Code.Input == nil {
			continue
		}
		basePath := step.JSONPointer + "/input"
		recipe.WalkStringsWithContext(step.Code.Input, basePath, func(ctx recipe.StringContext) {
			diags = append(diags, lintDatapillStringWithCatch(ctx, step, connRules, catchAliases)...)
		})
	}

	return diags
}

// lintDatapillStringWithCatch is lintDatapillString but with pre-built catch aliases.
func lintDatapillStringWithCatch(ctx recipe.StringContext, step *recipe.FlatStep, connRules map[string]*ConnectorRules, catchAliases map[string]bool) []LintDiagnostic {
	var diags []LintDiagnostic
	value := ctx.Value

	datapills := extractDatapills(value)
	if len(datapills) == 0 {
		if ctx.IsCondLHS && strings.HasPrefix(value, "=") {
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: "Condition LHS should not be a formula expression",
				Source:  &SourceRef{JSONPointer: ctx.Pointer},
				RuleID:  "DP_LHS_NO_FORMULA",
				Tier:    1,
			})
		}
		return diags
	}

	// DP_VALID_JSON
	for _, dp := range datapills {
		if dp.ParseErr != nil {
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: fmt.Sprintf("Datapill payload is not valid JSON: %v", dp.ParseErr),
				Source:  &SourceRef{JSONPointer: ctx.Pointer},
				RuleID:  "DP_VALID_JSON",
				Tier:    1,
			})
		}
	}

	// DP_LHS_NO_FORMULA
	if ctx.IsCondLHS && strings.HasPrefix(value, "=") {
		diags = append(diags, LintDiagnostic{
			Level:   LevelError,
			Message: "Condition LHS should not be a formula expression",
			Source:  &SourceRef{JSONPointer: ctx.Pointer},
			RuleID:  "DP_LHS_NO_FORMULA",
			Tier:    1,
		})
	}

	isFormula := strings.HasPrefix(value, "=")

	// DP_INTERPOLATION_SINGLE
	if isFormula && len(datapills) == 1 {
		dp := datapills[0]
		trimmed := strings.TrimSpace(value[1:])
		fullDP := value[dp.Start:dp.End]
		if trimmed == fullDP {
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: "Single datapill in formula mode — use interpolation mode instead (remove leading =)",
				Source:  &SourceRef{JSONPointer: ctx.Pointer},
				RuleID:  "DP_INTERPOLATION_SINGLE",
				Tier:    1,
			})
		}
	}

	// DP_FORMULA_CONCAT
	if !isFormula && len(datapills) >= 2 && strings.Contains(value, "#{") {
		diags = append(diags, LintDiagnostic{
			Level:   LevelWarn,
			Message: "Multiple datapills using interpolation — consider formula mode with concatenation instead",
			Source:  &SourceRef{JSONPointer: ctx.Pointer},
			RuleID:  "DP_FORMULA_CONCAT",
			Tier:    1,
		})
	}

	// DP_NO_OUTER_PARENS
	if isFormula {
		body := strings.TrimSpace(value[1:])
		if len(body) > 2 && body[0] == '(' && body[len(body)-1] == ')' {
			if !ternaryPattern.MatchString(body) {
				diags = append(diags, LintDiagnostic{
					Level:   LevelInfo,
					Message: "Formula wrapped in unnecessary outer parentheses",
					Source:  &SourceRef{JSONPointer: ctx.Pointer},
					RuleID:  "DP_NO_OUTER_PARENS",
					Tier:    1,
				})
			}
		}
	}

	for _, dp := range datapills {
		if dp.Payload == nil {
			continue
		}

		// DP_NO_BODY_NATIVE
		if containsBody(dp.Payload.Path) && !isAPIPlatformConnector(step) {
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: fmt.Sprintf("Datapill path contains \"body\" but step provider %q is not an API platform connector", providerName(step)),
				Source:  &SourceRef{JSONPointer: ctx.Pointer},
				RuleID:  "DP_NO_BODY_NATIVE",
				Tier:    1,
			})
		}

		// DP_CATCH_PROVIDER
		if dp.Payload.Provider == nil && dp.Payload.Line != "" && len(catchAliases) > 0 {
			if catchAliases[dp.Payload.Line] {
				diags = append(diags, LintDiagnostic{
					Level:   LevelWarn,
					Message: fmt.Sprintf("Datapill references catch alias %q with null provider — verify error variable binding", dp.Payload.Line),
					Source:  &SourceRef{JSONPointer: ctx.Pointer},
					RuleID:  "DP_CATCH_PROVIDER",
					Tier:    1,
				})
			}
		}
	}

	return diags
}

// containsBody checks if a datapill path contains "body".
func containsBody(path []interface{}) bool {
	for _, p := range path {
		if s, ok := p.(string); ok && s == "body" {
			return true
		}
	}
	return false
}

// isAPIPlatformConnector checks if the step's provider is an API platform connector.
func isAPIPlatformConnector(step *recipe.FlatStep) bool {
	if step.Code.Provider == nil {
		return false
	}
	p := *step.Code.Provider
	return p == "workato_api_platform" || p == "rest" || p == "http"
}

// providerName returns the provider name for a step, or "<none>" if nil.
func providerName(step *recipe.FlatStep) string {
	if step.Code.Provider == nil {
		return "<none>"
	}
	return *step.Code.Provider
}
