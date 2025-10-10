package storybuilder

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/story"
	workerops "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/converter"
)

type Builder struct {
	recipeName string
	recipe     *recipe.Recipe

	descriptors map[string]*nodeDescriptor
	nodes       map[string]*StoryNode
	parents     map[string]string

	runsByInvocation map[string]*NodeRun
	runsByScheduleID map[int64]*NodeRun

	timeline  []TimelineEntry
	metadata  StoryMetadata
	converter converter.DataConverter
}

type nodeDescriptor struct {
	Path        string
	Type        string
	DisplayName string
}

func New(recipeName string, r *recipe.Recipe) *Builder {
	desc := buildNodeDescriptors(r)
	b := &Builder{
		recipeName:       recipeName,
		recipe:           r,
		descriptors:      desc,
		nodes:            make(map[string]*StoryNode),
		parents:          make(map[string]string),
		runsByInvocation: make(map[string]*NodeRun),
		runsByScheduleID: make(map[int64]*NodeRun),
		timeline:         make([]TimelineEntry, 0, 64),
		converter:        converter.GetDefaultDataConverter(),
	}
	if r != nil {
		meta := r.GetMetdata()
		b.metadata.RecipeName = recipeName
		if meta.ID != "" {
			b.metadata.RecipeName = meta.ID
		}
	} else {
		b.metadata.RecipeName = recipeName
	}
	return b
}

func (b *Builder) SetExecutionInfo(info *workflowservice.DescribeWorkflowExecutionResponse) {
	if info == nil || info.WorkflowExecutionInfo == nil {
		return
	}
	wfInfo := info.WorkflowExecutionInfo
	b.metadata.WorkflowID = wfInfo.Execution.WorkflowId
	b.metadata.RunID = wfInfo.Execution.RunId
	b.metadata.Status = mapWorkflowStatus(wfInfo.Status)
	if wfInfo.StartTime != nil {
		start := wfInfo.StartTime.AsTime()
		b.metadata.StartedAt = start
	}
	if wfInfo.CloseTime != nil && !wfInfo.CloseTime.AsTime().IsZero() {
		completed := wfInfo.CloseTime.AsTime()
		b.metadata.CompletedAt = &completed
		dur := completed.Sub(b.metadata.StartedAt)
		b.metadata.Duration = &dur
	}
}

func (b *Builder) Process(event *historypb.HistoryEvent) {
	switch event.GetEventType() {
	case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED:
		if b.metadata.StartedAt.IsZero() && event.GetEventTime() != nil {
			b.metadata.StartedAt = event.GetEventTime().AsTime()
		}
		b.addTimeline(event.GetEventId(), event, "workflow-start", "workflow")
	case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED,
		enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_FAILED,
		enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_CANCELED,
		enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_TERMINATED,
		enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_TIMED_OUT:
		b.handleWorkflowClosed(event)
	case enumspb.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED:
		b.handleActivityScheduled(event)
	case enumspb.EVENT_TYPE_ACTIVITY_TASK_STARTED:
		b.handleActivityStarted(event)
	case enumspb.EVENT_TYPE_ACTIVITY_TASK_COMPLETED:
		b.handleActivityCompleted(event)
	case enumspb.EVENT_TYPE_ACTIVITY_TASK_FAILED:
		b.handleActivityFailed(event)
	case enumspb.EVENT_TYPE_ACTIVITY_TASK_TIMED_OUT:
		b.handleActivityTimedOut(event)
	case enumspb.EVENT_TYPE_MARKER_RECORDED:
		b.handleMarker(event)
	case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED:
		b.handleSignal(event)
	}
}

func (b *Builder) Build() *Story {
	root := b.buildTree()
	story := &Story{
		Metadata: b.metadata,
		Nodes:    root.Children,
	}
	sort.Slice(story.Nodes, func(i, j int) bool { return story.Nodes[i].Path < story.Nodes[j].Path })

	sort.Slice(b.timeline, func(i, j int) bool {
		if b.timeline[i].At.Equal(b.timeline[j].At) {
			return b.timeline[i].EventID < b.timeline[j].EventID
		}
		return b.timeline[i].At.Before(b.timeline[j].At)
	})
	story.Timeline = b.timeline

	if story.Metadata.CompletedAt != nil && story.Metadata.Duration == nil {
		d := story.Metadata.CompletedAt.Sub(story.Metadata.StartedAt)
		story.Metadata.Duration = &d
	}

	return story
}

