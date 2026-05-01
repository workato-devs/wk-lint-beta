package lint

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/workato-devs/wk-lint-beta/pkg/igm"
	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

// checkCatchLastInTry verifies that a catch block is the last child in its parent try block.
// Rule: CATCH_LAST_IN_TRY
func checkCatchLastInTry(graph *igm.Graph) []LintDiagnostic {
	var diags []LintDiagnostic

	for _, node := range graph.Nodes {
		if node.Kind != igm.NodeTry {
			continue
		}

		// Get the raw block from the recipe to check ordering.
		// We check via the graph: the catch node's pointer index should be
		// the last element in the parent's block array.
		children := graph.Children(node.ID)
		var catchNode *igm.Node
		maxActionPointerIdx := -1

		for i := range children {
			if children[i].Kind == igm.NodeCatch {
				catchNode = &children[i]
			}
			if children[i].Kind == igm.NodeAction {
				idx := pointerLastIndex(children[i].Pointer)
				if idx > maxActionPointerIdx {
					maxActionPointerIdx = idx
				}
			}
		}

		if catchNode == nil {
			continue
		}

		catchIdx := pointerLastIndex(catchNode.Pointer)
		if catchIdx >= 0 && maxActionPointerIdx >= 0 && catchIdx < maxActionPointerIdx {
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: "Catch block must be the last child in its try block",
				Source:  &SourceRef{JSONPointer: catchNode.Pointer},
				RuleID:  "CATCH_LAST_IN_TRY",
				Tier:    2,
			})
		}
	}
	return diags
}

// checkElseLastInIf verifies that an else block is the last child in its parent if block.
// Rule: ELSE_LAST_IN_IF
func checkElseLastInIf(graph *igm.Graph) []LintDiagnostic {
	var diags []LintDiagnostic

	for _, node := range graph.Nodes {
		if node.Kind != igm.NodeIf {
			continue
		}

		children := graph.Children(node.ID)
		var elseBranch *igm.Node
		maxActionPointerIdx := -1

		for i := range children {
			// The else branch is represented as a "branch" node with label "Else"
			if children[i].Kind == igm.NodeBranch && children[i].Label == "Else" {
				elseBranch = &children[i]
			}
			if children[i].Kind == igm.NodeAction {
				idx := pointerLastIndex(children[i].Pointer)
				if idx > maxActionPointerIdx {
					maxActionPointerIdx = idx
				}
			}
		}

		if elseBranch == nil {
			continue
		}

		elseIdx := pointerLastIndex(elseBranch.Pointer)
		if elseIdx >= 0 && maxActionPointerIdx >= 0 && elseIdx < maxActionPointerIdx {
			diags = append(diags, LintDiagnostic{
				Level:   LevelError,
				Message: "Else block must be the last child in its if block",
				Source:  &SourceRef{JSONPointer: elseBranch.Pointer},
				RuleID:  "ELSE_LAST_IN_IF",
				Tier:    2,
			})
		}
	}
	return diags
}

// checkSuccessBeforeCatch verifies that in an API platform recipe, the success
// return_response is in the try block (before catch), not after.
// Rule: SUCCESS_BEFORE_CATCH
func checkSuccessBeforeCatch(graph *igm.Graph, parsed *recipe.ParsedRecipe) []LintDiagnostic {
	var diags []LintDiagnostic

	// Only applies to API platform triggers
	if !isAPIPlatformTrigger(parsed) {
		return diags
	}

	for _, node := range graph.Nodes {
		if node.Kind != igm.NodeTry {
			continue
		}

		children := graph.Children(node.ID)
		var catchNode *igm.Node
		for i := range children {
			if children[i].Kind == igm.NodeCatch {
				catchNode = &children[i]
				break
			}
		}
		if catchNode == nil {
			continue
		}

		// Find success (2xx) return_response nodes that are descendants of the catch
		catchDescendants := allDescendantIDs(graph, catchNode.ID)
		for _, child := range children {
			if !child.IsTerminal {
				continue
			}
			// A 2xx return in catch path is suspicious
			if child.HTTPStatus != "" && strings.HasPrefix(child.HTTPStatus, "2") {
				if catchDescendants[child.ID] {
					diags = append(diags, LintDiagnostic{
						Level:   LevelWarn,
						Message: fmt.Sprintf("Success response (HTTP %s) is inside catch block — should be in try body", child.HTTPStatus),
						Source:  &SourceRef{JSONPointer: child.Pointer},
						RuleID:  "SUCCESS_BEFORE_CATCH",
						Tier:    2,
					})
				}
			}
		}
	}
	return diags
}

