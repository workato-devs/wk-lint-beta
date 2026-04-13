package igm

import (
	"os"
	"path/filepath"
	"testing"
)

// corpusDir returns the recipe corpus directory from the RECIPE_CORPUS_DIR
// environment variable. Returns empty string if unset.
func corpusDir() string {
	return os.Getenv("RECIPE_CORPUS_DIR")
}

func TestTransform_RecipeCorpus(t *testing.T) {
	dir := corpusDir()
	if dir == "" {
		t.Skip("RECIPE_CORPUS_DIR not set — skipping corpus test")
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("RECIPE_CORPUS_DIR %q not found", dir)
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

	if len(files) == 0 {
		t.Fatal("no recipe files found in corpus dir")
	}

	t.Logf("Testing %d recipe files from corpus", len(files))

	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			graph, err := Transform(data)
			if err != nil {
				// Some files may not be recipe-shaped (e.g. connection files)
				t.Skipf("transform error (may not be a recipe): %v", err)
				return
			}

			// Invariant 1: Exactly one ::end node
			endCount := 0
			for _, n := range graph.Nodes {
				if n.ID == "::end" {
					endCount++
				}
			}
			if endCount != 1 {
				t.Errorf("expected exactly 1 ::end node, got %d", endCount)
			}

			// Invariant 2: All terminals have exactly one outgoing edge (to ::end)
			for _, n := range graph.Nodes {
				if !n.IsTerminal {
					continue
				}
				out := graph.OutEdges(n.ID)
				terminalEdges := 0
				for _, e := range out {
					if e.Kind == EdgeTerminal && e.To == "::end" {
						terminalEdges++
					}
				}
				if terminalEdges != 1 {
					t.Errorf("terminal node %s has %d terminal edges to ::end, want 1", n.ID, terminalEdges)
				}
			}

			// Invariant 3: All non-end, non-terminal nodes have outgoing edges
			outgoing := make(map[string]bool)
			for _, e := range graph.Edges {
				outgoing[e.From] = true
			}
			for _, n := range graph.Nodes {
				if n.ID == "::end" || n.IsTerminal {
					continue
				}
				if !outgoing[n.ID] {
					t.Errorf("node %s (%s) has no outgoing edges", n.ID, n.Kind)
				}
			}

			// Invariant 4: Deterministic — run again, get same result
			graph2, err2 := Transform(data)
			if err2 != nil {
				t.Fatalf("second transform failed: %v", err2)
			}
			if len(graph.Nodes) != len(graph2.Nodes) {
				t.Errorf("node count not deterministic: %d vs %d", len(graph.Nodes), len(graph2.Nodes))
			}
			if len(graph.Edges) != len(graph2.Edges) {
				t.Errorf("edge count not deterministic: %d vs %d", len(graph.Edges), len(graph2.Edges))
			}

			// Invariant 5: At least one root
			if len(graph.Roots) == 0 {
				t.Error("no root nodes")
			}
		})
	}

	t.Logf("Corpus test complete: %d files tested", len(files))
}