func (b *Builder) handleWorkflowClosed(event *historypb.HistoryEvent) {
	if event.GetEventTime() != nil {
		closed := event.GetEventTime().AsTime()
		b.metadata.CompletedAt = &closed
		d := closed.Sub(b.metadata.StartedAt)
		b.metadata.Duration = &d
	}
	switch event.GetEventType() {
	case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED:
		b.metadata.Status = "completed"
	case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_FAILED:
		b.metadata.Status = "failed"
	case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_CANCELED:
		b.metadata.Status = "canceled"
	case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_TERMINATED:
		b.metadata.Status = "terminated"
	case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_TIMED_OUT:
		b.metadata.Status = "timed_out"
	}
	b.addTimeline(event.GetEventId(), event, "workflow-closed", "workflow")
}

func (b *Builder) handleActivityScheduled(event *historypb.HistoryEvent) {
	attrs := event.GetActivityTaskScheduledEventAttributes()
	if attrs == nil {
		return
	}
	var req workerops.ActivityInvocationRequest
	if err := b.converter.FromPayloads(attrs.Input, &req); err != nil {
		return
	}
	invID := req.Invocation.ID
	if invID == "" {
		invID = req.Invocation.Hash()
	}
	node := b.getOrCreateNode(req.Invocation.NodePath, req.Invocation.RecipeID)
	run := &NodeRun{
		InvocationID: invID,
		Status:       "scheduled",
		Attempt:      1,
		Inputs:       cloneMap(req.Input),
	}
	node.Runs = append(node.Runs, run)
	b.runsByInvocation[invID] = run
	b.runsByScheduleID[event.GetEventId()] = run
	b.appendNodeEvent(node, run, event, "activity-scheduled", map[string]interface{}{"op": attrs.ActivityType.GetName()})
	b.addTimeline(event.GetEventId(), event, "activity-scheduled", nodeTimelineRef(node.Path))
}

func (b *Builder) handleActivityStarted(event *historypb.HistoryEvent) {
	attrs := event.GetActivityTaskStartedEventAttributes()
	if attrs == nil {
		return
	}
	run := b.runsByScheduleID[attrs.ScheduledEventId]
	if run == nil {
		return
	}
	t := event.GetEventTime().AsTime()
	run.StartedAt = &t
	if attrs.Attempt > 0 {
		run.Attempt = int(attrs.Attempt)
	}
	run.Status = "running"
	node := b.findNodeByRun(run)
	b.appendNodeEvent(node, run, event, "activity-started", nil)
	b.addTimeline(event.GetEventId(), event, "activity-started", nodeTimelineRef(node.Path))
}

func (b *Builder) handleActivityCompleted(event *historypb.HistoryEvent) {
	attrs := event.GetActivityTaskCompletedEventAttributes()
	if attrs == nil {
		return
	}
	run := b.runsByScheduleID[attrs.ScheduledEventId]
	if run == nil {
		return
	}
	run.Status = "completed"
	if attrs.Result != nil {
		var outputs map[string]interface{}
		if err := b.converter.FromPayloads(attrs.Result, &outputs); err == nil {
			run.Outputs = cloneMap(outputs)
		}
	}
	if event.GetEventTime() != nil {
		completed := event.GetEventTime().AsTime()
		run.CompletedAt = &completed
	}
	node := b.findNodeByRun(run)
	b.appendNodeEvent(node, run, event, "activity-completed", nil)
	b.addTimeline(event.GetEventId(), event, "activity-completed", nodeTimelineRef(node.Path))
}

func (b *Builder) handleActivityFailed(event *historypb.HistoryEvent) {
	attrs := event.GetActivityTaskFailedEventAttributes()
	if attrs == nil {
		return
	}
	run := b.runsByScheduleID[attrs.ScheduledEventId]
	if run == nil {
		return
	}
	run.Status = "failed"
	if attrs.Failure != nil {
		run.Error = attrs.Failure.GetMessage()
	}
	if event.GetEventTime() != nil {
		failedAt := event.GetEventTime().AsTime()
		run.CompletedAt = &failedAt
	}
	node := b.findNodeByRun(run)
	b.appendNodeEvent(node, run, event, "activity-failed", nil)
	b.addTimeline(event.GetEventId(), event, "activity-failed", nodeTimelineRef(node.Path))
}

