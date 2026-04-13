//go:build snapshot

package igm

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Environment variables for snapshot comparison configuration:
//
//	IGM_TS_PROJECT_DIR  — path to the TypeScript IGM project root (contains core/snapshot-export.ts)
//	RECIPE_CORPUS_DIR   — path to the recipe corpus for cross-repo fixture selection
//
// Example:
//
//	IGM_TS_PROJECT_DIR=../my-visualizer/src RECIPE_CORPUS_DIR=../my-recipes/workato/recipes \
//	  go test -tags snapshot ./pkg/igm/ -run Snapshot

func tsProjectDir() string { return os.Getenv("IGM_TS_PROJECT_DIR") }

// snapshotFixtures returns recipe paths to test. Includes local fixtures
// (always available) and corpus fixtures (when RECIPE_CORPUS_DIR is set).
func snapshotFixtures(t *testing.T) []string {
	t.Helper()

	fixtures := []string{
		"../lint/testdata/fixtures/simple_connector.recipe.json",
		"../lint/testdata/fixtures/api_endpoint_try_catch.recipe.json",
	}

	dir := os.Getenv("RECIPE_CORPUS_DIR")
	if dir != "" {
		// Add a diverse selection from the corpus if available
		corpusFixtures := []string{
			"atomic-salesforce-recipes/upsert_contact.recipe.json",
			"orchestrator-recipes/check_in_guest.recipe.json",
			"orchestrator-recipes/create_booking_orchestrator.recipe.json",
			"orchestrator-recipes/manage_cases_orchestrator.recipe.json",
		}
		for _, f := range corpusFixtures {
			fixtures = append(fixtures, filepath.Join(dir, f))
		}
	}

	return fixtures
}

func TestSnapshot_GoVsTypeScript(t *testing.T) {
	tsDir := tsProjectDir()
	if tsDir == "" {
		t.Skip("IGM_TS_PROJECT_DIR not set — skipping snapshot comparison")
	}

	exportScript := filepath.Join(tsDir, "core", "snapshot-export.ts")
	if _, err := os.Stat(exportScript); os.IsNotExist(err) {
		t.Skipf("snapshot-export.ts not found at %s", exportScript)
	}
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available")
	}

	for _, fixture := range snapshotFixtures(t) {
		absPath := fixture
		if !filepath.IsAbs(fixture) {
			var err error
			absPath, err = filepath.Abs(fixture)
			if err != nil {
				t.Fatalf("abs path: %v", err)
			}
		}

		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			t.Logf("skipping %s (not found)", filepath.Base(absPath))
			continue
		}

		name := filepath.Base(absPath)
		t.Run(name, func(t *testing.T) {
			// --- Go snapshot ---
			data, err := os.ReadFile(absPath)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			goGraph, err := Transform(data)
			if err != nil {
				t.Fatalf("Go transform: %v", err)
			}
			goSnap := goGraph.ToSnapshot()

			// --- TS snapshot ---
			cmd := exec.Command("npx", "tsx", "core/snapshot-export.ts", absPath)
			cmd.Dir = tsDir
			tsOut, err := cmd.Output()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					t.Fatalf("TS transform failed: %s\n%s", err, string(exitErr.Stderr))
				}
				t.Fatalf("TS transform: %v", err)
			}

			var tsSnap Snapshot
			if err := json.Unmarshal(tsOut, &tsSnap); err != nil {
				t.Fatalf("parse TS output: %v", err)
			}

			// --- Compare ---
			compareAliasMap(t, goSnap.AliasMap, tsSnap.AliasMap)
			compareRoots(t, goSnap.Roots, tsSnap.Roots)
			compareNodes(t, goSnap.Nodes, tsSnap.Nodes)
			compareEdges(t, goSnap.Edges, tsSnap.Edges)
		})
	}
}

func compareAliasMap(t *testing.T, goMap, tsMap map[string]string) {
	t.Helper()

	for alias, goID := range goMap {
		tsID, ok := tsMap[alias]
		if !ok {
			t.Errorf("alias %q: Go has %q, TS missing", alias, goID)
			continue
		}
		if goID != tsID {
			t.Errorf("alias %q: Go=%q TS=%q", alias, goID, tsID)
		}
	}
	for alias, tsID := range tsMap {
		if _, ok := goMap[alias]; !ok {
			t.Errorf("alias %q: TS has %q, Go missing", alias, tsID)
		}
	}
}

