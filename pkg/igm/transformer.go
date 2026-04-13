package igm

import (
	"encoding/json"
	"fmt"
)

// Transform converts raw recipe JSON bytes into an IGM graph.
// This is the Go port of the TypeScript IGM transformer, simplified for linter use.
func Transform(data []byte) (*Graph, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("igm: invalid JSON: %w", err)
	}

	codeRaw, ok := raw["code"]
	if !ok {
		return nil, fmt.Errorf("igm: missing 'code' field")
	}

	ctx := &buildContext{
		graph: &Graph{
			AliasMap: make(map[string]string),
		},
	}

	rootID := ctx.processStep(codeRaw, "/code", "")
	if rootID != "" {
		ctx.graph.Roots = append(ctx.graph.Roots, rootID)
	}

	// Add ::end node
	ctx.graph.Nodes = append(ctx.graph.Nodes, Node{
		ID:      "::end",
		Kind:    NodeEnd,
		Label:   "End",
		Pointer: "/",
	})

	// Connect terminal nodes to ::end
	for _, n := range ctx.graph.Nodes {
		if n.IsTerminal {
			ctx.addEdge(EdgeTerminal, n.ID, "::end")
		}
	}

	// Connect dangling nodes (no outgoing edges) to ::end
	outgoing := make(map[string]bool)
	for _, e := range ctx.graph.Edges {
		outgoing[e.From] = true
	}
	for _, n := range ctx.graph.Nodes {
		if n.ID == "::end" || n.IsTerminal {
			continue
		}
		if !outgoing[n.ID] {
			ctx.addEdge(EdgeNext, n.ID, "::end")
		}
	}

	return ctx.graph, nil
}

// buildContext maintains state during recursive traversal.
type buildContext struct {
	graph         *Graph
	lastIfResult  traversalResult // cached result from last processIf call
	lastTryResult traversalResult // cached result from last processTry call
}

// traversalResult holds the head node ID and tail nodes from processing a step or block.
type traversalResult struct {
	headID string
	tails  []tailNode
}

type tailNode struct {
	id             string
	isTerminal     bool
	needsFalseExit bool // if-without-else: needs false edge to next step
}

// stepBlock is the raw JSON structure of a recipe step.
type stepBlock struct {
	Keyword                string            `json:"keyword"`
	Provider               *string           `json:"provider"`
	Name                   string            `json:"name,omitempty"`
	As                     string            `json:"as,omitempty"`
	Number                 *int              `json:"number,omitempty"`
	UUID                   string            `json:"uuid,omitempty"`
	Input                  json.RawMessage   `json:"input,omitempty"`
	Block                  []json.RawMessage `json:"block,omitempty"`
	ExtendedInputSchema    json.RawMessage   `json:"extended_input_schema,omitempty"`
	ExtendedOutputSchema   json.RawMessage   `json:"extended_output_schema,omitempty"`
}

func (ctx *buildContext) addNode(n Node) {
	ctx.graph.Nodes = append(ctx.graph.Nodes, n)
}

func (ctx *buildContext) addEdge(kind EdgeKind, from, to string) {
	ctx.graph.Edges = append(ctx.graph.Edges, Edge{
		ID:   fmt.Sprintf("%s|%s|%s", kind, from, to),
		From: from,
		To:   to,
		Kind: kind,
	})
}

func (ctx *buildContext) registerAlias(as, nodeID string) {
	if as != "" {
		ctx.graph.AliasMap[as] = nodeID
	}
}

// nodeID returns a stable ID for a step: UUID if present, else pointer-based.
func nodeID(step *stepBlock, pointer string) string {
	if step.UUID != "" {
		return step.UUID
	}
	return "ptr:" + pointer
}

// isExecutableStep checks if a JSON object represents an executable step.
func isExecutableStep(step *stepBlock) bool {
	if step.Keyword == "else" || step.Keyword == "catch" {
		return false
	}
	if step.Keyword != "" {
		return true
	}
	if step.Provider != nil && step.Name != "" {
		return true
	}
	return false
}

