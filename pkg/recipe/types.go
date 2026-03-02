package recipe

import "encoding/json"

// Recipe is the top-level Workato recipe structure.
type Recipe struct {
	Name        string          `json:"name"`
	Version     *int            `json:"version"`
	Private     *bool           `json:"private"`
	Concurrency json.RawMessage `json:"concurrency"`
	Code        json.RawMessage `json:"code"`
	Config      json.RawMessage `json:"config"`
	Description string          `json:"description,omitempty"`
}

// Code represents the top-level code block (trigger) and nested steps.
type Code struct {
	Keyword                  string            `json:"keyword"`
	Provider                 *string           `json:"provider"`
	Name                     string            `json:"name,omitempty"`
	As                       string            `json:"as,omitempty"`
	Number                   *int              `json:"number"`
	UUID                     string            `json:"uuid,omitempty"`
	Input                    json.RawMessage   `json:"input,omitempty"`
	Block                    []json.RawMessage `json:"block,omitempty"`
	Conditions               json.RawMessage   `json:"conditions,omitempty"`
	ExtendedInputSchema      json.RawMessage   `json:"extended_input_schema,omitempty"`
	ExtendedOutputSchema     json.RawMessage   `json:"extended_output_schema,omitempty"`
	Responses                json.RawMessage   `json:"responses,omitempty"`
	DynamicPickListSelection json.RawMessage   `json:"dynamicPickListSelection,omitempty"`
	ToggleCfg                json.RawMessage   `json:"toggleCfg,omitempty"`
	VisibleConfigFields      json.RawMessage   `json:"visible_config_fields,omitempty"`
	FormatVersion            *int              `json:"format_version,omitempty"`
}

// ConfigEntry represents one entry in the config array.
type ConfigEntry struct {
	Keyword        string          `json:"keyword"`
	Provider       string          `json:"provider,omitempty"`
	Name           string          `json:"name,omitempty"`
	SkipValidation *bool           `json:"skip_validation,omitempty"`
	AccountID      json.RawMessage `json:"account_id,omitempty"`
}

// FlatStep is a step extracted from the recipe tree with its JSON pointer path.
type FlatStep struct {
	Code        Code
	JSONPointer string
	Depth       int
}

// ParsedRecipe holds the result of fully parsing a recipe.
type ParsedRecipe struct {
	Raw       Recipe
	Trigger   Code
	Steps     []FlatStep
	Config    []ConfigEntry
	Providers []string // unique providers found in config
}
