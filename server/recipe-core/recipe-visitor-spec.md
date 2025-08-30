# Recipe Tree Visitor Pattern Specification

## Overview
Implement a visitor pattern for traversing and transforming recipe node trees. The visitor will walk all nodes in a recipe tree and allow replacement of nodes with alternatives. The first implementation will resolve SharedNode references by replacing them with their actual definitions from RecipeMetadata.Defs.

## Core Components

### 1. NodeVisitor Interface
```go
type NodeVisitor interface {
    // Root node visitors - return (replacement, error)
    VisitRecipeOp(node *RecipeOp, path []string) (*RecipeOp, error)
    VisitRecipeSequence(node *RecipeSequence, path []string) (*RecipeSequence, error)
    VisitRecipeState(node *RecipeState, path []string) (*RecipeState, error)
    
    // Intermediate node visitors - return (replacement, error)
    VisitNodeOp(node *NodeOp, path []string) (Node, error)
    VisitNodeShared(node *NodeShared, path []string) (Node, error)
    VisitNodeSequence(node *NodeSequence, path []string) (Node, error)
    VisitNodeState(node *NodeState, path []string) (Node, error)
    
    // Control whether to traverse children of container nodes
    ShouldTraverseSequenceChildren(node *NodeSequence, path []string) bool
    ShouldTraverseStateChildren(node *NodeState, path []string) bool
}
```

### 2. BaseVisitor (Default Implementation)
```go
// BaseVisitor provides default pass-through implementations
// Embed this to only override specific node types you care about
type BaseVisitor struct {
    walker *NodeWalker  // Set by walker to enable child traversal
}

// Leaf nodes - just return unchanged
func (b *BaseVisitor) VisitNodeOp(node *NodeOp, path []string) (Node, error) {
    return Node{NodeImpl: node}, nil
}

func (b *BaseVisitor) VisitNodeShared(node *NodeShared, path []string) (Node, error) {
    return Node{NodeImpl: node}, nil
}

// Container nodes - must traverse children
func (b *BaseVisitor) VisitNodeSequence(node *NodeSequence, path []string) (Node, error) {
    if b.ShouldTraverseSequenceChildren(node, path) {
        newSequence := &NodeSequence{
            NodeMetadata: node.NodeMetadata,
            SequenceData: SequenceData{
                Inputs:  node.Inputs,
                Outputs: node.Outputs,
            },
        }
        // Walk each child node
        for i, child := range node.Sequence {
            childPath := append(path, fmt.Sprintf("[%d]", i))
            newChild, err := b.walker.WalkNode(child, childPath)
            if err != nil {
                return Node{}, err
            }
            newSequence.Sequence = append(newSequence.Sequence, newChild)
        }
        return Node{NodeImpl: newSequence}, nil
    }
    return Node{NodeImpl: node}, nil
}

func (b *BaseVisitor) VisitNodeState(node *NodeState, path []string) (Node, error) {
    if b.ShouldTraverseStateChildren(node, path) {
        newState := &NodeState{
            NodeMetadata: node.NodeMetadata,
            StateData: StateData{
                Inputs:  node.Inputs,
                Outputs: node.Outputs,
                States: &StateMap{
                    Initial: node.States.Initial,
                    States:  make(map[string]State),
                },
            },
        }
        // Walk each state's node
        for name, state := range node.States.States {
            statePath := append(path, name)
            newNode, err := b.walker.WalkNode(state.Node, statePath)
            if err != nil {
                return Node{}, err
            }
            newState.States.States[name] = State{
                Node:                newNode,
                SingleStateMetadata: state.SingleStateMetadata,
            }
        }
        return Node{NodeImpl: newState}, nil
    }
    return Node{NodeImpl: node}, nil
}

// Root recipe nodes - similar pattern
func (b *BaseVisitor) VisitRecipeSequence(node *RecipeSequence, path []string) (*RecipeSequence, error) {
    // Must traverse the embedded NodeSequence
    innerNode := NodeSequence{
        NodeMetadata: node.NodeMetadata,
        SequenceData: node.SequenceData,
    }
    result, err := b.VisitNodeSequence(&innerNode, path)
    if err != nil {
        return nil, err
    }
    
    if seq, ok := result.NodeImpl.(*NodeSequence); ok {
        return &RecipeSequence{
            RecipeMetadata: node.RecipeMetadata,
            SequenceData:   seq.SequenceData,
        }, nil
    }
    return node, nil
}

// Control methods default to true (always traverse)
func (b *BaseVisitor) ShouldTraverseSequenceChildren(node *NodeSequence, path []string) bool {
    return true
}

func (b *BaseVisitor) ShouldTraverseStateChildren(node *NodeState, path []string) bool {
    return true
}
```