// checkTerminalCoverage verifies that every declared response code in the trigger
// has a corresponding return_response node in the graph.
// Rule: TERMINAL_COVERAGE
func checkTerminalCoverage(graph *igm.Graph, parsed *recipe.ParsedRecipe) []LintDiagnostic {
	var diags []LintDiagnostic

	if !isAPIPlatformTrigger(parsed) {
		return diags
	}

	// Extract declared response codes from trigger input
	declaredCodes := extractDeclaredResponseCodes(parsed)
	if len(declaredCodes) == 0 {
		return diags
	}

	// Collect all HTTP status codes from terminal nodes
	coveredCodes := make(map[string]bool)
	for _, n := range graph.TerminalNodes() {
		if n.HTTPStatus != "" {
			coveredCodes[n.HTTPStatus] = true
		}
	}

	for _, code := range declaredCodes {
		if !coveredCodes[code] {
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: fmt.Sprintf("Declared response code %s has no corresponding return_response", code),
				Source:  &SourceRef{JSONPointer: "/code/input/response"},
				RuleID:  "TERMINAL_COVERAGE",
				Tier:    2,
			})
		}
	}
	return diags
}

// checkAllPathsReturn verifies that every control flow path terminates in a return_response
// (or return_result). A non-terminal dangling path is flagged.
// Rule: ALL_PATHS_RETURN
func checkAllPathsReturn(graph *igm.Graph) []LintDiagnostic {
	var diags []LintDiagnostic

	// Find non-terminal nodes whose only outgoing edge goes to ::end via "next" (not terminal)
	for _, e := range graph.Edges {
		if e.To != "::end" || e.Kind == igm.EdgeTerminal {
			continue
		}
		// This is a non-terminal node flowing to ::end
		node := graph.NodeByID(e.From)
		if node == nil {
			continue
		}
		// Skip virtual nodes (branch, end) — they don't represent real steps
		if node.Kind == igm.NodeBranch || node.Kind == igm.NodeEnd {
			continue
		}
		// If this is a "next" edge to ::end, it means a path doesn't terminate with a return
		diags = append(diags, LintDiagnostic{
			Level:   LevelWarn,
			Message: fmt.Sprintf("Control flow path ending at %q does not terminate with return_response/return_result", node.Label),
			Source:  &SourceRef{JSONPointer: node.Pointer},
			RuleID:  "ALL_PATHS_RETURN",
			Tier:    2,
		})
	}
	return diags
}

// checkCatchReturnsAllFields verifies that return_response actions inside catch blocks
// provide all required response body fields.
// Rule: CATCH_RETURNS_ALL_FIELDS
func checkCatchReturnsAllFields(graph *igm.Graph, parsed *recipe.ParsedRecipe) []LintDiagnostic {
	var diags []LintDiagnostic

	if !isAPIPlatformTrigger(parsed) {
		return diags
	}

	// Get required fields from declared response schemas
	requiredFieldsByCode := extractResponseFieldsByCode(parsed)
	if len(requiredFieldsByCode) == 0 {
		return diags
	}

	// Find catch nodes and their descendant terminal nodes
	for _, node := range graph.Nodes {
		if node.Kind != igm.NodeCatch {
			continue
		}

		catchDescendants := allDescendantIDs(graph, node.ID)
		for _, tn := range graph.TerminalNodes() {
			if !catchDescendants[tn.ID] {
				continue
			}
			if tn.HTTPStatus == "" {
				continue
			}

			requiredFields, ok := requiredFieldsByCode[tn.HTTPStatus]
			if !ok {
				continue
			}

			// Find the actual step to check its input.response fields
			providedFields := getReturnResponseFields(parsed, tn.ID)
			for _, field := range requiredFields {
				if !providedFields[field] {
					diags = append(diags, LintDiagnostic{
						Level:   LevelWarn,
						Message: fmt.Sprintf("Catch return_response (HTTP %s) missing required field %q", tn.HTTPStatus, field),
						Source:  &SourceRef{JSONPointer: tn.Pointer},
						RuleID:  "CATCH_RETURNS_ALL_FIELDS",
						Tier:    2,
					})
				}
			}
		}
	}
	return diags
}

// checkRecipeCallZipName verifies that recipe function calls include a zip_name.
// Rule: RECIPE_CALL_ZIP_NAME
func checkRecipeCallZipName(graph *igm.Graph, parsed *recipe.ParsedRecipe) []LintDiagnostic {
	var diags []LintDiagnostic

	for _, node := range graph.Nodes {
		if node.Kind != igm.NodeAction {
			continue
		}
		if node.Provider == nil || *node.Provider != "workato_recipe_function" {
			continue
		}
		if node.StepName != "call_recipe" {
			continue
		}

		// Find the step in parsed recipe and check input.flow_id.zip_name
		step := findStepByUUID(parsed, node.ID)
		if step == nil {
			continue
		}

		if step.Code.Input == nil {
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: "Recipe call is missing input.flow_id.zip_name",
				Source:  &SourceRef{JSONPointer: node.Pointer + "/input"},
				RuleID:  "RECIPE_CALL_ZIP_NAME",
				Tier:    2,
			})
			continue
		}

		var input map[string]json.RawMessage
		if err := json.Unmarshal(step.Code.Input, &input); err != nil {
			continue
		}

		flowIDRaw, ok := input["flow_id"]
		if !ok {
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: "Recipe call is missing input.flow_id.zip_name",
				Source:  &SourceRef{JSONPointer: node.Pointer + "/input"},
				RuleID:  "RECIPE_CALL_ZIP_NAME",
				Tier:    2,
			})
			continue
		}

		var flowID map[string]json.RawMessage
		if err := json.Unmarshal(flowIDRaw, &flowID); err != nil {
			continue
		}

		if _, ok := flowID["zip_name"]; !ok {
			diags = append(diags, LintDiagnostic{
				Level:   LevelWarn,
				Message: "Recipe call is missing input.flow_id.zip_name",
				Source:  &SourceRef{JSONPointer: node.Pointer + "/input/flow_id"},
				RuleID:  "RECIPE_CALL_ZIP_NAME",
				Tier:    2,
			})
		}
	}
	return diags
}