func (b *Builder) handleActivityTimedOut(event *historypb.HistoryEvent) {
	attrs := event.GetActivityTaskTimedOutEventAttributes()
	if attrs == nil {
		return
	}
	run := b.runsByScheduleID[attrs.ScheduledEventId]
	if run == nil {
		return
	}
	run.Status = "timed_out"
	if event.GetEventTime() != nil {
		to := event.GetEventTime().AsTime()
		run.CompletedAt = &to
	}
	node := b.findNodeByRun(run)
	b.appendNodeEvent(node, run, event, "activity-timedout", nil)
	b.addTimeline(event.GetEventId(), event, "activity-timedout", nodeTimelineRef(node.Path))
}

func (b *Builder) handleMarker(event *historypb.HistoryEvent) {
	attrs := event.GetMarkerRecordedEventAttributes()
	if attrs == nil || attrs.MarkerName != "SideEffect" {
		return
	}
	env, err := story.DecodeMarkerEnvelope(attrs.Details, b.converter)
	if err != nil || env == nil {
		return
	}

	switch env.Kind {
	case "inline-op":
		b.handleInlineMarker(event, env)
	case "child-recipe":
		b.handleChildRecipeMarker(event, env)
	case "log":
		b.handleInlineLogMarker(event, env)
	}
}

func (b *Builder) handleSignal(event *historypb.HistoryEvent) {
	attrs := event.GetWorkflowExecutionSignaledEventAttributes()
	if attrs == nil {
		return
	}
	name := attrs.GetSignalName()
	const prefix = "user-response:"
	if !strings.HasPrefix(name, prefix) {
		return
	}
	invID := strings.TrimPrefix(name, prefix)
	run := b.runsByInvocation[invID]
	if run == nil {
		return
	}
	var signalPayload map[string]interface{}
	if attrs.Input != nil {
		_ = b.converter.FromPayloads(attrs.Input, &signalPayload)
	}
	node := b.findNodeByRun(run)
	data := map[string]interface{}{
		"signal": name,
	}
	for k, v := range signalPayload {
		data[k] = v
	}
	b.appendNodeEvent(node, run, event, "input-response", data)
	b.addTimeline(event.GetEventId(), event, "input-response", nodeTimelineRef(node.Path), data)
}

func (b *Builder) handleInlineMarker(event *historypb.HistoryEvent, env *story.MarkerEnvelope) {
	payloadMap, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}
	node := b.getOrCreateNode(env.Invocation.NodePath, env.Invocation.RecipeID)
	switch env.Stage {
	case "start":
		var payload story.InlineOpStartPayload
		if err := decodePayload(payloadMap, &payload); err != nil {
			return
		}
		run := &NodeRun{
			InvocationID: ensureInvocationID(env.Invocation),
			Status:       "running",
			Attempt:      len(node.Runs) + 1,
			Inputs:       cloneMap(payload.Inputs),
		}
		if !payload.StartedAt.IsZero() {
			started := payload.StartedAt
			run.StartedAt = &started
		}
		node.Runs = append(node.Runs, run)
		b.runsByInvocation[run.InvocationID] = run
		b.appendNodeEvent(node, run, event, "inline-start", nil)
		b.addTimeline(event.GetEventId(), event, "inline-start", nodeTimelineRef(node.Path))
	case "complete":
		run := b.runsByInvocation[ensureInvocationID(env.Invocation)]
		if run == nil {
			return
		}
		var payload story.InlineOpCompletePayload
		if err := decodePayload(payloadMap, &payload); err == nil {
			run.Outputs = cloneMap(payload.Outputs)
			run.Status = "completed"
			if !payload.CompletedAt.IsZero() {
				completed := payload.CompletedAt
				run.CompletedAt = &completed
			}
		}
		node := b.findNodeByRun(run)
		b.appendNodeEvent(node, run, event, "inline-completed", nil)
		b.addTimeline(event.GetEventId(), event, "inline-completed", nodeTimelineRef(node.Path))
	case "fail":
		run := b.runsByInvocation[ensureInvocationID(env.Invocation)]
		if run == nil {
			return
		}
		var payload story.InlineOpFailPayload
		if err := decodePayload(payloadMap, &payload); err == nil {
			run.Status = "failed"
			run.Error = payload.Message
			if !payload.FailedAt.IsZero() {
				failedAt := payload.FailedAt
				run.CompletedAt = &failedAt
			}
		}
		node := b.findNodeByRun(run)
		b.appendNodeEvent(node, run, event, "inline-failed", nil)
		b.addTimeline(event.GetEventId(), event, "inline-failed", nodeTimelineRef(node.Path))
	case "timeout":
		run := b.runsByInvocation[ensureInvocationID(env.Invocation)]
		if run == nil {
			return
		}
		var payload story.InlineOpTimeoutPayload
		if err := decodePayload(payloadMap, &payload); err == nil {
			run.Status = "timed_out"
			if !payload.TimeoutAt.IsZero() {
				to := payload.TimeoutAt
				run.CompletedAt = &to
			}
		}
		node := b.findNodeByRun(run)
		b.appendNodeEvent(node, run, event, "inline-timeout", nil)
		b.addTimeline(event.GetEventId(), event, "inline-timeout", nodeTimelineRef(node.Path))
	}
}

