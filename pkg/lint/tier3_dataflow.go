package lint

import (
	"fmt"
	"strings"

	"github.com/workato-devs/wk-lint-beta/pkg/igm"
	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

// lintTier3DataFlow runs Tier 3 cross-step data flow rules using the IGM alias map.
func lintTier3DataFlow(parsed *recipe.ParsedRecipe, graph *igm.Graph) []LintDiagnostic {
	var diags []LintDiagnostic

	// Build step providers map: nodeID → provider
	stepProviders := buildStepProviders(graph)

	// Build reachability index: for each node, which other nodes are reachable
	// (backwards — which nodes can provide data to this node)
	reachableFrom := buildReachabilityIndex(graph)

	for i := range parsed.Steps {
		step := &parsed.Steps[i]
		if step.Code.Input == nil {
			continue
		}

		basePath := step.JSONPointer + "/input"
		recipe.WalkStrings(step.Code.Input, basePath, func(pointer, value string) {
			dps := extractDatapills(value)
			for _, dp := range dps {
				if dp.Payload == nil {
					continue
				}

				diags = append(diags, checkDPLineResolves(dp.Payload, pointer, graph.AliasMap)...)
				diags = append(diags, checkDPProviderMatches(dp.Payload, pointer, graph.AliasMap, stepProviders)...)
				diags = append(diags, checkDPStepReachable(dp.Payload, pointer, step, graph, reachableFrom)...)
				diags = append(diags, checkDPTriggerPath(dp.Payload, pointer, parsed)...)
			}
		})
	}

	return diags
}

// checkDPLineResolves verifies that a datapill's line field matches an alias in the recipe.
// Rule: DP_LINE_RESOLVES
func checkDPLineResolves(payload *DatapillPayload, pointer string, aliasMap map[string]string) []LintDiagnostic {
	if payload.Line == "" {
		return nil
	}
	if _, ok := aliasMap[payload.Line]; ok {
		return nil
	}
	return []LintDiagnostic{{
		Level:   LevelWarn,
		Message: fmt.Sprintf("Datapill references step %q which does not match any step alias in the recipe", payload.Line),
		Source:  &SourceRef{JSONPointer: pointer},
		RuleID:  "DP_LINE_RESOLVES",
		Tier:    3,
	}}
}

// checkDPProviderMatches verifies that a datapill's provider matches the resolved step's provider.
// Rule: DP_PROVIDER_MATCHES
func checkDPProviderMatches(payload *DatapillPayload, pointer string, aliasMap map[string]string, stepProviders map[string]string) []LintDiagnostic {
	if payload.Line == "" {
		return nil
	}

	dpProvider, ok := payload.Provider.(string)
	if !ok || dpProvider == "" {
		return nil // null/absent provider — skip (already handled by DP_CATCH_PROVIDER in tier 1)
	}

	nodeID, ok := aliasMap[payload.Line]
	if !ok {
		return nil // unresolved — handled by DP_LINE_RESOLVES
	}

	actualProvider, ok := stepProviders[nodeID]
	if !ok {
		return nil // no provider info
	}

	if dpProvider != actualProvider {
		return []LintDiagnostic{{
			Level:   LevelWarn,
			Message: fmt.Sprintf("Datapill provider %q does not match step %q provider %q", dpProvider, payload.Line, actualProvider),
			Source:  &SourceRef{JSONPointer: pointer},
			RuleID:  "DP_PROVIDER_MATCHES",
			Tier:    3,
		}}
	}
	return nil
}

// checkDPStepReachable verifies that the step referenced by a datapill is reachable
// from the current step (i.e., it executes before the current step in the control flow).
// Rule: DP_STEP_REACHABLE
func checkDPStepReachable(payload *DatapillPayload, pointer string, currentStep *recipe.FlatStep, graph *igm.Graph, reachableFrom map[string]map[string]bool) []LintDiagnostic {
	if payload.Line == "" {
		return nil
	}

	sourceNodeID, ok := graph.AliasMap[payload.Line]
	if !ok {
		return nil // unresolved
	}

	// Find the current step's node ID
	currentNodeID := currentStep.Code.UUID
	if currentNodeID == "" {
		currentNodeID = "ptr:" + currentStep.JSONPointer
	}

	reachable, ok := reachableFrom[currentNodeID]
	if !ok {
		return nil // node not in graph
	}

	if !reachable[sourceNodeID] {
		return []LintDiagnostic{{
			Level:   LevelWarn,
			Message: fmt.Sprintf("Datapill references step %q which is not reachable from current step", payload.Line),
			Source:  &SourceRef{JSONPointer: pointer},
			RuleID:  "DP_STEP_REACHABLE",
			Tier:    3,
		}}
	}
	return nil
}

// checkDPTriggerPath verifies that datapills referencing an API endpoint trigger use
// the correct path format: ["request", "field_name"].
// Rule: DP_TRIGGER_PATH
func checkDPTriggerPath(payload *DatapillPayload, pointer string, parsed *recipe.ParsedRecipe) []LintDiagnostic {
	if !isAPIPlatformTrigger(parsed) {
		return nil
	}

	// Only check datapills referencing the trigger
	if payload.Line == "" {
		return nil
	}
	if len(parsed.Steps) == 0 {
		return nil
	}
	triggerAs := parsed.Steps[0].Code.As
	if payload.Line != triggerAs {
		return nil
	}

	// For API platform triggers, the path should start with "request"
	if len(payload.Path) == 0 {
		return nil
	}

	firstElement, ok := payload.Path[0].(string)
	if !ok {
		return nil
	}

	if !strings.EqualFold(firstElement, "request") {
		return []LintDiagnostic{{
			Level:   LevelInfo,
			Message: fmt.Sprintf("API endpoint datapill path should start with \"request\", got %q", firstElement),
			Source:  &SourceRef{JSONPointer: pointer},
			RuleID:  "DP_TRIGGER_PATH",
			Tier:    3,
		}}
	}
	return nil
}

// --- helpers ---

// buildStepProviders builds a map of nodeID → provider name from the graph.
func buildStepProviders(graph *igm.Graph) map[string]string {
	providers := make(map[string]string)
	for _, n := range graph.Nodes {
		if n.Provider != nil {
			providers[n.ID] = *n.Provider
		}
	}
	return providers
}

// buildReachabilityIndex builds a reverse reachability map:
// for each node, which nodes can "reach" it (i.e., are predecessors in the graph).
// This is used to verify that a datapill source step executes before the consuming step.
func buildReachabilityIndex(graph *igm.Graph) map[string]map[string]bool {
	// Build adjacency list (forward edges, excluding terminal edges to ::end)
	adj := make(map[string][]string)
	for _, e := range graph.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}

	// For each node, compute the set of all ancestors (nodes that can reach it)
	result := make(map[string]map[string]bool)

	for _, n := range graph.Nodes {
		if n.ID == "::end" {
			continue
		}
		// BFS/DFS backwards: find all nodes that can reach this node
		ancestors := make(map[string]bool)
		// Build reverse adjacency
		reverseAdj := make(map[string][]string)
		for _, e := range graph.Edges {
			reverseAdj[e.To] = append(reverseAdj[e.To], e.From)
		}

		visited := make(map[string]bool)
		queue := []string{n.ID}
		visited[n.ID] = true

		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]

			for _, pred := range reverseAdj[cur] {
				if !visited[pred] {
					visited[pred] = true
					ancestors[pred] = true
					queue = append(queue, pred)
				}
			}
		}

		result[n.ID] = ancestors
	}

	return result
}