### 3. NodeWalker Struct
```go
type NodeWalker struct {
    visitor NodeVisitor
}

func NewNodeWalker(visitor NodeVisitor) *NodeWalker {
    w := &NodeWalker{visitor: visitor}
    // If visitor is BaseVisitor or embeds it, set the walker reference
    if base, ok := visitor.(*BaseVisitor); ok {
        base.walker = w
    }
    // Handle embedded BaseVisitor
    if v := reflect.ValueOf(visitor).Elem(); v.Kind() == reflect.Struct {
        if field := v.FieldByName("BaseVisitor"); field.IsValid() {
            if base, ok := field.Addr().Interface().(*BaseVisitor); ok {
                base.walker = w
            }
        }
    }
    return w
}

// Walk traverses the entire recipe tree
func (w *NodeWalker) Walk(recipe Recipe) (Recipe, error) {
    switch r := recipe.RecipeImpl.(type) {
    case *RecipeOp:
        newOp, err := w.visitor.VisitRecipeOp(r, []string{"root"})
        if err != nil {
            return recipe, err
        }
        return Recipe{RecipeImpl: newOp}, nil
        
    case *RecipeSequence:
        newSeq, err := w.visitor.VisitRecipeSequence(r, []string{"root"})
        if err != nil {
            return recipe, err
        }
        return Recipe{RecipeImpl: newSeq}, nil
        
    case *RecipeState:
        newState, err := w.visitor.VisitRecipeState(r, []string{"root"})
        if err != nil {
            return recipe, err
        }
        return Recipe{RecipeImpl: newState}, nil
    }
}

// WalkNode recursively processes intermediate nodes
func (w *NodeWalker) WalkNode(node Node, path []string) (Node, error) {
    switch n := node.NodeImpl.(type) {
    case *NodeOp:
        return w.visitor.VisitNodeOp(n, path)
        
    case *NodeShared:
        return w.visitor.VisitNodeShared(n, path)
        
    case *NodeSequence:
        return w.visitor.VisitNodeSequence(n, path)
        
    case *NodeState:
        return w.visitor.VisitNodeState(n, path)
    }
    
    return node, fmt.Errorf("unknown node type: %T", node.NodeImpl)
}
```

### 4. SharedNodeResolver (Example Implementation)
```go
type SharedNodeResolver struct {
    BaseVisitor  // Embed to get default implementations
    defs map[string]Node
    seen map[string]bool
}

// Only override the one method we care about
func (r *SharedNodeResolver) VisitNodeShared(node *NodeShared, path []string) (Node, error) {
    if def, ok := r.defs[node.Shared]; ok {
        // Check for circular reference
        key := strings.Join(path, ".")
        if r.seen[key] {
            return Node{}, fmt.Errorf("circular reference detected: %s", node.Shared)
        }
        r.seen[key] = true
        return def, nil  // Walker will traverse the replacement
    }
    return Node{}, fmt.Errorf("shared node '%s' not found", node.Shared)
}
```


## Tree Traversal Rules

1. **Depth-First**: Visit children before parent processing completes
2. **Node Types to Traverse**:
   - `NodeSequence.Sequence` ([]Node)
   - `NodeState.StateData.States` (map[string]State containing Nodes)
   - `Recipe` root nodes (RecipeSequence, RecipeState, RecipeOp)
3. **Path Tracking**: Maintain path for debugging/error reporting (e.g., ["root", "sequence", "0", "state", "initial"])

## Usage Examples

### Example 1: Resolve Shared Nodes (No Boilerplate)
```go
// Only care about SharedNodes - embed BaseVisitor for everything else
resolver := &SharedNodeResolver{
    BaseVisitor: BaseVisitor{},
    defs: recipe.GetMetadata().Defs,
    seen: make(map[string]bool),
}
walker := NewNodeWalker(resolver)
resolvedRecipe, err := walker.Walk(recipe)
```


### Example 2: Collect Metrics Without Modifying
```go
type MetricsCollector struct {
    BaseVisitor
    opCount   int
    nodeCount int
}

func (m *MetricsCollector) VisitNodeOp(node *NodeOp, path []string) (Node, error) {
    m.opCount++
    m.nodeCount++
    return Node{NodeImpl: node}, nil  // Return unchanged
}

func (m *MetricsCollector) VisitNodeSequence(node *NodeSequence, path []string) (Node, error) {
    m.nodeCount++
    return Node{NodeImpl: node}, nil  // Return unchanged
}
```

## Key Considerations

1. **Immutability**: Create new nodes rather than modifying existing ones
2. **Circular Reference Detection**: Track visited shared nodes to prevent infinite loops
3. **Deep Copy**: When replacing SharedNode, deep copy the definition to avoid aliasing
4. **Metadata Preservation**: Maintain NodeMetadata (ID, Desc, Timeout, etc.) during transformations
5. **Error Context**: Include path information in error messages for debugging