func compareRoots(t *testing.T, goRoots, tsRoots []string) {
	t.Helper()
	if len(goRoots) != len(tsRoots) {
		t.Errorf("roots count: Go=%d TS=%d", len(goRoots), len(tsRoots))
		return
	}
	for i := range goRoots {
		if goRoots[i] != tsRoots[i] {
			t.Errorf("root[%d]: Go=%q TS=%q", i, goRoots[i], tsRoots[i])
		}
	}
}

func compareNodes(t *testing.T, goNodes, tsNodes []SnapshotNode) {
	t.Helper()

	goByID := make(map[string]SnapshotNode)
	for _, n := range goNodes {
		goByID[n.ID] = n
	}
	tsByID := make(map[string]SnapshotNode)
	for _, n := range tsNodes {
		tsByID[n.ID] = n
	}

	for id, goN := range goByID {
		tsN, ok := tsByID[id]
		if !ok {
			if strings.Contains(id, "::") {
				t.Logf("node %q: Go-only virtual node (OK)", id)
			} else {
				t.Errorf("node %q: Go has it, TS missing", id)
			}
			continue
		}
		compareNode(t, id, goN, tsN)
	}

	for id, tsN := range tsByID {
		if _, ok := goByID[id]; !ok {
			if strings.Contains(id, "::") {
				t.Logf("node %q: TS-only virtual node (OK)", id)
			} else {
				t.Errorf("node %q (%s): TS has it, Go missing", id, tsN.Kind)
			}
		}
	}
}

func compareNode(t *testing.T, id string, goN, tsN SnapshotNode) {
	t.Helper()

	if goN.Kind != tsN.Kind {
		t.Errorf("node %q kind: Go=%q TS=%q", id, goN.Kind, tsN.Kind)
	}
	if goN.IsTerminal != tsN.IsTerminal {
		t.Errorf("node %q isTerminal: Go=%v TS=%v", id, goN.IsTerminal, tsN.IsTerminal)
	}
	if goN.Pointer != tsN.Pointer {
		t.Errorf("node %q pointer: Go=%q TS=%q", id, goN.Pointer, tsN.Pointer)
	}
	if goN.StepAs != tsN.StepAs {
		t.Errorf("node %q stepAs: Go=%q TS=%q", id, goN.StepAs, tsN.StepAs)
	}

	goProvider := "<nil>"
	if goN.Provider != nil {
		goProvider = *goN.Provider
	}
	tsProvider := "<nil>"
	if tsN.Provider != nil {
		tsProvider = *tsN.Provider
	}
	if goProvider != tsProvider {
		t.Errorf("node %q provider: Go=%s TS=%s", id, goProvider, tsProvider)
	}

	if goN.HTTPStatus != tsN.HTTPStatus {
		t.Errorf("node %q httpStatus: Go=%q TS=%q", id, goN.HTTPStatus, tsN.HTTPStatus)
	}
}

func compareEdges(t *testing.T, goEdges, tsEdges []SnapshotEdge) {
	t.Helper()

	goSet := make(map[string]SnapshotEdge)
	for _, e := range goEdges {
		key := fmt.Sprintf("%s|%s|%s", e.Kind, e.From, e.To)
		goSet[key] = e
	}
	tsSet := make(map[string]SnapshotEdge)
	for _, e := range tsEdges {
		key := fmt.Sprintf("%s|%s|%s", e.Kind, e.From, e.To)
		tsSet[key] = e
	}

	for key := range goSet {
		if _, ok := tsSet[key]; !ok {
			if strings.Contains(key, "::then") || strings.Contains(key, "::else") || strings.Contains(key, "::catch") {
				t.Logf("edge %q: Go-only (virtual node edge, OK)", key)
			} else {
				t.Errorf("edge %q: Go has it, TS missing", key)
			}
		}
	}
	for key := range tsSet {
		if _, ok := goSet[key]; !ok {
			if strings.Contains(key, "::then") || strings.Contains(key, "::else") || strings.Contains(key, "::catch") {
				t.Logf("edge %q: TS-only (virtual node edge, OK)", key)
			} else {
				t.Errorf("edge %q: TS has it, Go missing", key)
			}
		}
	}
}
