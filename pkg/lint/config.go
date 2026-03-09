package lint

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LintConfig holds user-level lint configuration loaded from .wklintrc.json.
type LintConfig struct {
	Version     string            `json:"version"`
	Profile     string            `json:"profile,omitempty"`
	Rules       map[string]string `json:"rules"`
	IgnoreFiles []string          `json:"ignore_files"`
}

// LoadConfig reads a .wklintrc.json config file. If path is empty or the file
// does not exist, it returns nil, nil (no error).
func LoadConfig(path string) (*LintConfig, error) {
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cfg LintConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// EffectiveSeverity returns the configured severity for a rule, or the default
// if the rule is not explicitly configured. A value of "off" means suppress.
func (c *LintConfig) EffectiveSeverity(ruleID, defaultLevel string) string {
	if c == nil || c.Rules == nil {
		return defaultLevel
	}
	if level, ok := c.Rules[ruleID]; ok {
		return level
	}
	return defaultLevel
}

// ShouldIgnoreFile checks whether the given filename matches any of the
// configured ignore glob patterns.
func (c *LintConfig) ShouldIgnoreFile(filename string) bool {
	if c == nil {
		return false
	}
	base := filepath.Base(filename)
	for _, pattern := range c.IgnoreFiles {
		if matched, err := filepath.Match(pattern, base); err == nil && matched {
			return true
		}
		// Also try matching against the full path
		if matched, err := filepath.Match(pattern, filename); err == nil && matched {
			return true
		}
	}
	return false
}
