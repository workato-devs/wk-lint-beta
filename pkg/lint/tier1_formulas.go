package lint

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

//go:embed formulas.json
var formulasJSON []byte

// formulaAllowlist is the union of all valid method names from formulas.json.
var formulaAllowlist map[string]bool

// forbiddenPattern describes a pattern that should never appear in a formula.
type forbiddenPattern struct {
	Pattern string `json:"pattern"`
	Message string `json:"message"`
}

// forbiddenPatterns loaded from formulas.json.
var forbiddenPatterns []forbiddenPattern

func init() {
	var data struct {
		Methods           map[string][]string `json:"methods"`
		ForbiddenPatterns []forbiddenPattern  `json:"forbidden_patterns"`
	}
	if err := json.Unmarshal(formulasJSON, &data); err != nil {
		panic("failed to parse formulas.json: " + err.Error())
	}

	formulaAllowlist = make(map[string]bool)
	for _, methods := range data.Methods {
		for _, m := range methods {
			formulaAllowlist[m] = true
		}
	}
	forbiddenPatterns = data.ForbiddenPatterns
}

// extractMethods extracts method names from a Workato formula string.
// It handles _dp('...') datapill payloads, string literals, and parenthesized arguments.
func extractMethods(formula string) []string {
	if len(formula) == 0 {
		return nil
	}
	// Strip leading '='
	if formula[0] == '=' {
		formula = formula[1:]
	}
	if len(formula) == 0 {
		return nil
	}

	var methods []string
	i := 0
	n := len(formula)

	for i < n {
		ch := formula[i]

		// _dp(' → skip to ')
		if ch == '_' && i+4 < n && formula[i:i+5] == "_dp('" {
			i += 5
			for i < n {
				if formula[i] == '\'' && i+1 < n && formula[i+1] == ')' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		// String literal: skip to closing quote
		if ch == '\'' {
			i++
			for i < n && formula[i] != '\'' {
				i++
			}
			if i < n {
				i++ // skip closing quote
			}
			continue
		}

		// Parenthesized expression: skip to matching close with depth tracking
		if ch == '(' {
			depth := 1
			i++
			for i < n && depth > 0 {
				switch formula[i] {
				case '(':
					depth++
				case ')':
					depth--
				case '\'':
					// Skip string literals inside parens
					i++
					for i < n && formula[i] != '\'' {
						i++
					}
				}
				i++
			}
			continue
		}

		// Dot → read method identifier
		if ch == '.' {
			i++
			start := i
			for i < n {
				c := formula[i]
				if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
					i++
				} else if c == '?' && (i+1 >= n || formula[i+1] != '?') {
					// Include trailing '?' as part of method name (e.g., present?)
					// but not '??' which would be something else
					i++
					break
				} else {
					break
				}
			}
			if i > start {
				methods = append(methods, formula[start:i])
			}
			continue
		}

		i++
	}

	return methods
}

// lintFormulaString checks a single formula string for invalid methods.
func lintFormulaString(value string, pointer string) []LintDiagnostic {
	var diags []LintDiagnostic

	// Pass 1: forbidden patterns
	coveredMethods := make(map[string]bool)
	for _, fp := range forbiddenPatterns {
		if strings.Contains(value, fp.Pattern) {
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: fp.Message,
				Source:  &SourceRef{JSONPointer: pointer},
				RuleID:  "FORMULA_FORBIDDEN_PATTERN",
				Tier:    1,
			})
			// Extract method names from the pattern to suppress duplicate INVALID findings
			patternMethods := extractMethods(fp.Pattern)
			for _, m := range patternMethods {
				coveredMethods[m] = true
			}
		}
	}

	// Pass 2: allowlist check
	methods := extractMethods(value)
	for _, m := range methods {
		if formulaAllowlist[m] {
			continue
		}
		if coveredMethods[m] {
			continue
		}
		diags = append(diags, LintDiagnostic{
			Level:   LevelWarn,
			Message: fmt.Sprintf("Unknown formula method %q; not in Workato formula allowlist", m),
			Source:  &SourceRef{JSONPointer: pointer},
			RuleID:  "FORMULA_METHOD_INVALID",
			Tier:    1,
		})
	}

	return diags
}

// checkFormulaMethods walks all step inputs and validates formula strings.
func checkFormulaMethods(parsed *recipe.ParsedRecipe) []LintDiagnostic {
	var diags []LintDiagnostic

	for _, step := range parsed.Steps {
		if step.Code.Input == nil {
			continue
		}
		basePath := step.JSONPointer + "/input"
		recipe.WalkStrings(step.Code.Input, basePath, func(pointer string, value string) {
			if !strings.HasPrefix(value, "=") {
				return
			}
			diags = append(diags, lintFormulaString(value, pointer)...)
		})
	}

	return diags
}