func (b *Builder) handleChildRecipeMarker(event *historypb.HistoryEvent, env *story.MarkerEnvelope) {
	payloadMap, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}
	node := b.getOrCreateNode(env.Invocation.NodePath, env.Invocation.RecipeID)
	switch env.Stage {
	case "trigger":
		var payload story.ChildRecipeTriggerPayload
		if err := decodePayload(payloadMap, &payload); err != nil {
			return
		}
		run := b.ensureRunForInvocation(env.Invocation)
		run.Status = "running"
		if !payload.TriggeredAt.IsZero() {
			started := payload.TriggeredAt
			run.StartedAt = &started
		}
		if len(payload.Inputs) > 0 {
			run.Inputs = cloneMap(payload.Inputs)
		}
		run.ChildWorkflowID = payload.ChildWorkflowID
		run.ChildRunID = payload.ChildRunID
		if payload.RecipeName != "" {
			run.ChildRecipeName = payload.RecipeName
		}
		data := map[string]interface{}{
			"child_workflow_id": payload.ChildWorkflowID,
			"child_run_id":      payload.ChildRunID,
			"recipe_name":       payload.RecipeName,
		}
		for k, v := range payload.Metadata {
			data[k] = v
		}
		b.appendNodeEvent(node, run, event, "child-trigger", data)
		b.addTimeline(event.GetEventId(), event, "child-trigger", nodeTimelineRef(node.Path), data)
	case "result":
		var payload story.ChildRecipeResultPayload
		if err := decodePayload(payloadMap, &payload); err != nil {
			return
		}
		run := b.ensureRunForInvocation(env.Invocation)
		if payload.Status != "" {
			run.Status = payload.Status
		}
		if !payload.CompletedAt.IsZero() {
			completed := payload.CompletedAt
			run.CompletedAt = &completed
		}
		if len(payload.Outputs) > 0 {
			run.Outputs = cloneMap(payload.Outputs)
		}
		run.ChildWorkflowID = payload.ChildWorkflowID
		run.ChildRunID = payload.ChildRunID
		if payload.RecipeName != "" {
			run.ChildRecipeName = payload.RecipeName
		}
		data := map[string]interface{}{
			"child_workflow_id": payload.ChildWorkflowID,
			"child_run_id":      payload.ChildRunID,
			"status":            payload.Status,
			"recipe_name":       payload.RecipeName,
		}
		if payload.ErrorMessage != "" {
			data["error"] = payload.ErrorMessage
			run.Error = payload.ErrorMessage
		}
		for k, v := range payload.Metadata {
			data[k] = v
		}
		b.appendNodeEvent(node, run, event, "child-result", data)
		b.addTimeline(event.GetEventId(), event, "child-result", nodeTimelineRef(node.Path), data)
	}
}

func (b *Builder) handleInlineLogMarker(event *historypb.HistoryEvent, env *story.MarkerEnvelope) {
	payloadMap, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}
	run := b.ensureRunForInvocation(env.Invocation)
	node := b.getOrCreateNode(env.Invocation.NodePath, env.Invocation.RecipeID)
	b.appendNodeEvent(node, run, event, "inline-log", payloadMap)
	b.addTimeline(event.GetEventId(), event, "inline-log", nodeTimelineRef(node.Path), payloadMap)
}

