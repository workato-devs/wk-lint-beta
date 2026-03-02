package recipe

import (
	"encoding/json"
	"fmt"
)

// Parse takes raw recipe JSON bytes and returns a fully parsed recipe.
// It validates basic structure (code is object, not array) and recursively
// walks code.block to produce a flat list of all steps with JSON pointers.
func Parse(data []byte) (*ParsedRecipe, error) {
	var raw Recipe
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Parse trigger (top-level code)
	var trigger Code
	if err := json.Unmarshal(raw.Code, &trigger); err != nil {
		return nil, fmt.Errorf("cannot parse code block: %w", err)
	}

	// Build flat step list
	steps := []FlatStep{{
		Code:        trigger,
		JSONPointer: "/code",
		Depth:       0,
	}}
	if err := walkBlock(trigger.Block, "/code/block", 1, &steps); err != nil {
		return nil, err
	}

	// Parse config
	var config []ConfigEntry
	if raw.Config != nil {
		if err := json.Unmarshal(raw.Config, &config); err != nil {
			return nil, fmt.Errorf("cannot parse config: %w", err)
		}
	}

	// Extract unique providers
	providerSet := make(map[string]bool)
	for _, c := range config {
		if c.Provider != "" {
			providerSet[c.Provider] = true
		}
	}
	var providers []string
	for p := range providerSet {
		providers = append(providers, p)
	}

	return &ParsedRecipe{
		Raw:       raw,
		Trigger:   trigger,
		Steps:     steps,
		Config:    config,
		Providers: providers,
	}, nil
}

// walkBlock recursively walks a block array, appending each step to the flat list.
func walkBlock(block []json.RawMessage, basePath string, depth int, steps *[]FlatStep) error {
	for i, raw := range block {
		var step Code
		if err := json.Unmarshal(raw, &step); err != nil {
			return fmt.Errorf("cannot parse step at %s/%d: %w", basePath, i, err)
		}
		pointer := fmt.Sprintf("%s/%d", basePath, i)
		*steps = append(*steps, FlatStep{
			Code:        step,
			JSONPointer: pointer,
			Depth:       depth,
		})
		if len(step.Block) > 0 {
			if err := walkBlock(step.Block, pointer+"/block", depth+1, steps); err != nil {
				return err
			}
		}
	}
	return nil
}
