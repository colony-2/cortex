package yaml

import (
	"fmt"
)

// NodeVisitor defines an interface for visiting and potentially transforming nodes in a recipe tree
type NodeVisitor interface {
	// VisitNode is called for each node in the tree
	// It returns a potentially modified node and an error
	// If the returned node is nil, the node is removed from the tree
	VisitNode(node *Node, path []string) (*Node, error)

	// PreVisit is called before visiting child nodes (optional)
	// Return false to skip visiting children
	PreVisit(node *Node, path []string) bool

	// PostVisit is called after visiting all child nodes (optional)
	PostVisit(node *Node, path []string) error
}

// Accept implements the visitor pattern for RecipeDefinition
func (r *RecipeDefinition) Accept(visitor NodeVisitor) (*RecipeDefinition, error) {
	if r == nil {
		return nil, nil
	}

	// Create a copy of the recipe definition
	result := &RecipeDefinition{
		Version:     r.Version,
		Defs:        r.Defs, // This might be cleared by visitor
		InputSchema: r.InputSchema,
	}

	// Visit the root node
	rootNode, err := r.Node.Accept(visitor, []string{"root"})
	if err != nil {
		return nil, fmt.Errorf("failed to visit root node: %w", err)
	}
	if rootNode != nil {
		result.Node = *rootNode
	}

	// Visit defs if they exist
	if len(r.Defs) > 0 {
		newDefs := make(map[string]Node)
		for name, def := range r.Defs {
			visitedDef, err := def.Accept(visitor, []string{"defs", name})
			if err != nil {
				return nil, fmt.Errorf("failed to visit def '%s': %w", name, err)
			}
			if visitedDef != nil {
				newDefs[name] = *visitedDef
			}
		}
		result.Defs = newDefs
	}

	return result, nil
}

// Accept implements the visitor pattern for Node
func (n *Node) Accept(visitor NodeVisitor, path []string) (*Node, error) {
	if n == nil {
		return nil, nil
	}

	// Check if we should visit children
	if !visitor.PreVisit(n, path) {
		// Skip visiting children, just visit this node
		return visitor.VisitNode(n, path)
	}

	// Create a working copy of the node
	result := &Node{
		ID:      n.ID,
		Desc:    n.Desc,
		Op:      n.Op,
		Shared:  n.Shared,
		Inputs:  copyMap(n.Inputs),
		Outputs: copyMap(n.Outputs),
		Timeout: n.Timeout,
		Retry:   n.Retry,
		When:    n.When,
	}

	// Visit sequence nodes
	if len(n.Sequence) > 0 {
		newSequence := make([]Node, 0, len(n.Sequence))
		for i, seqNode := range n.Sequence {
			visitedNode, err := seqNode.Accept(visitor, append(path, fmt.Sprintf("sequence[%d]", i)))
			if err != nil {
				return nil, err
			}
			if visitedNode != nil {
				newSequence = append(newSequence, *visitedNode)
			}
		}
		result.Sequence = newSequence
	}

	// Visit parallel nodes
	if len(n.Parallel) > 0 {
		newParallel := make([]Node, 0, len(n.Parallel))
		for i, parNode := range n.Parallel {
			visitedNode, err := parNode.Accept(visitor, append(path, fmt.Sprintf("parallel[%d]", i)))
			if err != nil {
				return nil, err
			}
			if visitedNode != nil {
				newParallel = append(newParallel, *visitedNode)
			}
		}
		result.Parallel = newParallel
	}

	// Visit state machine states
	if n.States != nil {
		newStates := &StateMap{
			Initial: n.States.Initial,
			States:  make(map[string]State),
		}

		for stateName, state := range n.States.States {
			// Visit the embedded node in the state
			stateNode := Node{
				ID:       state.ID,
				Desc:     state.Desc,
				Op:       state.Op,
				Sequence: state.Sequence,
				Parallel: state.Parallel,
				States:   state.States,
				Shared:   state.Shared,
				Inputs:   state.Inputs,
				Outputs:  state.Outputs,
				Timeout:  state.Timeout,
				Retry:    state.Retry,
				When:     state.When,
			}

			visitedStateNode, err := stateNode.Accept(visitor, append(path, "states", stateName))
			if err != nil {
				return nil, err
			}

			if visitedStateNode != nil {
				newStates.States[stateName] = State{
					Node:        *visitedStateNode,
					Transitions: state.Transitions,
					Error:       state.Error,
				}
			}
		}

		result.States = newStates
	}

	// Call PostVisit
	if err := visitor.PostVisit(result, path); err != nil {
		return nil, err
	}

	// Finally, visit this node itself
	return visitor.VisitNode(result, path)
}

