package cortex

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"sync"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/recipejob"
	workerworkflow "github.com/colony-2/c2j/pkg/worker/workflow"
	"github.com/colony-2/c2j/pkg/workflowctl"
	ca "github.com/colony-2/cortex-assistant/assistant"
	"github.com/colony-2/jobdb/pkg/jobdb"
)

// AssistantRuntimeConfig is trusted server configuration, never a browser DTO.
// CursorKey is a stable, base64-encoded integrity key (at least 32 bytes).
// ExecutionReady is an operator assertion that ordinary workers can run this release.
type AssistantRuntimeConfig struct {
	Root           recipejob.ResolvedTarget `json:"root"`
	Bindings       ca.OrdinaryBindings      `json:"bindings"`
	CursorKey      []byte                   `json:"cursor_key"`
	LegacyReleases []ca.ReleaseDescriptor   `json:"legacy_releases,omitempty"`
	ExecutionReady bool                     `json:"execution_ready"`
}

const assistantRetainedOperations = 128
const assistantOperationBytes = 1 << 20
const assistantRetainedBytes = 32 << 20

type assistantOperationKey struct {
	caller          AssistantCaller
	action, request string
}
type assistantPreparedOperation struct {
	mu       sync.Mutex
	envelope string
	client   *ca.Client // freezes runtime destination and trusted configuration
	frozen   *ca.FrozenStart
}
type assistantFacadeService struct {
	control    *workerworkflow.SWFWorkflowControl
	server     *Server
	client     *ca.Client
	generation string
	mu         sync.Mutex
	operations map[assistantOperationKey]*assistantPreparedOperation
	reserved   int
}

