package lint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProfileDef defines a named lint profile loaded from a JSON file.
type ProfileDef struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Extends     string            `json:"extends,omitempty"`
	Rules       map[string]string `json:"rules"`
}

// ResolvedProfile is the result of resolving a profile's inheritance chain.
type ResolvedProfile struct {
	Name  string
	Chain []string
	Rules map[string]string
}

const maxProfileDepth = 5

// discoverProfiles loads profiles from two tiers: plugin-bundled profiles
// (pluginDir) are loaded first, then project-level profiles (projectRoot/.wklint/profiles)
// can override them by name.
func discoverProfiles(projectRoot, pluginDir string) (map[string]*ProfileDef, error) {
	// Layer 0: embedded built-in profiles (lowest precedence)
	profiles, err := loadEmbeddedProfiles()
	if err != nil {
		return nil, fmt.Errorf("loading embedded profiles: %w", err)
	}

	// Layer 1: plugin-bundled profiles
	if pluginDir != "" {
		bundledDir := filepath.Join(pluginDir, "profiles")
		if err := loadProfilesFromDir(bundledDir, profiles); err != nil {
			return nil, fmt.Errorf("loading bundled profiles: %w", err)
		}
	}

	// Layer 2: project-level profiles (overwrite bundled)
	if projectRoot != "" {
		projectDir := filepath.Join(projectRoot, ".wklint", "profiles")
		if err := loadProfilesFromDir(projectDir, profiles); err != nil {
			return nil, fmt.Errorf("loading project profiles: %w", err)
		}
	}

	return profiles, nil
}

// loadProfilesFromDir reads all *.json files from dir and adds them to the
// profiles map. The profile name must match the filename (without .json).
// If dir does not exist, this is a no-op.
func loadProfilesFromDir(dir string, into map[string]*ProfileDef) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("reading profile %s: %w", entry.Name(), err)
		}

		var prof ProfileDef
		if err := json.Unmarshal(data, &prof); err != nil {
			return fmt.Errorf("parsing profile %s: %w", entry.Name(), err)
		}

		expectedName := strings.TrimSuffix(entry.Name(), ".json")
		if prof.Name != expectedName {
			return fmt.Errorf("profile %s: name field %q must match filename %q", entry.Name(), prof.Name, expectedName)
		}

		into[prof.Name] = &prof
	}

	return nil
}

// resolveProfileChain walks the inheritance chain for the named profile,
// merging rules from ancestor to descendant (child overrides parent).
// Detects cycles and enforces a maximum depth of 5.
func resolveProfileChain(name string, discovered map[string]*ProfileDef) (*ResolvedProfile, error) {
	// Walk the chain to collect profiles from child to root
	var chain []string
	visited := make(map[string]bool)
	current := name

	for current != "" {
		if visited[current] {
			return nil, fmt.Errorf("profile cycle detected: %s appears twice in chain %v", current, chain)
		}
		if len(chain) >= maxProfileDepth {
			return nil, fmt.Errorf("profile inheritance too deep (max %d): chain %v", maxProfileDepth, chain)
		}

		prof, ok := discovered[current]
		if !ok {
			if len(chain) == 0 {
				return nil, fmt.Errorf("profile %q not found", current)
			}
			return nil, fmt.Errorf("profile %q not found (parent of %q)", current, chain[len(chain)-1])
		}

		visited[current] = true
		chain = append(chain, current)
		current = prof.Extends
	}

	// Merge rules from root to child (so child overrides parent)
	merged := make(map[string]string)
	for i := len(chain) - 1; i >= 0; i-- {
		prof := discovered[chain[i]]
		for k, v := range prof.Rules {
			merged[k] = v
		}
	}

	return &ResolvedProfile{
		Name:  name,
		Chain: chain,
		Rules: merged,
	}, nil
}
