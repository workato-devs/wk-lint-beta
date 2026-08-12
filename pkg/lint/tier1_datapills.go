package lint

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/workato-devs/recipe-lint/pkg/recipe"
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

	// Parse the trigger's declared result schema once — it types the fields a
	// return step writes under input.result when the step declares no EIS.
	triggerResult := parseTriggerResultSchema(parsed)

	for i := range parsed.Steps {
		step := &parsed.Steps[i]
		if step.Code.Input != nil {
			basePath := step.JSONPointer + "/input"
			recipe.WalkStringsWithContext(step.Code.Input, basePath, func(ctx recipe.StringContext) {
				diags = append(diags, lintDatapillStringWithCatch(ctx, step, connRules, catchAliases, triggerResult)...)
			})
		}
		// Trigger-level "only continue if..." condition (code.filter). Same
		// {type, operand, conditions} shape as an if-block's input, but it lives
		// outside `input` so it was previously invisible to all DP_* checks.
		if step.Code.Filter != nil {
			basePath := step.JSONPointer + "/filter"
			recipe.WalkStringsWithContext(step.Code.Filter, basePath, func(ctx recipe.StringContext) {
				diags = append(diags, lintDatapillStringWithCatch(ctx, step, connRules, catchAliases, triggerResult)...)
			})
		}
	}

	return diags
}