// copyMap creates a deep copy of a map[string]interface{}
func copyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}

	copied := make(map[string]interface{}, len(m))
	for k, v := range m {
		// Handle nested maps
		if nestedMap, ok := v.(map[string]interface{}); ok {
			copied[k] = copyMap(nestedMap)
		} else if slice, ok := v.([]interface{}); ok {
			// Handle slices
			copiedSlice := make([]interface{}, len(slice))
			copy(copiedSlice, slice)
			copied[k] = copiedSlice
		} else {
			copied[k] = v
		}
	}
	return copied
}

// mergeInputs merges two input maps, with overrides taking precedence
func mergeInputs(base, override map[string]interface{}) map[string]interface{} {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}

	// Create a copy of base
	result := copyMap(base)

	// Apply overrides
	for k, v := range override {
		result[k] = v
	}

	return result
}

// ResolveSharedReferences creates a new recipe with all shared references resolved
func ResolveSharedReferences(recipe *RecipeDefinition) (*RecipeDefinition, error) {
	if recipe == nil {
		return nil, fmt.Errorf("recipe definition cannot be nil")
	}

	// If there are no defs, return the recipe as-is
	if len(recipe.Defs) == 0 {
		return recipe, nil
	}

	// First, detect cycles in the defs
	if err := detectCycles(recipe.Defs); err != nil {
		return nil, fmt.Errorf("cycle detection failed: %w", err)
	}

	// Create a resolver with caching
	resolver := &CachingSharedReferenceResolver{
		defs:     recipe.Defs,
		resolved: make(map[string]*Node),
	}

	// Apply the resolver to the recipe
	resolved, err := recipe.Accept(resolver)
	if err != nil {
		return nil, err
	}

	// Clear the defs in the resolved recipe
	resolved.Defs = nil

	return resolved, nil
}

// detectCycles checks for circular references in defs
func detectCycles(defs map[string]Node) error {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var visitDef func(name string) error
	visitDef = func(name string) error {
		visited[name] = true
		recStack[name] = true

		def, exists := defs[name]
		if !exists {
			return nil // Skip non-existent defs
		}

		// Check all shared references in this def
		refs := collectSharedRefs(&def)
		for _, ref := range refs {
			if !visited[ref] {
				if err := visitDef(ref); err != nil {
					return err
				}
			} else if recStack[ref] {
				return fmt.Errorf("circular reference detected: %s -> %s", name, ref)
			}
		}

		recStack[name] = false
		return nil
	}

	// Check each def
	for name := range defs {
		if !visited[name] {
			if err := visitDef(name); err != nil {
				return err
			}
		}
	}

	return nil
}

// collectSharedRefs collects all shared references in a node tree
func collectSharedRefs(node *Node) []string {
	if node == nil {
		return nil
	}

	var refs []string

	// Collect this node's shared reference
	if node.Shared != "" {
		refs = append(refs, node.Shared)
	}

	// Recursively collect from sequences
	for _, seqNode := range node.Sequence {
		refs = append(refs, collectSharedRefs(&seqNode)...)
	}

	// Recursively collect from parallels
	for _, parNode := range node.Parallel {
		refs = append(refs, collectSharedRefs(&parNode)...)
	}

	// Recursively collect from states
	if node.States != nil {
		for _, state := range node.States.States {
			stateNode := &state.Node
			refs = append(refs, collectSharedRefs(stateNode)...)
		}
	}

	return refs
}

// CachingSharedReferenceResolver is a visitor that resolves shared references with caching
type CachingSharedReferenceResolver struct {
	defs     map[string]Node
	resolved map[string]*Node // Cache of resolved defs
}

// PreVisit is called before visiting children - we always visit children
func (r *CachingSharedReferenceResolver) PreVisit(node *Node, path []string) bool {
	return true
}

// PostVisit is called after visiting children - nothing to do here
func (r *CachingSharedReferenceResolver) PostVisit(node *Node, path []string) error {
	return nil
}

// VisitNode resolves shared references in the node
func (r *CachingSharedReferenceResolver) VisitNode(node *Node, path []string) (*Node, error) {
	if node == nil {
		return nil, nil
	}

	// If this node has a shared reference, resolve it
	if node.Shared != "" {
		resolvedDef, err := r.resolveDefCached(node.Shared)
		if err != nil {
			return nil, fmt.Errorf("at path %v: %w", path, err)
		}

		// Create a merged node with def as base and node overrides
		merged := &Node{
			// Keep the original node's identity fields
			ID:   node.ID,
			Desc: node.Desc,

			// Copy structural fields from resolved def
			Op:       resolvedDef.Op,
			Sequence: resolvedDef.Sequence,
			Parallel: resolvedDef.Parallel,
			States:   resolvedDef.States,

			// Merge or override other fields
			Inputs:  mergeInputs(resolvedDef.Inputs, node.Inputs),
			Outputs: resolvedDef.Outputs,
			Timeout: resolvedDef.Timeout,
			Retry:   resolvedDef.Retry,
			When:    resolvedDef.When,
		}

		// Override with node-specific values if present
		if node.Desc == "" && resolvedDef.Desc != "" {
			merged.Desc = resolvedDef.Desc
		}
		if node.Timeout != 0 {
			merged.Timeout = node.Timeout
		}
		if node.Retry != nil {
			merged.Retry = node.Retry
		}
		if node.When.String() != "" {
			merged.When = node.When
		}
		if node.Outputs != nil {
			merged.Outputs = node.Outputs
		}

		// Clear the shared reference
		merged.Shared = ""

		return merged, nil
	}

	return node, nil
}

