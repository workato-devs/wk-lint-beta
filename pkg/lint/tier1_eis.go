package lint

import (
	"encoding/json"
	"fmt"

	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

// EISField represents a field in the extended_input_schema or extended_output_schema.
type EISField struct {
	Name        string     `json:"name"`
	Label       string     `json:"label,omitempty"`
	Type        string     `json:"type,omitempty"`
	ParseOutput string     `json:"parse_output,omitempty"`
	Properties  []EISField `json:"properties,omitempty"`
}

// parseEIS unmarshals a raw JSON array into EIS fields.
func parseEIS(raw json.RawMessage) ([]EISField, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var fields []EISField
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

// flattenEISNames recursively extracts all field names from EIS, returning a set.
// Nested properties are joined with "." prefix.
func flattenEISNames(fields []EISField, prefix string) map[string]bool {
	names := make(map[string]bool)
	for _, f := range fields {
		fullName := f.Name
		if prefix != "" {
			fullName = prefix + "." + f.Name
		}
		names[fullName] = true
		if len(f.Properties) > 0 {
			for k, v := range flattenEISNames(f.Properties, fullName) {
				names[k] = v
			}
		}
	}
	return names
}

// flattenInputKeys extracts top-level keys from a raw JSON input object,
// excluding keys in the internals set.
func flattenInputKeys(raw json.RawMessage, internals map[string]bool) (map[string]bool, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var inputMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &inputMap); err != nil {
		return nil, err
	}
	keys := make(map[string]bool)
	for k := range inputMap {
		if internals != nil && internals[k] {
			continue
		}
		keys[k] = true
	}
	return keys, nil
}

// getConnectorInternals returns the set of connector-internal field names for a step.
func getConnectorInternals(step recipe.FlatStep, connRules map[string]*ConnectorRules) map[string]bool {
	internals := make(map[string]bool)
	if step.Code.Provider == nil || connRules == nil {
		return internals
	}
	cr, ok := connRules[*step.Code.Provider]
	if !ok {
		return internals
	}
	for _, name := range cr.ConnectorInternals {
		internals[name] = true
	}
	return internals
}

// checkEIS validates extended_input_schema consistency with input fields.
func checkEIS(parsed *recipe.ParsedRecipe, connRules map[string]*ConnectorRules) []LintDiagnostic {
	var diags []LintDiagnostic

	for _, step := range parsed.Steps {
		if step.Code.ExtendedInputSchema == nil {
			continue
		}

		eisFields, err := parseEIS(step.Code.ExtendedInputSchema)
		if err != nil {
			// Can't parse EIS — skip
			continue
		}
		if len(eisFields) == 0 {
			continue
		}

		internals := getConnectorInternals(step, connRules)
		eisNames := flattenEISNames(eisFields, "")
		eisTopLevel := make(map[string]bool)
		for _, f := range eisFields {
			eisTopLevel[f.Name] = true
		}

		// EIS_NO_CONNECTOR_INTERNAL: check EIS doesn't contain connector-internal fields
		for _, f := range eisFields {
			if internals[f.Name] {
				diags = append(diags, LintDiagnostic{
					Level:   LevelWarn,
					Message: fmt.Sprintf("EIS field %q is a connector-internal field and should not be in extended_input_schema", f.Name),
					Source:  &SourceRef{JSONPointer: step.JSONPointer + "/extended_input_schema"},
					RuleID:  "EIS_NO_CONNECTOR_INTERNAL",
					Tier:    1,
				})
			}
		}

		// Need input to check mirrors/match rules
		if step.Code.Input == nil {
			continue
		}

		inputKeys, err := flattenInputKeys(step.Code.Input, internals)
		if err != nil {
			continue
		}

		// EIS_MIRRORS_INPUT: every input key should have a matching EIS name
		for key := range inputKeys {
			if !eisTopLevel[key] {
				diags = append(diags, LintDiagnostic{
					Level:   LevelWarn,
					Message: fmt.Sprintf("Input field %q not found in extended_input_schema", key),
					Source:  &SourceRef{JSONPointer: step.JSONPointer + "/input/" + key},
					RuleID:  "EIS_MIRRORS_INPUT",
					Tier:    1,
				})
			}
		}

		// EIS_NAME_MATCH: check for exact name matches (catches typos/case mismatches)
		for _, f := range eisFields {
			if internals[f.Name] {
				continue
			}
			if !inputKeys[f.Name] {
				diags = append(diags, LintDiagnostic{
					Level:   LevelWarn,
					Message: fmt.Sprintf("EIS field %q not found in input", f.Name),
					Source:  &SourceRef{JSONPointer: step.JSONPointer + "/extended_input_schema"},
					RuleID:  "EIS_NAME_MATCH",
					Tier:    1,
				})
			}
		}

		// EIS_NESTED_MATCH: input has nested object but EIS lacks properties
		for _, f := range eisFields {
			if !inputKeys[f.Name] {
				continue
			}
			// Check if input value is an object
			var inputMap map[string]json.RawMessage
			if err := json.Unmarshal(step.Code.Input, &inputMap); err != nil {
				continue
			}
			inputVal, ok := inputMap[f.Name]
			if !ok {
				continue
			}
			// Try to unmarshal as object
			var nested map[string]json.RawMessage
			if json.Unmarshal(inputVal, &nested) == nil && len(nested) > 0 {
				if len(f.Properties) == 0 {
					diags = append(diags, LintDiagnostic{
						Level:   LevelWarn,
						Message: fmt.Sprintf("Input field %q is a nested object but EIS field lacks properties", f.Name),
						Source:  &SourceRef{JSONPointer: step.JSONPointer + "/extended_input_schema"},
						RuleID:  "EIS_NESTED_MATCH",
						Tier:    1,
					})
				}
			}
		}

		// EIS_OUTPUT_MIRRORS_INPUT: for return_result actions, check EOS matches EIS
		if step.Code.Name == "return_result" && step.Code.ExtendedOutputSchema != nil {
			eosFields, eosErr := parseEIS(step.Code.ExtendedOutputSchema)
			if eosErr == nil && len(eosFields) > 0 {
				eosNames := make(map[string]bool)
				for _, f := range eosFields {
					eosNames[f.Name] = true
				}
				_ = eisNames // use the full flattened set
				for _, f := range eisFields {
					if !eosNames[f.Name] {
						diags = append(diags, LintDiagnostic{
							Level:   LevelInfo,
							Message: fmt.Sprintf("EIS field %q not found in extended_output_schema for return_result action", f.Name),
							Source:  &SourceRef{JSONPointer: step.JSONPointer + "/extended_output_schema"},
							RuleID:  "EIS_OUTPUT_MIRRORS_INPUT",
							Tier:    1,
						})
					}
				}
			}
		}
	}

	return diags
}
