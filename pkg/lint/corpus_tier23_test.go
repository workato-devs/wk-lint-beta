package lint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTier23_RecipeCorpus(t *testing.T) {
	dir := os.Getenv("RECIPE_CORPUS_DIR")
	if dir == "" {
		t.Skip("RECIPE_CORPUS_DIR not set — skipping corpus test")
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("RECIPE_CORPUS_DIR %q not found", dir)
	}

	opts := LintOptions{
		SkillsPath: os.Getenv("LINT_SKILLS_PATH"),
		ConfigPath: os.Getenv("LINT_CONFIG_PATH"),
	}

	if opts.SkillsPath != "" {
		t.Logf("Skills path: %s", opts.SkillsPath)
	}
	if opts.ConfigPath != "" {
		t.Logf("Config path: %s", opts.ConfigPath)
	}

	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".json" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking corpus dir: %v", err)
	}

	t.Logf("Testing %d recipe files against tiers 0-3", len(files))

	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			fileOpts := opts
			fileOpts.Filename = name

			diags, err := LintRecipe(data, fileOpts)
			if err != nil {
				t.Skipf("lint error (may not be a recipe): %v", err)
				return
			}

			tierCounts := make(map[int]int)
			ruleCounts := make(map[string]int)
			for _, d := range diags {
				tierCounts[d.Tier]++
				ruleCounts[d.RuleID]++
			}

			t.Logf("  Diagnostics: tier0=%d tier1=%d tier2=%d tier3=%d total=%d",
				tierCounts[0], tierCounts[1], tierCounts[2], tierCounts[3], len(diags))

			for _, d := range diags {
				if d.Tier >= 2 {
					ptr := ""
					if d.Source != nil {
						ptr = d.Source.JSONPointer
					}
					t.Logf("  [T%d] %s: %s (%s)", d.Tier, d.RuleID, d.Message, ptr)
				}
			}
		})
	}
}