func (b *Builder) ensureRunForInvocation(inv story.MarkerInvocation) *NodeRun {
	invID := ensureInvocationID(inv)
	if run, ok := b.runsByInvocation[invID]; ok {
		return run
	}
	node := b.getOrCreateNode(inv.NodePath, inv.RecipeID)
	run := &NodeRun{
		InvocationID: invID,
		Status:       "pending",
		Attempt:      len(node.Runs) + 1,
	}
	node.Runs = append(node.Runs, run)
	b.runsByInvocation[invID] = run
	return run
}

func (b *Builder) getOrCreateNode(path, recipeID string) *StoryNode {
	if path == "" {
		path = "root"
	}
	if node, ok := b.nodes[path]; ok {
		return node
	}
	desc := b.descriptors[path]
	node := &StoryNode{
		Path:        path,
		Type:        descType(desc),
		DisplayName: descDisplayName(desc, path),
		Runs:        make([]*NodeRun, 0, 2),
		Events:      make([]StoryEvent, 0, 4),
		Children:    []*StoryNode{},
	}
	b.nodes[path] = node
	b.parents[path] = parentPath(path)
	return node
}

func (b *Builder) findNodeByRun(run *NodeRun) *StoryNode {
	for _, node := range b.nodes {
		for _, candidate := range node.Runs {
			if candidate == run {
				return node
			}
		}
	}
	return nil
}

func (b *Builder) appendNodeEvent(node *StoryNode, run *NodeRun, event *historypb.HistoryEvent, kind string, data map[string]interface{}) {
	if node == nil {
		return
	}
	var ts time.Time
	if event.GetEventTime() != nil {
		ts = event.GetEventTime().AsTime()
	}
	evt := StoryEvent{Kind: kind, At: ts, Data: data}
	node.Events = append(node.Events, evt)
}

func (b *Builder) addTimeline(eventID int64, event *historypb.HistoryEvent, kind, ref string, extras ...map[string]interface{}) {
	entry := TimelineEntry{
		EventID: eventID,
		Kind:    kind,
		Ref:     ref,
	}
	if event.GetEventTime() != nil {
		entry.At = event.GetEventTime().AsTime()
	}
	if len(extras) > 0 && extras[0] != nil {
		entry.Data = extras[0]
	}
	b.timeline = append(b.timeline, entry)
}

func (b *Builder) buildTree() *StoryNode {
	root := &StoryNode{
		Path:        "",
		Type:        "recipe",
		DisplayName: b.metadata.RecipeName,
		Children:    []*StoryNode{},
	}

	nodes := make([]string, 0, len(b.nodes))
	for path := range b.nodes {
		nodes = append(nodes, path)
	}
	sort.Slice(nodes, func(i, j int) bool {
		si := strings.Count(nodes[i], "/")
		sj := strings.Count(nodes[j], "/")
		if si == sj {
			return nodes[i] < nodes[j]
		}
		return si < sj
	})

	index := map[string]*StoryNode{"": root}
	for _, path := range nodes {
		node := b.nodes[path]
		parent := b.parents[path]
		parentNode, ok := index[parent]
		if !ok {
			parentNode = root
		}
		parentNode.Children = append(parentNode.Children, node)
		index[path] = node
	}

	return root
}

func buildNodeDescriptors(r *recipe.Recipe) map[string]*nodeDescriptor {
	desc := make(map[string]*nodeDescriptor)
	if r == nil {
		return desc
	}
	recipeMeta := r.GetMetdata()
	switch impl := r.RecipeImpl.(type) {
	case *recipe.RecipeOp:
		segment := segmentForMetadata(recipeMeta.NodeMetadata, "recipe-op")
		path := segment
		desc[path] = &nodeDescriptor{Path: path, Type: "op", DisplayName: displayName(recipeMeta.NodeMetadata, path)}
	case *recipe.RecipeSequence:
		segment := segmentForMetadata(recipeMeta.NodeMetadata, "recipe-sequence")
		path := segment
		desc[path] = &nodeDescriptor{Path: path, Type: "sequence", DisplayName: displayName(recipeMeta.NodeMetadata, path)}
		traverseSequence(desc, path, impl.SequenceData.Sequence)
	case *recipe.RecipeState:
		segment := segmentForMetadata(recipeMeta.NodeMetadata, "recipe-state")
		path := segment
		desc[path] = &nodeDescriptor{Path: path, Type: "state_machine", DisplayName: displayName(recipeMeta.NodeMetadata, path)}
		traverseStateMap(desc, path, impl.StateData.States)
	}
	return desc
}

