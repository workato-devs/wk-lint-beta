package lint

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestDatapillRegexCorpusValidation validates the _dp() extraction regex against
// the dewy-resort recipe corpus to ensure no datapill occurrences are missed.
func TestDatapillRegexCorpusValidation(t *testing.T) {
	corpusDir := filepath.Join(os.Getenv("HOME"), "Github", "dewy-resort", "workato", "recipes")
	if _, err := os.Stat(corpusDir); os.IsNotExist(err) {
		t.Skip("corpus directory not found at ~/Github/dewy-resort/workato/recipes — skipping")
	}

	// Broad pattern: any _dp(...) including nested parens
	broadPattern := regexp.MustCompile(`_dp\([^)]*\)`)

	var totalBroad int
	var totalPrecise int
	var deltas []string

	err := filepath.Walk(corpusDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".recipe.json") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(data)

		broadMatches := broadPattern.FindAllString(content, -1)
		preciseMatches := dpRegex.FindAllString(content, -1)

		totalBroad += len(broadMatches)
		totalPrecise += len(preciseMatches)

		// Check for deltas: broad matches not covered by precise
		preciseSet := make(map[string]bool)
		for _, m := range preciseMatches {
			preciseSet[m] = true
		}
		for _, m := range broadMatches {
			if !preciseSet[m] {
				relPath, _ := filepath.Rel(corpusDir, path)
				deltas = append(deltas, relPath+": "+m)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk error: %v", err)
	}

	t.Logf("Corpus validation: %d broad matches, %d precise matches, %d deltas", totalBroad, totalPrecise, len(deltas))

	if len(deltas) > 0 {
		t.Errorf("Found %d datapill occurrences matched by broad pattern but not precise regex:", len(deltas))
		for _, d := range deltas {
			t.Logf("  delta: %s", d)
		}
	}
}