// isExplicitTerminal returns true for return_response and return_result actions.
func isExplicitTerminal(step *stepBlock) bool {
	if step.Provider == nil {
		return false
	}
	p := *step.Provider
	return (p == "workato_api_platform" && step.Name == "return_response") ||
		(p == "workato_recipe_function" && step.Name == "return_result")
}

// httpStatusFromInput extracts http_status_code from step input if present.
func httpStatusFromInput(input json.RawMessage) string {
	if input == nil {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(input, &m); err != nil {
		return ""
	}
	raw, ok := m["http_status_code"]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// processStep dispatches to the appropriate step processor.
func (ctx *buildContext) processStep(raw json.RawMessage, pointer, parentID string) string {
	var step stepBlock
	if err := json.Unmarshal(raw, &step); err != nil {
		return ""
	}

	if !isExecutableStep(&step) {
		return ""
	}

	switch step.Keyword {
	case "trigger":
		return ctx.processTrigger(&step, pointer, parentID)
	case "action":
		return ctx.processAction(&step, pointer, parentID)
	case "if":
		return ctx.processIf(&step, pointer, parentID)
	case "try":
		return ctx.processTry(&step, pointer, parentID)
	default:
		// provider+name without keyword → action
		if step.Provider != nil && step.Name != "" {
			return ctx.processAction(&step, pointer, parentID)
		}
		return ""
	}
}

// processStepSequence processes a list of steps and creates sequential edges.
func (ctx *buildContext) processStepSequence(block []json.RawMessage, basePointer, parentID string) traversalResult {
	if len(block) == 0 {
		return traversalResult{}
	}

	var results []traversalResult
	for i, raw := range block {
		pointer := fmt.Sprintf("%s/%d", basePointer, i)

		var step stepBlock
		if err := json.Unmarshal(raw, &step); err != nil {
			continue
		}
		if !isExecutableStep(&step) {
			continue
		}

		result := ctx.processStepToResult(&step, raw, pointer, parentID)
		results = append(results, result)
	}

	if len(results) == 0 {
		return traversalResult{}
	}

	// Create sequential edges between adjacent steps
	for i := 0; i < len(results)-1; i++ {
		cur := results[i]
		next := results[i+1]
		if next.headID == "" {
			continue
		}
		for _, tail := range cur.tails {
			if tail.isTerminal {
				continue
			}
			kind := EdgeNext
			if tail.needsFalseExit {
				kind = EdgeFalse
			}
			ctx.addEdge(kind, tail.id, next.headID)
		}
	}

	firstHead := ""
	for _, r := range results {
		if r.headID != "" {
			firstHead = r.headID
			break
		}
	}
	lastTails := results[len(results)-1].tails

	return traversalResult{headID: firstHead, tails: lastTails}
}

// processStepToResult processes a single step and returns a traversalResult.
func (ctx *buildContext) processStepToResult(step *stepBlock, raw json.RawMessage, pointer, parentID string) traversalResult {
	id := ctx.processStep(raw, pointer, parentID)
	if id == "" {
		return traversalResult{}
	}

	// For simple steps (action, trigger), the tail is the step itself
	// For control flow (if, try), processIf/processTry returns the proper result
	// We need to look up what was created to reconstruct the result
	node := ctx.graph.NodeByID(id)
	if node == nil {
		return traversalResult{headID: id}
	}

	switch node.Kind {
	case NodeIf:
		return ctx.lastIfResult
	case NodeTry:
		return ctx.lastTryResult
	default:
		return traversalResult{
			headID: id,
			tails:  []tailNode{{id: id, isTerminal: node.IsTerminal}},
		}
	}
}

func (ctx *buildContext) processTrigger(step *stepBlock, pointer, parentID string) string {
	id := nodeID(step, pointer)
	ctx.addNode(Node{
		ID:       id,
		Kind:     NodeTrigger,
		Label:    labelOr(step.As, step.Name, "trigger"),
		Pointer:  pointer,
		Provider: step.Provider,
		StepAs:   step.As,
		StepName: step.Name,
		ParentID: parentID,
	})
	ctx.registerAlias(step.As, id)

	if len(step.Block) > 0 {
		blockResult := ctx.processStepSequence(step.Block, pointer+"/block", id)
		if blockResult.headID != "" {
			ctx.addEdge(EdgeNext, id, blockResult.headID)
			return id // tails are handled by the sequence
		}
	}

	return id
}

func (ctx *buildContext) processAction(step *stepBlock, pointer, parentID string) string {
	id := nodeID(step, pointer)
	terminal := isExplicitTerminal(step)

	n := Node{
		ID:         id,
		Kind:       NodeAction,
		Label:      labelOr(step.As, step.Name, "action"),
		Pointer:    pointer,
		IsTerminal: terminal,
		Provider:   step.Provider,
		StepAs:     step.As,
		StepName:   step.Name,
		ParentID:   parentID,
	}
	if terminal {
		n.HTTPStatus = httpStatusFromInput(step.Input)
	}

	ctx.addNode(n)
	ctx.registerAlias(step.As, id)
	return id
}

func (ctx *buildContext) processIf(step *stepBlock, pointer, parentID string) string {
	id := nodeID(step, pointer)
	ctx.addNode(Node{
		ID:       id,
		Kind:     NodeIf,
		Label:    labelOr(step.As, "", "If"),
		Pointer:  pointer,
		StepAs:   step.As,
		ParentID: parentID,
	})
	ctx.registerAlias(step.As, id)

	block := step.Block
	thenSteps, elseContainer, elseIdx := partitionBlock(block, "else")

	// Create then branch
	thenBranchID := id + "::then"
	ctx.addNode(Node{
		ID:       thenBranchID,
		Kind:     NodeBranch,
		Label:    "Then",
		Pointer:  pointer,
		ParentID: id,
	})
	ctx.addEdge(EdgeTrue, id, thenBranchID)

	thenResult := ctx.processStepSequence(thenSteps, pointer+"/block", id)
	if thenResult.headID != "" {
		ctx.addEdge(EdgeNext, thenBranchID, thenResult.headID)
	}

	var branchTails []tailNode
	if len(thenResult.tails) > 0 {
		branchTails = append(branchTails, thenResult.tails...)
	} else {
		branchTails = append(branchTails, tailNode{id: thenBranchID})
	}

	// Process else branch if present
	if elseContainer != nil {
		elsePointer := fmt.Sprintf("%s/block/%d", pointer, elseIdx)
		elseBranchID := id + "::else"

		// Parse the else container to get its block
		var elseParsed stepBlock
		_ = json.Unmarshal(*elseContainer, &elseParsed)

		ctx.addNode(Node{
			ID:       elseBranchID,
			Kind:     NodeBranch,
			Label:    "Else",
			Pointer:  elsePointer,
			ParentID: id,
		})
		ctx.addEdge(EdgeFalse, id, elseBranchID)

		elseSteps := elseParsed.Block
		elseResult := ctx.processStepSequence(elseSteps, elsePointer+"/block", id)
		if elseResult.headID != "" {
			ctx.addEdge(EdgeNext, elseBranchID, elseResult.headID)
		}

		if len(elseResult.tails) > 0 {
			branchTails = append(branchTails, elseResult.tails...)
		} else {
			branchTails = append(branchTails, tailNode{id: elseBranchID})
		}
	}

	// Determine return tails
	nonTerminalTails := filterNonTerminal(branchTails)

	var result traversalResult
	if elseContainer == nil {
		// No else: if node itself is a tail with false exit
		result = traversalResult{
			headID: id,
			tails:  append([]tailNode{{id: id, needsFalseExit: true}}, nonTerminalTails...),
		}
	} else if len(nonTerminalTails) > 0 {
		result = traversalResult{headID: id, tails: nonTerminalTails}
	} else {
		// All terminal
		result = traversalResult{headID: id, tails: filterTerminal(branchTails)}
	}

	ctx.lastIfResult = result
	return id
}

func (ctx *buildContext) processTry(step *stepBlock, pointer, parentID string) string {
	id := nodeID(step, pointer)
	ctx.addNode(Node{
		ID:       id,
		Kind:     NodeTry,
		Label:    labelOr(step.As, "", "Try"),
		Pointer:  pointer,
		StepAs:   step.As,
		ParentID: parentID,
	})
	ctx.registerAlias(step.As, id)

	block := step.Block
	trySteps, catchContainer, catchIdx := partitionBlock(block, "catch")

	// Process try steps
	tryResult := ctx.processStepSequence(trySteps, pointer+"/block", id)
	if tryResult.headID != "" {
		ctx.addEdge(EdgeNext, id, tryResult.headID)
	}

	var branchTails []tailNode
	if len(tryResult.tails) > 0 {
		branchTails = append(branchTails, tryResult.tails...)
	} else {
		branchTails = append(branchTails, tailNode{id: id})
	}

	// Process catch branch if present
	if catchContainer != nil {
		catchPointer := fmt.Sprintf("%s/block/%d", pointer, catchIdx)

		var catchParsed stepBlock
		_ = json.Unmarshal(*catchContainer, &catchParsed)

		catchNodeID := nodeID(&catchParsed, catchPointer)
		ctx.addNode(Node{
			ID:       catchNodeID,
			Kind:     NodeCatch,
			Label:    labelOr(catchParsed.As, "", "Catch"),
			Pointer:  catchPointer,
			StepAs:   catchParsed.As,
			ParentID: id,
		})
		ctx.registerAlias(catchParsed.As, catchNodeID)

		ctx.addEdge(EdgeError, id, catchNodeID)

		catchSteps := catchParsed.Block
		catchResult := ctx.processStepSequence(catchSteps, catchPointer+"/block", id)
		if catchResult.headID != "" {
			ctx.addEdge(EdgeNext, catchNodeID, catchResult.headID)
		}

		if len(catchResult.tails) > 0 {
			branchTails = append(branchTails, catchResult.tails...)
		} else {
			branchTails = append(branchTails, tailNode{id: catchNodeID})
		}
	}

	nonTerminalTails := filterNonTerminal(branchTails)

	var result traversalResult
	if len(nonTerminalTails) > 0 {
		result = traversalResult{headID: id, tails: nonTerminalTails}
	} else {
		result = traversalResult{headID: id, tails: filterTerminal(branchTails)}
	}

	ctx.lastTryResult = result
	return id
}

// partitionBlock separates a block array into main executable steps and a container
// (else or catch). Returns the main steps as raw messages, the container raw message, and its index.
func partitionBlock(block []json.RawMessage, containerKeyword string) ([]json.RawMessage, *json.RawMessage, int) {
	var mainSteps []json.RawMessage
	var container *json.RawMessage
	containerIdx := -1

	for i, raw := range block {
		var peek struct {
			Keyword string `json:"keyword"`
		}
		if err := json.Unmarshal(raw, &peek); err != nil {
			mainSteps = append(mainSteps, raw)
			continue
		}
		if peek.Keyword == containerKeyword {
			if container == nil {
				r := raw // copy for pointer
				container = &r
				containerIdx = i
			}
		} else {
			mainSteps = append(mainSteps, raw)
		}
	}

	return mainSteps, container, containerIdx
}

func filterNonTerminal(tails []tailNode) []tailNode {
	var out []tailNode
	for _, t := range tails {
		if !t.isTerminal {
			out = append(out, t)
		}
	}
	return out
}

func filterTerminal(tails []tailNode) []tailNode {
	var out []tailNode
	for _, t := range tails {
		if t.isTerminal {
			out = append(out, t)
		}
	}
	return out
}

func labelOr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return "unknown"
}