func traverseSequence(desc map[string]*nodeDescriptor, parentPath string, nodes recipe.NodeList) {
	for idx, node := range nodes {
		fallback := fmt.Sprintf("seq[%d]", idx)
		meta := node.GetMetadata()
		segment := segmentForMetadata(meta, fallback)
		path := joinPath(parentPath, segment)
		switch impl := node.NodeImpl.(type) {
		case *recipe.NodeOp:
			desc[path] = &nodeDescriptor{Path: path, Type: "op", DisplayName: displayName(meta, segment)}
		case *recipe.NodeSequence:
			desc[path] = &nodeDescriptor{Path: path, Type: "sequence", DisplayName: displayName(meta, segment)}
			traverseSequence(desc, path, impl.SequenceData.Sequence)
		case *recipe.NodeState:
			desc[path] = &nodeDescriptor{Path: path, Type: "state_machine", DisplayName: displayName(meta, segment)}
			traverseStateMap(desc, path, impl.StateData.States)
		}
	}
}

func traverseStateMap(desc map[string]*nodeDescriptor, parentPath string, stateMap *recipe.StateMap) {
	if stateMap == nil {
		return
	}
	for stateName, state := range stateMap.States {
		meta := state.Node.GetMetadata()
		segment := segmentForMetadata(meta, stateName)
		path := joinPath(parentPath, segment)
		desc[path] = &nodeDescriptor{Path: path, Type: "state", DisplayName: displayName(meta, stateName)}
		switch impl := state.Node.NodeImpl.(type) {
		case *recipe.NodeOp:
			// Already recorded as state node; op executes as child? For state with op, path used for the op itself.
			// We can treat op as same path.
		case *recipe.NodeSequence:
			traverseSequence(desc, path, impl.SequenceData.Sequence)
		case *recipe.NodeState:
			traverseStateMap(desc, path, impl.StateData.States)
		}
	}
}

func segmentForMetadata(meta recipe.NodeMetadata, fallback string) string {
	if meta.ID != "" {
		return meta.ID
	}
	if fallback != "" {
		return fallback
	}
	return "node"
}

func joinPath(base, segment string) string {
	if base == "" {
		return segment
	}
	if segment == "" {
		return base
	}
	return base + "/" + segment
}

func parentPath(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx == -1 {
		return ""
	}
	return path[:idx]
}

func displayName(meta recipe.NodeMetadata, fallback string) string {
	if meta.ID != "" {
		return meta.ID
	}
	if meta.Desc != "" {
		return meta.Desc
	}
	return fallback
}

func descType(desc *nodeDescriptor) string {
	if desc == nil || desc.Type == "" {
		return "op"
	}
	return desc.Type
}

func descDisplayName(desc *nodeDescriptor, path string) string {
	if desc == nil || desc.DisplayName == "" {
		segment := path
		if idx := strings.LastIndex(path, "/"); idx != -1 {
			segment = path[idx+1:]
		}
		return segment
	}
	return desc.DisplayName
}

func nodeTimelineRef(path string) string {
	if path == "" {
		return "node"
	}
	return "node:" + path
}

func ensureInvocationID(inv story.MarkerInvocation) string {
	if inv.ID != "" {
		return inv.ID
	}
	return fmt.Sprintf("%s#%d", inv.NodePath, inv.Seq)
}

func decodePayload(payload map[string]interface{}, target interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mapWorkflowStatus(status enumspb.WorkflowExecutionStatus) string {
	switch status {
	case enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return "completed"
	case enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return "running"
	case enumspb.WORKFLOW_EXECUTION_STATUS_FAILED:
		return "failed"
	case enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return "canceled"
	case enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return "terminated"
	case enumspb.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		return "continued_as_new"
	case enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return "timed_out"
	default:
		return "unknown"
	}
}