// resolveDefCached resolves a def with caching
func (r *CachingSharedReferenceResolver) resolveDefCached(name string) (*Node, error) {
	// Check cache first
	if resolved, exists := r.resolved[name]; exists {
		return resolved, nil
	}

	// Get the def
	def, exists := r.defs[name]
	if !exists {
		availableKeys := make([]string, 0, len(r.defs))
		for key := range r.defs {
			availableKeys = append(availableKeys, key)
		}
		return nil, fmt.Errorf("shared node reference '%s' not found in defs. Available defs: %v",
			name, availableKeys)
	}

	// Resolve the def recursively (this handles nested shared refs within the def)
	resolvedDef, err := r.resolveDef(&def)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve def '%s': %w", name, err)
	}

	// Cache the result
	r.resolved[name] = resolvedDef

	return resolvedDef, nil
}

// resolveDef recursively resolves all shared references within a def
func (r *CachingSharedReferenceResolver) resolveDef(node *Node) (*Node, error) {
	if node == nil {
		return nil, nil
	}

	// Create a copy of the node
	result := &Node{
		ID:      node.ID,
		Desc:    node.Desc,
		Op:      node.Op,
		Inputs:  copyMap(node.Inputs),
		Outputs: copyMap(node.Outputs),
		Timeout: node.Timeout,
		Retry:   node.Retry,
		When:    node.When,
	}

	// If this node itself has a shared reference, resolve it first
	if node.Shared != "" {
		resolvedDef, err := r.resolveDefCached(node.Shared)
		if err != nil {
			return nil, err
		}

		// Merge with resolved def
		result.Op = resolvedDef.Op
		result.Sequence = resolvedDef.Sequence
		result.Parallel = resolvedDef.Parallel
		result.States = resolvedDef.States
		result.Inputs = mergeInputs(resolvedDef.Inputs, result.Inputs)

		if result.Desc == "" && resolvedDef.Desc != "" {
			result.Desc = resolvedDef.Desc
		}
		if result.Timeout == 0 && resolvedDef.Timeout != 0 {
			result.Timeout = resolvedDef.Timeout
		}
		if result.Retry == nil && resolvedDef.Retry != nil {
			result.Retry = resolvedDef.Retry
		}
		if result.When.String() == "" && resolvedDef.When.String() != "" {
			result.When = resolvedDef.When
		}
		if result.Outputs == nil && resolvedDef.Outputs != nil {
			result.Outputs = copyMap(resolvedDef.Outputs)
		}

		// Clear the shared reference
		result.Shared = ""
		return result, nil
	}

	// Recursively resolve sequences
	if len(node.Sequence) > 0 {
		resolvedSeq := make([]Node, 0, len(node.Sequence))
		for _, seqNode := range node.Sequence {
			resolved, err := r.resolveDef(&seqNode)
			if err != nil {
				return nil, err
			}
			resolvedSeq = append(resolvedSeq, *resolved)
		}
		result.Sequence = resolvedSeq
	}

	// Recursively resolve parallels
	if len(node.Parallel) > 0 {
		resolvedPar := make([]Node, 0, len(node.Parallel))
		for _, parNode := range node.Parallel {
			resolved, err := r.resolveDef(&parNode)
			if err != nil {
				return nil, err
			}
			resolvedPar = append(resolvedPar, *resolved)
		}
		result.Parallel = resolvedPar
	}

	// Recursively resolve states
	if node.States != nil {
		resolvedStates := &StateMap{
			Initial: node.States.Initial,
			States:  make(map[string]State),
		}

		for stateName, state := range node.States.States {
			stateNode := &state.Node
			resolved, err := r.resolveDef(stateNode)
			if err != nil {
				return nil, err
			}

			resolvedStates.States[stateName] = State{
				Node:        *resolved,
				Transitions: state.Transitions,
				Error:       state.Error,
			}
		}

		result.States = resolvedStates
	}

	return result, nil
}
