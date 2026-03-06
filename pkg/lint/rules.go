package lint

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ConnectorRules holds lint rules specific to a connector.
type ConnectorRules struct {
	Version            string       `json:"version"`
	Connector          string       `json:"connector"`
	ConnectorInternals []string     `json:"connector_internals"`
	ValidActionNames   []string     `json:"valid_action_names,omitempty"`
	ActionRules        []ActionRule `json:"action_rules"`
}

// ActionRule defines a single connector-specific lint rule.
type ActionRule struct {
	RuleID          string                `json:"rule_id"`
	ActionNames     []string              `json:"action_names"`
	RequireFields   []string              `json:"require_fields,omitempty"`
	RequireIn       []string              `json:"require_in,omitempty"`
	EISMustBeEmpty  bool                  `json:"eis_must_be_empty,omitempty"`
	FieldTypeChecks map[string]FieldCheck `json:"field_type_checks,omitempty"`
	Message         string                `json:"message"`
}

// FieldCheck defines a type check for a specific field.
type FieldCheck struct {
	Type     string `json:"type"`
	Expected string `json:"expected"`
}

// LoadConnectorRules scans skillsPath for lint-rules.json files and returns
// a map keyed by connector name. Returns an empty map if the path is empty
// or does not exist.
func LoadConnectorRules(skillsPath string) (map[string]*ConnectorRules, error) {
	result := make(map[string]*ConnectorRules)

	if skillsPath == "" {
		return result, nil
	}

	info, err := os.Stat(skillsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return result, nil
	}

	err = filepath.Walk(skillsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			return nil
		}
		if info.Name() != "lint-rules.json" {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil // skip unreadable files
		}

		var rules ConnectorRules
		if jsonErr := json.Unmarshal(data, &rules); jsonErr != nil {
			return nil // skip malformed files
		}

		if rules.Connector != "" {
			result[rules.Connector] = &rules
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}