// lintDatapillStringWithCatch is lintDatapillString but with pre-built catch aliases.
func lintDatapillStringWithCatch(ctx recipe.StringContext, step *recipe.FlatStep, connRules map[string]*ConnectorRules, catchAliases map[string]bool, triggerResult []EISField) []LintDiagnostic {
	var diags []LintDiagnostic
	value := ctx.Value

	datapills := extractDatapills(value)
	if len(datapills) == 0 {
		if ctx.IsCondLHS && strings.HasPrefix(value, "=") {
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: "Condition input should be a datapill, not a formula expression",
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

	isFormula := strings.HasPrefix(value, "=")

	// DP_BARE_UNWRAPPED — a datapill that is neither interpolated nor in formula
	// mode is not evaluated at all: the platform emits the literal characters
	// _dp('{...}') as the field value. One diagnostic per string value.
	if !isFormula {
		for _, dp := range datapills {
			if isInsideInterpolation(value, dp.Start) {
				continue
			}
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: "Datapill is neither interpolated nor in formula mode — it is emitted as literal text",
				Source:  &SourceRef{JSONPointer: ctx.Pointer},
				RuleID:  "DP_BARE_UNWRAPPED",
				Tier:    1,
			})
			break
		}
	}

	// DP_INTERPOLATION_SINGLE — only for targets that hold a string. Interpolation
	// mode always yields a string, so on a field declared array/object/number/
	// integer/boolean the "remove the leading =" advice would stringify a
	// correctly-typed value.
	if isFormula && len(datapills) == 1 {
		dp := datapills[0]
		trimmed := strings.TrimSpace(value[1:])
		fullDP := value[dp.Start:dp.End]
		if trimmed == fullDP && !nonStringFieldTypes[declaredFieldType(step, ctx.Pointer, triggerResult)] {
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: "Single datapill in formula mode — use interpolation mode instead (remove leading =)",
				Source:  &SourceRef{JSONPointer: ctx.Pointer},
				RuleID:  "DP_INTERPOLATION_SINGLE",
				Tier:    1,
			})
		}
	}

	// DP_INTERPOLATION_TYPED — the inverse of DP_INTERPOLATION_SINGLE. Interpolation
	// mode always yields a string, so a lone pill wrapped in "#{...}" on a field
	// declared array/object/number/integer/boolean hands the platform a stringified
	// value. Such a recipe pushes, lints clean and runs green; only the caller sees
	// the wrong type. Restricted to a value that is exactly the wrapped pill: any
	// surrounding literal text makes the field a string by construction, and formula
	// mode is not the fix for it.
	if !isFormula && len(datapills) == 1 {
		dp := datapills[0]
		fullDP := value[dp.Start:dp.End]
		if value == "#{"+fullDP+"}" {
			if fieldType := declaredFieldType(step, ctx.Pointer, triggerResult); nonStringFieldTypes[fieldType] {
				diags = append(diags, LintDiagnostic{
					Level: LevelWarn,
					Message: fmt.Sprintf(
						"Datapill interpolated into a field declared %s — interpolation always yields a string; use formula mode (=) to preserve the type",
						fieldType,
					),
					Source: &SourceRef{JSONPointer: ctx.Pointer},
					RuleID: "DP_INTERPOLATION_TYPED",
					Tier:   1,
				})
			}
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

		// DP_NO_BODY_NATIVE — check the datapill's source provider, not the consuming step
		if containsBody(dp.Payload.Path) && !isAPIPlatformSourceProvider(dp.Payload) {
			srcProvider := "<nil>"
			if dp.Payload.Provider != nil {
				if s, ok := dp.Payload.Provider.(string); ok {
					srcProvider = s
				}
			}
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: fmt.Sprintf("Datapill path contains \"body\" but source provider %q is not an API platform connector", srcProvider),
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

// isInsideInterpolation reports whether the datapill starting at index start sits
// inside a "#{...}" wrapper. It looks back for the nearest "#{" and rejects it if a
// "}" closes it before the datapill begins — so in
// `#{_dp('{...}')} and _dp('{...}')` only the second occurrence is bare. The braces
// of the pill's own JSON payload sit after start and cannot affect the result.
func isInsideInterpolation(value string, start int) bool {
	open := strings.LastIndex(value[:start], "#{")
	if open < 0 {
		return false
	}
	return !strings.Contains(value[open+2:start], "}")
}

// nonStringFieldTypes are declared field types whose value is not a string.
// Interpolation mode ("#{...}") always resolves to a string; formula mode ("=")
// preserves the datapill's type. A rule that recommends interpolation must not
// fire on these targets.
var nonStringFieldTypes = map[string]bool{
	"array":   true,
	"object":  true,
	"number":  true,
	"integer": true,
	"boolean": true,
}

// declaredFieldType returns the type a recipe declares for the input field at
// pointer, or "" when nothing declares it. Sources, in order: the step's
// extended_input_schema, then — for a return step whose input.result mirrors the
// recipe's declared output — the trigger's result_schema_json.
func declaredFieldType(step *recipe.FlatStep, pointer string, triggerResult []EISField) string {
	prefix := step.JSONPointer + "/input/"
	if !strings.HasPrefix(pointer, prefix) {
		return ""
	}
	segments := strings.Split(strings.TrimPrefix(pointer, prefix), "/")

	eisFields, err := parseEIS(step.Code.ExtendedInputSchema)
	if err == nil {
		if t := lookupSchemaType(eisFields, segments); t != "" {
			return t
		}
	}

	if len(segments) > 1 && segments[0] == "result" {
		return lookupSchemaType(triggerResult, segments[1:])
	}
	return ""
}

// lookupSchemaType walks a schema field list along segments and returns the
// declared type of the leaf, or "" if any segment is undeclared or the schema
// stops before the leaf (an open container). Numeric segments are array indices;
// element fields live at the same schema level, so they are skipped — the same
// descent DP_PATH_RESOLVES performs over an output schema.
func lookupSchemaType(fields []EISField, segments []string) string {
	current := fields
	for i, seg := range segments {
		if _, err := strconv.Atoi(seg); err == nil {
			continue
		}
		field := findEISField(current, seg)
		if field == nil {
			return ""
		}
		if i == len(segments)-1 {
			return field.Type
		}
		if len(field.Properties) == 0 {
			return ""
		}
		current = field.Properties
	}
	return ""
}

// parseTriggerResultSchema parses the trigger's result_schema_json — the schema a
// recipe-function/skill trigger declares for the recipe's own output. It is a
// JSON-encoded string holding the same field-list shape as an EIS. Returns nil
// when absent or unparseable.
func parseTriggerResultSchema(parsed *recipe.ParsedRecipe) []EISField {
	if len(parsed.Steps) == 0 || parsed.Steps[0].Code.Input == nil {
		return nil
	}
	var input struct {
		ResultSchemaJSON string `json:"result_schema_json"`
	}
	if err := json.Unmarshal(parsed.Steps[0].Code.Input, &input); err != nil || input.ResultSchemaJSON == "" {
		return nil
	}
	fields, err := parseEIS(json.RawMessage(input.ResultSchemaJSON))
	if err != nil {
		return nil
	}
	return fields
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

// isAPIPlatformSourceProvider checks if the datapill's source provider is an
// API platform connector that uses body-prefixed output paths.
func isAPIPlatformSourceProvider(payload *DatapillPayload) bool {
	if payload.Provider == nil {
		return false
	}
	p, ok := payload.Provider.(string)
	if !ok {
		return false
	}
	return p == "workato_api_platform" || p == "rest" || p == "http"
}

