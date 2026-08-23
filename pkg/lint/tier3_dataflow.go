package lint

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/workato-devs/recipe-lint/pkg/igm"
	"github.com/workato-devs/recipe-lint/pkg/recipe"
)

// lintTier3DataFlow runs Tier 3 cross-step data flow rules using the IGM alias map.
func lintTier3DataFlow(parsed *recipe.ParsedRecipe, graph *igm.Graph, connRules map[string]*ConnectorRules) []LintDiagnostic {
	var diags []LintDiagnostic

	// Build step providers map: nodeID → provider
	stepProviders := buildStepProviders(graph)

	// Build reachability index: for each node, which other nodes are reachable
	// (backwards — which nodes can provide data to this node)
	reachableFrom := buildReachabilityIndex(graph)

	// Build alias → step map for resolving datapill paths against a step's EOS.
	aliasToStep := make(map[string]*recipe.FlatStep, len(parsed.Steps))
	for i := range parsed.Steps {
		if as := parsed.Steps[i].Code.As; as != "" {
			aliasToStep[as] = &parsed.Steps[i]
		}
	}

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
				diags = append(diags, checkDPPathResolves(dp.Payload, pointer, aliasToStep, connRules)...)
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

// checkDPPathResolves verifies that a datapill's path resolves to a field declared in the
// referenced step's output schema. The recipe's extended_output_schema (EOS)
// remains authoritative where it declares a root. Connector-declared intrinsic
// fields may augment a partial recipe EOS, and connector action schemas may be
// used conservatively when a recipe does not materialize an EOS:
//   - Skips when the line is unresolved (owned by DP_LINE_RESOLVES) — no double-flag.
//   - Applies connector fallback only to action steps with a known provider/action.
//   - Static connector schemas are closed; dynamic schemas validate only their
//     known intrinsic fields and accept unknown runtime-defined roots.
//   - Skips unknown/malformed connector declarations for forward compatibility.
//   - Stops and accepts at any open container (an object/array field with no declared
//     properties) — a dynamic/raw-JSON subtree whose shape isn't materialized in the recipe.
//   - Ignores numeric path segments (array indices); array element fields live under properties.
//
// Rule: DP_PATH_RESOLVES
func checkDPPathResolves(payload *DatapillPayload, pointer string, aliasToStep map[string]*recipe.FlatStep, connRules map[string]*ConnectorRules) []LintDiagnostic {
	if payload.Line == "" {
		return nil
	}
	if len(payload.Path) == 0 {
		return nil // reference to the step's whole output
	}
	if firstPathName(payload.Path) == "" {
		return nil // no named field to validate
	}
	step, ok := aliasToStep[payload.Line]
	if !ok {
		return nil // unresolved alias — owned by DP_LINE_RESOLVES
	}

	recipeFields, err := parseEIS(step.Code.ExtendedOutputSchema)
	if err != nil {
		return nil // malformed recipe EOS remains conservative
	}

	connectorSchema, connectorSchemaOK := actionOutputSchemaForStep(step, connRules)

	// A materialized recipe EOS owns every root it declares. Connector intrinsic
	// fields can augment it because Workato canonical exports may omit those stable
	// transport outputs (for example REST status_code) while retaining response.
	if len(recipeFields) > 0 {
		if firstPathName(payload.Path) != "" && findEISField(recipeFields, firstPathName(payload.Path)) != nil {
			return dpPathDiagnostic(payload, pointer, recipeFields)
		}
		if connectorSchemaOK && pathRootDeclared(connectorSchema.IntrinsicFields, payload.Path) {
			return dpPathDiagnostic(payload, pointer, connectorSchema.IntrinsicFields)
		}
		return missingDPPathDiagnostic(payload, pointer, firstPathName(payload.Path))
	}

	if !connectorSchemaOK {
		return nil
	}

	switch connectorSchema.Kind {
	case "static":
		fields := append(append([]EISField(nil), connectorSchema.Fields...), connectorSchema.IntrinsicFields...)
		return dpPathDiagnostic(payload, pointer, fields)
	case "dynamic":
		if !pathRootDeclared(connectorSchema.IntrinsicFields, payload.Path) {
			return nil
		}
		return dpPathDiagnostic(payload, pointer, connectorSchema.IntrinsicFields)
	default:
		return nil
	}
}

func actionOutputSchemaForStep(step *recipe.FlatStep, connRules map[string]*ConnectorRules) (ActionOutputSchema, bool) {
	if step.Code.Keyword != "action" || step.Code.Provider == nil || step.Code.Name == "" || connRules == nil {
		return ActionOutputSchema{}, false
	}
	rules, ok := connRules[*step.Code.Provider]
	if !ok || rules == nil {
		return ActionOutputSchema{}, false
	}
	raw, ok := rules.ActionOutputSchemas[step.Code.Name]
	if !ok || len(raw) == 0 {
		return ActionOutputSchema{}, false
	}
	var schema ActionOutputSchema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return ActionOutputSchema{}, false
	}
	if schema.Kind != "static" && schema.Kind != "dynamic" {
		return ActionOutputSchema{}, false
	}
	return schema, true
}

func firstPathName(path []interface{}) string {
	for _, seg := range path {
		if name, ok := seg.(string); ok {
			return name
		}
	}
	return ""
}

func pathRootDeclared(fields []EISField, path []interface{}) bool {
	root := firstPathName(path)
	return root != "" && findEISField(fields, root) != nil
}

func dpPathDiagnostic(payload *DatapillPayload, pointer string, fields []EISField) []LintDiagnostic {
	current := fields
	for i, seg := range payload.Path {
		name, isStr := seg.(string)
		if !isStr {
			// numeric array index (or other non-string) — element fields are at the
			// same schema level, so stay put.
			continue
		}
		field := findEISField(current, name)
		if field == nil {
			return missingDPPathDiagnostic(payload, pointer, name)
		}
		if len(field.Properties) == 0 {
			// Open object/array containers accept runtime-defined descendants. Scalar
			// leaves reject a subsequent named segment. Empty field types stay
			// conservative for older connector files.
			nextName := firstPathName(payload.Path[i+1:])
			if nextName != "" && field.Type != "" && field.Type != "object" && field.Type != "array" {
				return missingDPPathDiagnostic(payload, pointer, nextName)
			}
			return nil
		}
		current = field.Properties
	}
	return nil
}

func missingDPPathDiagnostic(payload *DatapillPayload, pointer, name string) []LintDiagnostic {
	return []LintDiagnostic{{
		Level:   LevelWarn,
		Message: fmt.Sprintf("Datapill path field %q is not declared in step %q output schema", name, payload.Line),
		Source:  &SourceRef{JSONPointer: pointer},
		RuleID:  "DP_PATH_RESOLVES",
		Tier:    3,
	}}
}

// findEISField returns the field with the given name (exact match) from a field list, or nil.
// Matching is case-sensitive: a datapill path is generated from the schema, so a case
// difference indicates a hand-edited (broken) reference, which is what this rule catches.
func findEISField(fields []EISField, name string) *EISField {
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
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
