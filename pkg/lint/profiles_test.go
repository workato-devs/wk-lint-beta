package lint

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProfile(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveProfile_NoParent(t *testing.T) {
	profiles := map[string]*ProfileDef{
		"basic": {
			Name:  "basic",
			Rules: map[string]string{"RULE_A": "warning"},
		},
	}

	resolved, err := resolveProfileChain("basic", profiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Name != "basic" {
		t.Errorf("expected name basic, got %s", resolved.Name)
	}
	if len(resolved.Chain) != 1 || resolved.Chain[0] != "basic" {
		t.Errorf("expected chain [basic], got %v", resolved.Chain)
	}
	if resolved.Rules["RULE_A"] != "warning" {
		t.Errorf("expected RULE_A=warning, got %s", resolved.Rules["RULE_A"])
	}
}

func TestResolveProfile_SingleInheritance(t *testing.T) {
	profiles := map[string]*ProfileDef{
		"parent": {
			Name:  "parent",
			Rules: map[string]string{"RULE_A": "warning", "RULE_B": "info"},
		},
		"child": {
			Name:    "child",
			Extends: "parent",
			Rules:   map[string]string{"RULE_A": "error"},
		},
	}

	resolved, err := resolveProfileChain("child", profiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Chain) != 2 {
		t.Fatalf("expected chain length 2, got %d", len(resolved.Chain))
	}
	// Child overrides parent
	if resolved.Rules["RULE_A"] != "error" {
		t.Errorf("expected RULE_A=error (child override), got %s", resolved.Rules["RULE_A"])
	}
	// Parent rule inherited
	if resolved.Rules["RULE_B"] != "info" {
		t.Errorf("expected RULE_B=info (inherited), got %s", resolved.Rules["RULE_B"])
	}
}

func TestResolveProfile_ThreeLevelInheritance(t *testing.T) {
	profiles := map[string]*ProfileDef{
		"base": {
			Name:  "base",
			Rules: map[string]string{"A": "info", "B": "info", "C": "info"},
		},
		"mid": {
			Name:    "mid",
			Extends: "base",
			Rules:   map[string]string{"B": "warning"},
		},
		"top": {
			Name:    "top",
			Extends: "mid",
			Rules:   map[string]string{"C": "error"},
		},
	}

	resolved, err := resolveProfileChain("top", profiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Chain) != 3 {
		t.Fatalf("expected chain length 3, got %d", len(resolved.Chain))
	}
	if resolved.Rules["A"] != "info" {
		t.Errorf("expected A=info, got %s", resolved.Rules["A"])
	}
	if resolved.Rules["B"] != "warning" {
		t.Errorf("expected B=warning, got %s", resolved.Rules["B"])
	}
	if resolved.Rules["C"] != "error" {
		t.Errorf("expected C=error, got %s", resolved.Rules["C"])
	}
}

func TestResolveProfile_CycleDetection(t *testing.T) {
	profiles := map[string]*ProfileDef{
		"a": {Name: "a", Extends: "b", Rules: map[string]string{}},
		"b": {Name: "b", Extends: "a", Rules: map[string]string{}},
	}

	_, err := resolveProfileChain("a", profiles)
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func TestResolveProfile_DepthLimit(t *testing.T) {
	profiles := make(map[string]*ProfileDef)
	// Create a chain of 6 profiles (exceeds maxProfileDepth=5)
	for i := 0; i < 6; i++ {
		name := string(rune('a' + i))
		parent := ""
		if i > 0 {
			parent = string(rune('a' + i - 1))
		}
		profiles[name] = &ProfileDef{
			Name:    name,
			Extends: parent,
			Rules:   map[string]string{},
		}
	}

	// Resolve from the deepest
	_, err := resolveProfileChain("f", profiles)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
}

func TestResolveProfile_MissingParent(t *testing.T) {
	profiles := map[string]*ProfileDef{
		"child": {Name: "child", Extends: "nonexistent", Rules: map[string]string{}},
	}

	_, err := resolveProfileChain("child", profiles)
	if err == nil {
		t.Fatal("expected missing parent error")
	}
}

func TestResolveProfile_NotFound(t *testing.T) {
	profiles := map[string]*ProfileDef{}

	_, err := resolveProfileChain("missing", profiles)
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestResolveProfile_EmptyRules(t *testing.T) {
	profiles := map[string]*ProfileDef{
		"empty": {Name: "empty", Rules: map[string]string{}},
	}

	resolved, err := resolveProfileChain("empty", profiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Rules) != 0 {
		t.Errorf("expected empty rules, got %v", resolved.Rules)
	}
}

func TestDiscoverProfiles_ProjectOverridesPlugin(t *testing.T) {
	tmpDir := t.TempDir()

	pluginDir := filepath.Join(tmpDir, "plugin")
	projectRoot := filepath.Join(tmpDir, "project")

	pluginProfilesDir := filepath.Join(pluginDir, "profiles")
	projectProfilesDir := filepath.Join(projectRoot, ".wklint", "profiles")

	// Plugin has standard with RULE_A=warning
	writeProfile(t, pluginProfilesDir, "standard.json", `{
		"name": "standard",
		"rules": {"RULE_A": "warning"}
	}`)

	// Project overrides standard with RULE_A=error
	writeProfile(t, projectProfilesDir, "standard.json", `{
		"name": "standard",
		"rules": {"RULE_A": "error"}
	}`)

	profiles, err := discoverProfiles(projectRoot, pluginDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prof, ok := profiles["standard"]
	if !ok {
		t.Fatal("expected standard profile")
	}
	if prof.Rules["RULE_A"] != "error" {
		t.Errorf("expected project override RULE_A=error, got %s", prof.Rules["RULE_A"])
	}
}

func TestLoadProfilesFromDir_NameMustMatchFilename(t *testing.T) {
	tmpDir := t.TempDir()
	dir := filepath.Join(tmpDir, "profiles")

	writeProfile(t, dir, "standard.json", `{
		"name": "wrong-name",
		"rules": {}
	}`)

	profiles := make(map[string]*ProfileDef)
	err := loadProfilesFromDir(dir, profiles)
	if err == nil {
		t.Fatal("expected name mismatch error")
	}
}