func (s *Server) composeAssistant(cfg AssistantRuntimeConfig, ctl *workerworkflow.SWFWorkflowControl, inputs *input.Runtime) error {
	goals, ok := s.engine.(ca.GoalReader)
	if !ok {
		return ErrAssistantUnavailable
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	a := &assistantFacadeService{server: s, control: ctl, generation: hex.EncodeToString(raw), operations: map[assistantOperationKey]*assistantPreparedOperation{}}
	host := ca.Host{Goals: goals, History: s.engine, Control: ctl, Story: s.story, Input: inputs, Submitter: nativeSubmitter{s.engine},
		Authorize: func(_ context.Context, c ca.Caller, _ string) error {
			if c.TenantID != s.cfg.DefaultTenantID {
				return ca.ErrForbidden
			}
			return nil
		},
		ResolveCell: func(ctx context.Context, c ca.Caller, name string) (recipejob.ResolvedTarget, error) {
			return recipejob.ResolveTarget(ctx, recipejob.ResolveTargetRequest{WorkingDir: s.cfg.WorkingDir, Cell: name, TenantID: c.TenantID, ResolveCell: func(ctx context.Context, name string) (jobdb.ResolvedCell, error) {
				rows, err := s.cells.ListCells(ctx, jobdb.ListCellsRequest{TenantID: c.TenantID, Name: name, PageSize: 2})
				if err != nil {
					return jobdb.ResolvedCell{}, err
				}
				if len(rows.Cells) != 1 {
					return jobdb.ResolvedCell{}, ca.ErrForbidden
				}
				v := rows.Cells[0]
				return jobdb.ResolvedCell{Cell: v.Name, CellID: v.ID, CellRepo: v.Repo}, nil
			}})
		},
	}
	client, err := ca.New(host, ca.Config{Protocol: ca.GoalProtocol, Root: cfg.Root, Recipe: assistantGoalRecipe, RecipeCommit: assistantRecipeCommit, CursorKey: cfg.CursorKey, Bindings: &cfg.Bindings, LegacyReleases: cfg.LegacyReleases})
	if err != nil {
		return assistantFacadeError(err)
	}
	a.client = client
	s.cfg.Assistant = &AssistantIntegration{Service: a, Ready: func(context.Context) bool { return cfg.ExecutionReady }, Generation: a.generation,
		AuthorizeTenant: func(_ context.Context, _ AssistantPrincipal, tenant string) error {
			if tenant != s.cfg.DefaultTenantID {
				return ErrAssistantDenied
			}
			return nil
		},
	}
	return nil
}
func facadeCaller(c AssistantCaller) ca.Caller {
	return ca.Caller{TenantID: c.TenantID, Actor: input.Actor{Kind: "human", ID: c.Principal.ID}}
}
func (a *assistantFacadeService) project(ctx context.Context, c AssistantCaller, v ca.Session, err error) (AssistantSession, error) {
	if err != nil {
		return AssistantSession{}, assistantFacadeError(err)
	}
	record, e := recipejob.ListRecipeJobs(ctx, assistantControlLister{a.control}, recipejob.ListRecipeJobsRequest{TenantID: c.TenantID, JobIDs: []string{v.JobID}, PageSize: 1})
	if e != nil || len(record.Jobs) != 1 {
		return AssistantSession{}, ErrAssistantUnavailable
	}
	inspection, e := a.control.InspectJob(ctx, jobdb.JobKey{TenantId: c.TenantID, JobId: v.JobID})
	if e != nil || inspection.JobKey.JobId != v.JobID || inspection.GoalID != v.GoalID {
		return AssistantSession{}, ErrAssistantUnavailable
	}
	// Story records preserve message origins; live job inspection owns execution
	// state, including cancellation without a final story event.
	if inspection.Terminal {
		v.Status = inspection.Status
		if v.Status == "timed_out" {
			v.Status = "failed"
		}
		v.Current = nil
	} else if record.Jobs[0].CancelRequested {
		v.Status = "closing"
		v.Current = nil
	} else if v.Current == nil && v.Status != "closing" {
		switch record.Jobs[0].Status {
		case jobdb.JobStatusReady, jobdb.JobStatusAwaitingFuture, jobdb.JobStatusPendingJobs:
			v.Status = "queued"
		}
	}
	lifecycle := "legacy"
	// The facade already validates approved legacy descriptors. Select the new
	// lifecycle only from the recorded immutable release, never equal IDs.
	if record.Jobs[0].RecipeName == assistantGoalRecipe {
		lifecycle = "goal"
	}
	return projectAssistantSession(v, lifecycle)
}

// Reserve worst-case storage before capture/preparation, including in-flight
// work. Records are never evicted: admission fails closed at either bound.
func (a *assistantFacadeService) prepared(ctx context.Context, c AssistantCaller, action, request, generation string, envelope any, prepare func() (ca.FrozenStart, error)) (AssistantSession, error) {
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return AssistantSession{}, ErrAssistantInvalid
	}
	key := assistantOperationKey{c, action, request}
	a.mu.Lock()
	op := a.operations[key]
	if op != nil {
		if op.envelope != string(encoded) {
			a.mu.Unlock()
			return AssistantSession{}, ErrAssistantConflict
		}
	} else {
		if generation == "" || generation != a.generation {
			a.mu.Unlock()
			return AssistantSession{}, ErrAssistantUnresolved
		}
		if len(a.operations) >= assistantRetainedOperations || a.reserved+assistantOperationBytes > assistantRetainedBytes {
			a.mu.Unlock()
			return AssistantSession{}, ErrAssistantUnavailable
		}
		op = &assistantPreparedOperation{envelope: string(encoded), client: a.client}
		a.operations[key] = op
		a.reserved += assistantOperationBytes
	}
	a.mu.Unlock()
	op.mu.Lock()
	defer op.mu.Unlock()
	if op.frozen == nil {
		frozen, e := prepare()
		if e != nil {
			return AssistantSession{}, e
		}
		// Do not re-encode nested JSON: retain the complete opaque wrapper as returned.
		size := len(frozen.Payload) + len(frozen.MAC) + len(op.envelope) + 4096 // bounded map/key/wrapper overhead
		if size > assistantOperationBytes {
			return AssistantSession{}, ErrAssistantUnavailable
		}
		op.frozen = &frozen
		a.mu.Lock()
		a.reserved += size - assistantOperationBytes
		a.mu.Unlock()
	}
	v, e := op.client.SubmitFrozenStart(ctx, facadeCaller(c), *op.frozen)
	return a.project(ctx, c, v, e)
}
func (a *assistantFacadeService) Start(ctx context.Context, c AssistantCaller, in AssistantStart) (AssistantSession, error) {
	return a.prepared(ctx, c, "start", in.RequestID, in.Generation, in, func() (ca.FrozenStart, error) {
		snapshot, selected, err := a.server.captureFrozenAssistantSource(ctx, c.TenantID, in.Source)
		if err != nil {
			return ca.FrozenStart{}, err
		}
		v, err := a.client.PrepareStartFrozen(ctx, facadeCaller(c), ca.StartInput{RequestID: in.RequestID, SubmittedAt: in.SubmittedAt, Prompt: in.Prompt, Source: ca.Source(in.Source)}, snapshot, selected)
		return v, assistantFacadeError(err)
	})
}
func (a *assistantFacadeService) List(ctx context.Context, c AssistantCaller, p AssistantPage) (AssistantSessions, error) {
	v, err := a.client.List(ctx, facadeCaller(c), ca.PageRequest{Cursor: p.Cursor, Limit: p.Limit})
	if err != nil {
		return AssistantSessions{}, assistantFacadeError(err)
	}
	out := AssistantSessions{Sessions: []AssistantSession{}, Cursor: v.Cursor}
	for _, s := range v.Sessions {
		projected, err := a.project(ctx, c, s, nil)
		if err != nil {
			return AssistantSessions{}, err
		}
		out.Sessions = append(out.Sessions, projected)
	}
	return out, nil
}
func (a *assistantFacadeService) Get(ctx context.Context, c AssistantCaller, id string) (AssistantSession, error) {
	v, e := a.client.Get(ctx, facadeCaller(c), id)
	return a.project(ctx, c, v, e)
}
func (a *assistantFacadeService) Replay(ctx context.Context, c AssistantCaller, id string, p AssistantPage) (AssistantReplay, error) {
	v, e := a.client.Replay(ctx, facadeCaller(c), id, ca.PageRequest{Cursor: p.Cursor, Limit: p.Limit})
	return projectAssistantReplay(v), assistantFacadeError(e)
}
func (a *assistantFacadeService) Respond(ctx context.Context, c AssistantCaller, id string, in AssistantSubmission) (AssistantReceipt, error) {
	if in.Command != "send" {
		session, e := a.Get(ctx, c, id)
		if e != nil {
			return AssistantReceipt{}, e
		}
		if session.Lifecycle == "goal" {
			return AssistantReceipt{}, ErrAssistantInvalid
		}
	}
	v, e := a.client.Respond(ctx, facadeCaller(c), id, input.StructuredSubmission{RequestID: in.RequestID, SubmissionID: in.SubmissionID, Response: ca.Command{Command: in.Command, Prompt: in.Prompt}})
	if e != nil {
		return AssistantReceipt{}, assistantFacadeError(e)
	}
	if v.Receipt == nil {
		return AssistantReceipt{}, ErrAssistantUnavailable
	}
	return AssistantReceipt{RequestID: v.Receipt.RequestID, SubmissionID: v.Receipt.SubmissionID, SubmittedAt: &v.Receipt.SubmittedAt, Outcome: "accepted"}, nil
}
func (a *assistantFacadeService) Reopen(ctx context.Context, c AssistantCaller, id string) (AssistantSession, error) {
	v, e := a.client.Reopen(ctx, facadeCaller(c), id)
	return a.project(ctx, c, v, e)
}
func (a *assistantFacadeService) Close(ctx context.Context, c AssistantCaller, id string, in AssistantLifecycleRequest) (AssistantSession, error) {
	captured, e := a.client.Get(ctx, facadeCaller(c), in.JobID)
	if e != nil {
		return AssistantSession{}, assistantFacadeError(e)
	}
	if captured.ID != id || captured.GoalID != id {
		return AssistantSession{}, ErrAssistantConflict
	}
	state, e := a.client.Close(ctx, facadeCaller(c), in.JobID)
	if e != nil {
		return AssistantSession{}, assistantFacadeError(e)
	}
	// Get by captured execution, never retarget the cancellation to a successor.
	v, e := a.client.Get(ctx, facadeCaller(c), in.JobID)
	if e == nil && !state.Terminal {
		v.Status = "closing"
	}
	return a.project(ctx, c, v, e)
}
func (a *assistantFacadeService) ReopenGoal(ctx context.Context, c AssistantCaller, id string, in AssistantLifecycleRequest) (AssistantSession, error) {
	envelope := struct {
		ID    string
		Input AssistantLifecycleRequest
	}{id, in}
	return a.prepared(ctx, c, "reopen", in.RequestID, in.Generation, envelope, func() (ca.FrozenStart, error) {
		v, e := a.client.Get(ctx, facadeCaller(c), in.JobID)
		if e != nil {
			return ca.FrozenStart{}, assistantFacadeError(e)
		}
		if v.ID != id || v.GoalID != id {
			return ca.FrozenStart{}, ErrAssistantConflict
		}
		// Reconcile a prior uncertain close of this captured execution first.
		state, e := a.client.Close(ctx, facadeCaller(c), in.JobID)
		if e != nil {
			return ca.FrozenStart{}, assistantFacadeError(e)
		}
		if !state.Terminal {
			return ca.FrozenStart{}, ErrAssistantConflict
		}
		frozen, e := a.client.PrepareReopen(ctx, facadeCaller(c), in.JobID, in.RequestID, in.SubmittedAt)
		return frozen, assistantFacadeError(e)
	})
}
func (a *assistantFacadeService) OpenArtifact(ctx context.Context, c AssistantCaller, id, artifact string) (AssistantDownload, error) {
	if strings.HasPrefix(artifact, "source-") {
		session, e := a.client.Get(ctx, facadeCaller(c), id)
		if e != nil {
			return AssistantDownload{}, assistantFacadeError(e)
		}
		if session.Snapshot != nil {
			for _, d := range session.Snapshot.Documents {
				if d.Outcome != "included" || d.Ref == nil || d.Ref.Stored == nil || d.SHA256 == "" || assistantSourceArtifactID(d) != artifact {
					continue
				}
				value, e := a.server.engine.GetArtifact(c.TenantID, d.Ref.Stored.Key)
				if e != nil {
					return AssistantDownload{}, ErrAssistantUnavailable
				}
				reader, e := value.Open()
				if e != nil {
					return AssistantDownload{}, ErrAssistantUnavailable
				}
				data, e := io.ReadAll(io.LimitReader(reader, ca.SnapshotDocumentMaxBytes+1))
				closeErr := reader.Close()
				sum := sha256.Sum256(data)
				if e != nil || closeErr != nil || int64(len(data)) != d.Size || hex.EncodeToString(sum[:]) != d.SHA256 {
					return AssistantDownload{}, ErrAssistantUnavailable
				}
				return AssistantDownload{ReadCloser: io.NopCloser(bytes.NewReader(data)), Filename: d.Ref.Stored.Key.Name, MIME: d.MIME, Size: int64(len(data))}, nil
			}
		}
		return AssistantDownload{}, ErrAssistantDenied
	}
	v, e := a.client.OpenArtifact(ctx, facadeCaller(c), id, artifact)
	return AssistantDownload{ReadCloser: v.ReadCloser, Filename: v.Filename, MIME: v.MIME, Size: v.Size}, assistantFacadeError(e)
}

// Adapt the existing control listing without introducing a new runtime API.
type assistantControlLister struct{ workflowctl.WorkflowControl }

func (l assistantControlLister) ListJobs(ctx context.Context, q jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	rows, next, e := l.WorkflowControl.ListJobs(ctx, q)
	out := jobdb.ListJobsResponse{NextPageToken: next}
	for _, r := range rows {
		out.Jobs = append(out.Jobs, r.JobSummary)
	}
	return out, e
}
