//go:build snapshot

package igm

import (
	"encoding/json"
	"sort"
)

// Snapshot is the normalized graph output used for cross-implementation comparison.
type Snapshot struct {
	Nodes    []SnapshotNode    `json:"nodes"`
	Edges    []SnapshotEdge    `json:"edges"`
	Roots    []string          `json:"roots"`
	AliasMap map[string]string `json:"alias_map"`
}

// SnapshotNode is a normalized node for snapshot comparison.
type SnapshotNode struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	Pointer    string `json:"pointer"`
	IsTerminal bool   `json:"is_terminal"`
	Provider   *string `json:"provider"`
	StepAs     string `json:"step_as"`
	StepName   string `json:"step_name"`
	HTTPStatus string `json:"http_status"`
}

// SnapshotEdge is a normalized edge for snapshot comparison.
type SnapshotEdge struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// ToSnapshot converts a Graph into a normalized Snapshot suitable for
// cross-implementation comparison with the TypeScript transformer.
func (g *Graph) ToSnapshot() *Snapshot {
	nodes := make([]SnapshotNode, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		nodes = append(nodes, SnapshotNode{
			ID:         n.ID,
			Kind:       string(n.Kind),
			Label:      n.Label,
			Pointer:    n.Pointer,
			IsTerminal: n.IsTerminal,
			Provider:   n.Provider,
			StepAs:     n.StepAs,
			StepName:   n.StepName,
			HTTPStatus: n.HTTPStatus,
		})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	edges := make([]SnapshotEdge, 0, len(g.Edges))
	for _, e := range g.Edges {
		edges = append(edges, SnapshotEdge{
			ID:   e.ID,
			From: e.From,
			To:   e.To,
			Kind: string(e.Kind),
		})
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })

	roots := make([]string, len(g.Roots))
	copy(roots, g.Roots)
	sort.Strings(roots)

	aliasMap := make(map[string]string)
	for k, v := range g.AliasMap {
		aliasMap[k] = v
	}

	return &Snapshot{
		Nodes:    nodes,
		Edges:    edges,
		Roots:    roots,
		AliasMap: aliasMap,
	}
}

// SnapshotJSON returns the snapshot as indented JSON bytes.
func (g *Graph) SnapshotJSON() ([]byte, error) {
	return json.MarshalIndent(g.ToSnapshot(), "", "  ")
}