// --- helpers ---

// isAPIPlatformTrigger checks if the recipe trigger is workato_api_platform.
func isAPIPlatformTrigger(parsed *recipe.ParsedRecipe) bool {
	if len(parsed.Steps) == 0 {
		return false
	}
	trigger := parsed.Steps[0]
	return trigger.Code.Provider != nil && *trigger.Code.Provider == "workato_api_platform"
}

// extractDeclaredResponseCodes extracts HTTP status codes from trigger responses.
func extractDeclaredResponseCodes(parsed *recipe.ParsedRecipe) []string {
	if len(parsed.Steps) == 0 {
		return nil
	}
	trigger := parsed.Steps[0]
	if trigger.Code.Input == nil {
		return nil
	}

	var input map[string]json.RawMessage
	if err := json.Unmarshal(trigger.Code.Input, &input); err != nil {
		return nil
	}

	responseRaw, ok := input["response"]
	if !ok {
		return nil
	}

	var response struct {
		Responses []struct {
			HTTPStatusCode string `json:"http_status_code"`
		} `json:"responses"`
	}
	if err := json.Unmarshal(responseRaw, &response); err != nil {
		return nil
	}

	var codes []string
	for _, r := range response.Responses {
		if r.HTTPStatusCode != "" {
			codes = append(codes, r.HTTPStatusCode)
		}
	}
	return codes
}

// extractResponseFieldsByCode returns map[statusCode][]fieldName from trigger response schemas.
func extractResponseFieldsByCode(parsed *recipe.ParsedRecipe) map[string][]string {
	if len(parsed.Steps) == 0 {
		return nil
	}
	trigger := parsed.Steps[0]
	if trigger.Code.Input == nil {
		return nil
	}

	var input map[string]json.RawMessage
	if err := json.Unmarshal(trigger.Code.Input, &input); err != nil {
		return nil
	}

	responseRaw, ok := input["response"]
	if !ok {
		return nil
	}

	var response struct {
		Responses []struct {
			HTTPStatusCode string `json:"http_status_code"`
			BodySchema     string `json:"body_schema"`
		} `json:"responses"`
	}
	if err := json.Unmarshal(responseRaw, &response); err != nil {
		return nil
	}

	result := make(map[string][]string)
	for _, r := range response.Responses {
		if r.BodySchema == "" || r.HTTPStatusCode == "" {
			continue
		}
		var fields []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(r.BodySchema), &fields); err != nil {
			continue
		}
		var names []string
		for _, f := range fields {
			names = append(names, f.Name)
		}
		result[r.HTTPStatusCode] = names
	}
	return result
}

// getReturnResponseFields returns the set of field names provided in a return_response's input.response.
func getReturnResponseFields(parsed *recipe.ParsedRecipe, nodeID string) map[string]bool {
	step := findStepByUUID(parsed, nodeID)
	if step == nil || step.Code.Input == nil {
		return nil
	}

	var input map[string]json.RawMessage
	if err := json.Unmarshal(step.Code.Input, &input); err != nil {
		return nil
	}

	responseRaw, ok := input["response"]
	if !ok {
		return nil
	}

	var responseFields map[string]json.RawMessage
	if err := json.Unmarshal(responseRaw, &responseFields); err != nil {
		return nil
	}

	result := make(map[string]bool)
	for k := range responseFields {
		result[k] = true
	}
	return result
}

// findStepByUUID finds a step in the parsed recipe by its UUID.
func findStepByUUID(parsed *recipe.ParsedRecipe, uuid string) *recipe.FlatStep {
	for i := range parsed.Steps {
		if parsed.Steps[i].Code.UUID == uuid {
			return &parsed.Steps[i]
		}
	}
	return nil
}

// allDescendantIDs returns a set of all node IDs that are descendants of the given node.
func allDescendantIDs(graph *igm.Graph, rootID string) map[string]bool {
	result := make(map[string]bool)
	var walk func(id string)
	walk = func(id string) {
		for _, child := range graph.Children(id) {
			result[child.ID] = true
			walk(child.ID)
		}
	}
	walk(rootID)
	return result
}

// pointerLastIndex extracts the last numeric segment from a JSON pointer.
func pointerLastIndex(pointer string) int {
	parts := strings.Split(pointer, "/")
	if len(parts) == 0 {
		return -1
	}
	last := parts[len(parts)-1]
	var n int
	if _, err := fmt.Sscanf(last, "%d", &n); err != nil {
		return -1
	}
	return n
}
