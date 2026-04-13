package igm

// NodeKind classifies the type of an IGM node.
type NodeKind string

const (
	NodeTrigger NodeKind = "trigger"
	NodeAction  NodeKind = "action"
	NodeIf      NodeKind = "if"
	NodeElse    NodeKind = "else"
	NodeTry     NodeKind = "try"
	NodeCatch   NodeKind = "catch"
	NodeBranch  NodeKind = "branch"
	NodeEnd     NodeKind = "end"
)

// EdgeKind classifies the type of an IGM edge.
type EdgeKind string

const (
	EdgeNext     EdgeKind = "next"
	EdgeTrue     EdgeKind = "true"
	EdgeFalse    EdgeKind = "false"
	EdgeError    EdgeKind = "error"
	EdgeTerminal EdgeKind = "terminal"
)

// Node represents a single node in the IGM graph.
type Node struct {
	ID         string   `json:"id"`
	Kind       NodeKind `json:"kind"`
	Label      string   `json:"label"`
	Pointer    string   `json:"pointer"`              // JSON Pointer into recipe
	IsTerminal bool     `json:"is_terminal,omitempty"` // true for return_response / return_result
	Provider   *string  `json:"provider,omitempty"`    // connector provider name
	StepAs     string   `json:"step_as,omitempty"`     // step alias (used by datapills as "line")
	StepName   string   `json:"step_name,omitempty"`   // action name
	ParentID   string   `json:"parent_id,omitempty"`   // parent node ID for containment
	HTTPStatus string   `json:"http_status,omitempty"` // for return_response actions
}

// Edge represents a directed edge in the IGM graph.
type Edge struct {
	ID   string   `json:"id"`
	From string   `json:"from"`
	To   string   `json:"to"`
	Kind EdgeKind `json:"kind"`
}

// Graph is the complete IGM graph produced by the transformer.
type Graph struct {
	Nodes    []Node            `json:"nodes"`
	Edges    []Edge            `json:"edges"`
	Roots    []string          `json:"roots"`
	AliasMap map[string]string `json:"alias_map"` // step.as → nodeId
}

// NodeByID returns the node with the given ID, or nil if not found.
func (g *Graph) NodeByID(id string) *Node {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}

// OutEdges returns all edges originating from the given node ID.
func (g *Graph) OutEdges(nodeID string) []Edge {
	var out []Edge
	for _, e := range g.Edges {
		if e.From == nodeID {
			out = append(out, e)
		}
	}
	return out
}

// InEdges returns all edges targeting the given node ID.
func (g *Graph) InEdges(nodeID string) []Edge {
	var in []Edge
	for _, e := range g.Edges {
		if e.To == nodeID {
			in = append(in, e)
		}
	}
	return in
}

// Children returns direct child node IDs of a parent (nodes with ParentID == id).
func (g *Graph) Children(parentID string) []Node {
	var children []Node
	for _, n := range g.Nodes {
		if n.ParentID == parentID {
			children = append(children, n)
		}
	}
	return children
}

// TerminalNodes returns all nodes marked as terminals.
func (g *Graph) TerminalNodes() []Node {
	var terminals []Node
	for _, n := range g.Nodes {
		if n.IsTerminal {
			terminals = append(terminals, n)
		}
	}
	return terminals
}
