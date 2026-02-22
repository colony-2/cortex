package recipetesting

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	recipecore "github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	coretask "github.com/colony-2/colony2/server/recipe-core/pkg/task"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	"github.com/colony-2/colony2/server/recipe-template/pkg/template"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	workerops "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	recipesvc "github.com/colony-2/colony2/server/recipes/pkg/recipe"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/go-playground/validator/v10"
)

var recipeTestReqValidator = validator.New()

type recipeTestCaseRequest struct {
	TargetRecipe recipeTestTargetRecipe `json:"target_recipe" validate:"required"`
	Case         recipeTestCase         `json:"case" validate:"required"`
	Execution    recipeTestExecution    `json:"execution,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

type recipeTestTargetRecipe struct {
	Mode    string `json:"mode" validate:"required,oneof=server_ref inline_recipe"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Format  string `json:"format,omitempty" validate:"omitempty,oneof=yaml json"`
	Content string `json:"content,omitempty"`
}

type recipeTestCase struct {
	ID          string                 `json:"id" validate:"required"`
	Type        string                 `json:"type" validate:"required,oneof=op_case recipe_case integration_case"`
	Target      map[string]interface{} `json:"target,omitempty"`
	Inputs      map[string]interface{} `json:"inputs,omitempty"`
	Mocks       recipeTestMocks        `json:"mocks,omitempty"`
	Assertions  []recipeTestAssertion  `json:"assertions,omitempty"`
	Evaluations []recipeTestEvaluation `json:"evaluations,omitempty"`
	Options     map[string]interface{} `json:"options,omitempty"`
}

type recipeTestMocks struct {
	Ops []recipeTestOpMock `json:"ops,omitempty" validate:"omitempty,dive"`
}

type recipeTestOpMock struct {
	Match    recipeTestOpMockMatch  `json:"match" validate:"required"`
	Behavior recipeTestMockBehavior `json:"behavior" validate:"required"`
}

type recipeTestOpMockMatch struct {
	NodePath string `json:"node_path,omitempty"`
	Op       string `json:"op,omitempty"`
}

type recipeTestMockBehavior struct {
	Mode        string            `json:"mode" validate:"required,oneof=return fail passthrough record_passthrough replay"`
	Outputs     map[string]any    `json:"outputs,omitempty"`
	Artifacts   map[string]string `json:"artifacts,omitempty"`
	Error       *recipeTestErr    `json:"error,omitempty"`
	CassetteKey string            `json:"cassette_key,omitempty"`
}

type recipeTestErr struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type recipeTestAssertion struct {
	Type     string      `json:"type" validate:"required,oneof=output_equals output_matches artifact_exists artifact_json_equals node_executed node_not_executed status_is cel_true"`
	Path     string      `json:"path,omitempty"`
	Value    interface{} `json:"value,omitempty"`
	Regex    string      `json:"regex,omitempty"`
	JsonPath string      `json:"json_path,omitempty"`
	NodePath string      `json:"node_path,omitempty"`
	Status   string      `json:"status,omitempty"`
	Expr     string      `json:"expr,omitempty"`
}

type recipeTestEvaluation struct {
	ID     string                 `json:"id" validate:"required"`
	Type   string                 `json:"type" validate:"required,oneof=text_pattern llm_judge"`
	Mode   string                 `json:"mode,omitempty" validate:"omitempty,oneof=enforce report_only"`
	Source recipeTestEvalSource   `json:"source" validate:"required"`
	Config map[string]interface{} `json:"config,omitempty"`
}

type recipeTestEvalSource struct {
	Kind           string `json:"kind" validate:"required,oneof=artifact artifact_glob output_path trace"`
	Path           string `json:"path,omitempty"`
	Glob           string `json:"glob,omitempty"`
	OutputPath     string `json:"output_path,omitempty"`
	TraceName      string `json:"trace_name,omitempty"`
	AllowSensitive bool   `json:"allow_sensitive,omitempty"`
}

type recipeTestExecution struct {
	Mode             string `json:"mode,omitempty" validate:"omitempty,oneof=isolated"`
	Timeout          string `json:"timeout,omitempty"`
	ArtifactMode     string `json:"artifact_mode,omitempty" validate:"omitempty,oneof=none inline"`
	ArtifactMaxBytes int64  `json:"artifact_max_bytes,omitempty" validate:"omitempty,gte=1"`
	EvaluationMode   string `json:"evaluation_mode,omitempty" validate:"omitempty,oneof=enforce report_only"`
}

type recipeTestValidateResponse struct {
	Valid    bool              `json:"valid"`
	CaseHash string            `json:"case_hash"`
	Errors   []recipeTestIssue `json:"errors,omitempty"`
	Warnings []recipeTestIssue `json:"warnings,omitempty"`
}

type recipeTestIssue struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type recipeTestExecuteResponse struct {
	CaseId          string                          `json:"case_id"`
	Status          string                          `json:"status"`
	CaseHash        string                          `json:"case_hash"`
	DurationMs      int64                           `json:"duration_ms"`
	FailureCategory string                          `json:"failure_category,omitempty"`
	FailureReason   string                          `json:"failure_reason,omitempty"`
	Outputs         map[string]interface{}          `json:"outputs,omitempty"`
	Assertions      []recipeTestAssertionResult     `json:"assertions,omitempty"`
	Evaluations     []recipeTestEvaluationResult    `json:"evaluations,omitempty"`
	Diagnostics     recipeTestDiagnostics           `json:"diagnostics,omitempty"`
	Artifacts       map[string]recipeInlineArtifact `json:"artifacts,omitempty"`
}

type recipeTestAssertionResult struct {
	Type     string      `json:"type"`
	Passed   bool        `json:"passed"`
	Message  string      `json:"message,omitempty"`
	Expected interface{} `json:"expected,omitempty"`
	Actual   interface{} `json:"actual,omitempty"`
}

type recipeTestEvaluationResult struct {
	Id            string                 `json:"id"`
	Type          string                 `json:"type"`
	Mode          string                 `json:"mode"`
	Passed        bool                   `json:"passed"`
	ErrorCategory string                 `json:"error_category,omitempty"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	Output        map[string]interface{} `json:"output,omitempty"`
}

type recipeInlineArtifact struct {
	ContentBase64 string `json:"content_base64"`
	SizeBytes     int64  `json:"size_bytes"`
	Truncated     bool   `json:"truncated"`
}

type recipeTestDiagnostics struct {
	MockHits   []recipeTestMockHit  `json:"mock_hits,omitempty"`
	MockMisses []recipeTestMockMiss `json:"mock_misses,omitempty"`
}

type recipeTestMockHit struct {
	NodePath string `json:"node_path"`
	Op       string `json:"op"`
	Mode     string `json:"mode"`
}

type recipeTestMockMiss struct {
	NodePath string `json:"node_path"`
	Op       string `json:"op"`
	Reason   string `json:"reason"`
}

type CaseRequest = recipeTestCaseRequest
type ValidateResponse = recipeTestValidateResponse
type ExecuteResponse = recipeTestExecuteResponse
type Issue = recipeTestIssue

type PreparedCase struct {
	Recipe       *recipecore.Recipe
	ResolvedHash string
	Validation   ValidateResponse
}

type Service struct {
	recipeSvc   recipesvc.Service
	deps        coreops.ServiceDependencies2
	celProvider template.CELOptionsProvider
}

func NewService(recipeSvc recipesvc.Service, deps coreops.ServiceDependencies2, provider ...template.CELOptionsProvider) *Service {
	if deps == nil {
		deps = coreops.NewServiceDepsBuilder().Build()
	}
	var celProvider template.CELOptionsProvider
	if len(provider) > 0 {
		celProvider = provider[0]
	}
	return &Service{
		recipeSvc:   recipeSvc,
		deps:        deps,
		celProvider: celProvider,
	}
}

func DecodeRequest(r io.Reader) (CaseRequest, []Issue) {
	var req recipeTestCaseRequest
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return req, []recipeTestIssue{{Code: "invalid_request", Message: err.Error()}}
	}
	if err := recipeTestReqValidator.Struct(req); err != nil {
		return req, validationIssues(err)
	}
	return req, validateRecipeSemantics(req)
}

func validationIssues(err error) []recipeTestIssue {
	out := make([]recipeTestIssue, 0)
	if verrs, ok := err.(validator.ValidationErrors); ok {
		for _, fe := range verrs {
			out = append(out, recipeTestIssue{Code: "validation_error", Field: fe.Namespace(), Message: fe.Error()})
		}
		return out
	}
	return []recipeTestIssue{{Code: "validation_error", Message: err.Error()}}
}

func validateRecipeSemantics(req recipeTestCaseRequest) []recipeTestIssue {
	issues := make([]recipeTestIssue, 0)
	target := req.TargetRecipe
	switch target.Mode {
	case "server_ref":
		hasVersion := strings.TrimSpace(target.Version) != ""
		hasRef := strings.TrimSpace(target.Ref) != ""
		if strings.TrimSpace(target.Name) == "" || hasVersion == hasRef {
			issues = append(issues, recipeTestIssue{Code: "invalid_target", Field: "target_recipe", Message: "server_ref requires name and exactly one of version/ref"})
		}
	case "inline_recipe":
		if strings.TrimSpace(target.Content) == "" {
			issues = append(issues, recipeTestIssue{Code: "invalid_target", Field: "target_recipe.content", Message: "inline_recipe requires content"})
		}
	}
	if req.Case.Type == "op_case" {
		nodePath, _ := req.Case.Target["node_path"].(string)
		if strings.TrimSpace(nodePath) == "" {
			issues = append(issues, recipeTestIssue{Code: "invalid_case", Field: "case.target.node_path", Message: "op_case requires target.node_path"})
		}
	}
	for i, m := range req.Case.Mocks.Ops {
		if strings.TrimSpace(m.Match.NodePath) == "" && strings.TrimSpace(m.Match.Op) == "" {
			issues = append(issues, recipeTestIssue{Code: "invalid_mock", Field: fmt.Sprintf("case.mocks.ops[%d].match", i), Message: "at least one matcher field is required"})
		}
		if m.Behavior.Mode == "replay" && strings.TrimSpace(m.Behavior.CassetteKey) == "" {
			issues = append(issues, recipeTestIssue{Code: "invalid_mock", Field: fmt.Sprintf("case.mocks.ops[%d].behavior.cassette_key", i), Message: "replay mode requires cassette_key"})
		}
	}
	if req.Case.Type == "op_case" {
		if _, ok := parseOpCaseTarget(req.Case); !ok {
			issues = append(issues, recipeTestIssue{Code: "invalid_case", Field: "case.target.node_path", Message: "op_case requires non-empty target.node_path"})
		}
	}
	if req.Case.Options != nil {
		policyOpts := getMap(req.Case.Options, "policy")
		for _, dep := range stringSlice(policyOpts["required_dependencies"]) {
			if !isSupportedDependencyName(dep) {
				issues = append(issues, recipeTestIssue{Code: "invalid_option", Field: "case.options.policy.required_dependencies", Message: "unsupported dependency name: " + dep})
			}
		}
	}
	return issues
}

func (s *Service) Prepare(ctx context.Context, projectID project.ID, req recipeTestCaseRequest) PreparedCase {
	validate := recipeTestValidateResponse{Valid: false}
	recipeDef, recipeHash, errs, warns := s.resolveRecipeTestTarget(ctx, projectID, req.TargetRecipe)
	caseHash := computeCaseHash(recipeHash, req.Case)
	validate.CaseHash = caseHash
	validate.Errors = append(errs, validateRecipeSemantics(req)...)
	validate.Errors = append(validate.Errors, s.validateDependencyAvailability(req)...)
	validate.Warnings = warns
	validate.Valid = len(validate.Errors) == 0
	return PreparedCase{Recipe: recipeDef, ResolvedHash: recipeHash, Validation: validate}
}

func (s *Service) resolveRecipeTestTarget(ctx context.Context, projectID project.ID, target recipeTestTargetRecipe) (*recipecore.Recipe, string, []recipeTestIssue, []recipeTestIssue) {
	errorsList := make([]recipeTestIssue, 0)
	warnings := make([]recipeTestIssue, 0)
	if target.Mode == "server_ref" {
		if s.recipeSvc == nil {
			return nil, "", []recipeTestIssue{{Code: "service_unavailable", Message: "recipe service unavailable"}}, warnings
		}
		ref := target.Ref
		if ref == "" {
			ref = target.Version
		}
		recipeWithContent, err := s.recipeSvc.GetRecipe(ctx, projectID, target.Name, ref)
		if err != nil {
			return nil, "", []recipeTestIssue{{Code: "target_not_found", Field: "target_recipe", Message: err.Error()}}, warnings
		}
		recipeDef, dynamicWarnings, parseErr := loadRecipeWithDynamicOpStubs(recipeWithContent.Content)
		warnings = append(warnings, dynamicWarnings...)
		if parseErr != nil {
			return nil, "", []recipeTestIssue{{Code: "invalid_recipe", Field: "target_recipe", Message: parseErr.Error()}}, warnings
		}
		hash := sha256.Sum256(recipeWithContent.Content)
		return recipeDef, hex.EncodeToString(hash[:]), errorsList, warnings
	}

	content := []byte(target.Content)
	if target.Format == "json" && !json.Valid(content) {
		return nil, "", []recipeTestIssue{{Code: "invalid_target", Field: "target_recipe.content", Message: "invalid json content"}}, warnings
	}
	recipeDef, dynamicWarnings, err := loadRecipeWithDynamicOpStubs(content)
	warnings = append(warnings, dynamicWarnings...)
	if err != nil {
		return nil, "", []recipeTestIssue{{Code: "invalid_recipe", Field: "target_recipe.content", Message: err.Error()}}, warnings
	}
	hash := sha256.Sum256(content)
	return recipeDef, hex.EncodeToString(hash[:]), errorsList, warnings
}

func (s *Service) validateDependencyAvailability(req recipeTestCaseRequest) []recipeTestIssue {
	issues := make([]recipeTestIssue, 0)
	if !requestUsesPassthrough(req) {
		return issues
	}
	if s.deps == nil {
		issues = append(issues, recipeTestIssue{Code: "dependency_unavailable", Field: "case.mocks.ops", Message: "passthrough mock mode requires service dependencies"})
		return issues
	}
	required := normalizedExecutionConfig(req).Policy.RequiredDependencies
	for dep := range required {
		if !s.dependencyAvailable(dep) {
			issues = append(issues, recipeTestIssue{Code: "dependency_unavailable", Field: "case.options.policy.required_dependencies", Message: "required dependency unavailable: " + dep})
		}
	}
	return issues
}

func (s *Service) dependencyAvailable(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "database":
		return s.deps != nil && s.deps.Database() != nil
	case "workflow_control":
		return s.deps != nil && s.deps.WorkflowControl() != nil
	case "sse_manager":
		return s.deps != nil && s.deps.SSEManager() != nil
	default:
		return false
	}
}

func requestUsesPassthrough(req recipeTestCaseRequest) bool {
	for _, m := range req.Case.Mocks.Ops {
		switch m.Behavior.Mode {
		case "passthrough", "record_passthrough", "replay":
			return true
		}
	}
	return false
}

func (s *Service) Execute(ctx context.Context, projectID project.ID, req recipeTestCaseRequest, prepared PreparedCase) recipeTestExecuteResponse {
	started := time.Now()
	execResp := recipeTestExecuteResponse{CaseId: req.Case.ID, Status: "passed", CaseHash: prepared.Validation.CaseHash}
	timeout := parseTimeout(req.Execution.Timeout, 60*time.Second)

	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	execCfg := normalizedExecutionConfig(req)
	jobCtx := newRecipeTestJobContext(projectID, req.Case, execCfg.Policy, s.deps)
	wfCtx := workflow.Context{JobContext: jobCtx, ServiceDependencies2: coreops.NewServiceDepsBuilder().Build()}
	rawInputs := req.Case.Inputs
	if rawInputs == nil {
		rawInputs = map[string]interface{}{}
	}
	runCtx := contextual.JobContext{
		Environment: contextual.EnvironmentContext{WorktreePath: "/tmp/recipe-tests/worktree", WorkdirPath: "/tmp/recipe-tests/workdir", ArtifactInbox: "/tmp/recipe-tests/inbox", ArtifactOutbox: "/tmp/recipe-tests/outbox"},
		Workflow:    contextual.WorkflowContext{CellName: "recipe-tests", CellPath: "recipe-tests", ProjectId: string(projectID)},
		GitBase:     contextual.GitBaseContext{BaseRepo: "recipe-tests", BaseRef: prepared.ResolvedHash, ResolvedBaseHash: prepared.ResolvedHash},
	}
	gitCtx := contextual.GitCommitContext{ParentRef: prepared.ResolvedHash}

	outputs, artifacts, err := compiler.ExecuteRecipe(
		wfCtx,
		*prepared.Recipe,
		rawInputs,
		runCtx,
		gitCtx,
		compiler.ExecutionOptions{CELOptionsProvider: s.celProvider},
	)
	if err != nil {
		execResp.Status = "failed"
		execResp.FailureCategory = failureCategoryFromError(err)
		execResp.FailureReason = err.Error()
	} else {
		execResp.Outputs = outputs
	}

	artifactBytes := collectArtifactBytes(tctx, jobCtx, artifacts)
	assertionResults, assertionFailed := runRecipeTestAssertions(req.Case.Assertions, execResp.Outputs, artifactBytes, jobCtx.executedNodes, execResp.Status)
	execResp.Assertions = assertionResults
	if assertionFailed {
		markFailure(&execResp, "assertion_failure", "one or more assertions failed")
	}

	evalResults, evalFailed := runRecipeTestEvaluations(req.Case.Evaluations, execResp.Outputs, artifactBytes, execCfg.EvaluationMode)
	execResp.Evaluations = evalResults
	if evalFailed {
		markFailure(&execResp, "evaluation_failure", "one or more enforced evaluations failed")
	}

	execResp.Diagnostics = recipeTestDiagnostics{MockHits: jobCtx.mockHits, MockMisses: jobCtx.mockMisses}
	if execCfg.ArtifactMode == "inline" {
		execResp.Artifacts = inlineArtifacts(artifactBytes, execCfg.ArtifactMaxBytes)
	}
	if execCfg.Policy.AllowedOnlyNodePath != "" && !jobCtx.executedNodes[execCfg.Policy.AllowedOnlyNodePath] {
		markFailure(&execResp, "runtime_error", "target op_case node was not executed")
	}

	if tctx.Err() == context.DeadlineExceeded {
		execResp.Status = "timed_out"
		execResp.FailureCategory = "timeout"
		execResp.FailureReason = "execution timed out"
	}
	execResp.DurationMs = time.Since(started).Milliseconds()
	return execResp
}

var unknownOpPattern = regexp.MustCompile(`unknown op: \[([^\]]+)\]`)

func loadRecipeWithDynamicOpStubs(content []byte) (*recipecore.Recipe, []recipeTestIssue, error) {
	warnings := make([]recipeTestIssue, 0)
	for i := 0; i < 32; i++ {
		r, err := recipecore.LoadRecipeFromString(content)
		if err == nil {
			return r, warnings, nil
		}
		m := unknownOpPattern.FindStringSubmatch(err.Error())
		if len(m) != 2 {
			return nil, warnings, err
		}
		opName := strings.TrimSpace(m[1])
		if opName == "" {
			return nil, warnings, err
		}
		registerRecipeTestStubOp(opName)
		warnings = append(warnings, recipeTestIssue{Code: "stubbed_unknown_op", Message: fmt.Sprintf("registered dynamic test stub for op %q", opName)})
	}
	return nil, warnings, fmt.Errorf("too many unknown ops while loading recipe")
}

func registerRecipeTestStubOp(opName string) {
	if _, exists := coreops.Get(opName); exists {
		return
	}
	op := coreops.NewActivityMappedOpV2[map[string]interface{}, map[string]interface{}](coreops.OpMetadata{Type: opName},
		func(_ coreops.OpDependencies, _ context.Context, _ map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{}, nil
		})
	coreops.Register(op)
}

func computeCaseHash(recipeHash string, c recipeTestCase) string {
	payload := map[string]interface{}{"recipe_hash": recipeHash, "case": c}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func getMap(root map[string]interface{}, key string) map[string]interface{} {
	if root == nil {
		return nil
	}
	v, ok := root[key]
	if !ok {
		return nil
	}
	m, _ := v.(map[string]interface{})
	return m
}

func stringSlice(v interface{}) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func parseOpCaseTarget(c recipeTestCase) (string, bool) {
	if c.Type != "op_case" || c.Target == nil {
		return "", false
	}
	nodePath, _ := c.Target["node_path"].(string)
	nodePath = strings.TrimSpace(nodePath)
	return nodePath, nodePath != ""
}

func isSupportedDependencyName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "database", "workflow_control", "sse_manager":
		return true
	default:
		return false
	}
}

type recipeTestRuntimeConfig struct {
	Policy           recipeTestPolicy
	ArtifactMode     string
	ArtifactMaxBytes int64
	EvaluationMode   string
}

type recipeTestPolicy struct {
	Mode                 string
	RequireMocks         bool
	BlockedOps           map[string]struct{}
	AllowedOnlyNodePath  string
	RequiredDependencies map[string]struct{}
}

func normalizedExecutionConfig(req recipeTestCaseRequest) recipeTestRuntimeConfig {
	mode := req.Execution.Mode
	if mode == "" {
		mode = "isolated"
	}
	artifactMode := req.Execution.ArtifactMode
	if artifactMode == "" {
		artifactMode = "none"
	}
	artifactMax := req.Execution.ArtifactMaxBytes
	if artifactMax <= 0 {
		artifactMax = 64 * 1024
	}
	evalMode := req.Execution.EvaluationMode
	policy := recipeTestPolicy{
		Mode:                 mode,
		RequireMocks:         mode == "isolated",
		BlockedOps:           map[string]struct{}{},
		RequiredDependencies: map[string]struct{}{},
	}

	if options, ok := req.Case.Options["policy"].(map[string]interface{}); ok {
		if v, ok := options["require_mocks"].(bool); ok {
			policy.RequireMocks = v
		}
		if arr, ok := options["blocked_ops"].([]interface{}); ok {
			for _, item := range arr {
				s, _ := item.(string)
				s = strings.TrimSpace(s)
				if s != "" {
					policy.BlockedOps[s] = struct{}{}
				}
			}
		}
		for _, dep := range stringSlice(options["required_dependencies"]) {
			if dep = strings.TrimSpace(dep); dep != "" {
				policy.RequiredDependencies[dep] = struct{}{}
			}
		}
	}

	if req.Case.Type == "op_case" {
		if targetNode, ok := parseOpCaseTarget(req.Case); ok {
			policy.AllowedOnlyNodePath = targetNode
		}
	}

	return recipeTestRuntimeConfig{Policy: policy, ArtifactMode: artifactMode, ArtifactMaxBytes: artifactMax, EvaluationMode: evalMode}
}

type recipeTestJobContext struct {
	jobKey           swf.JobKey
	caseDef          recipeTestCase
	policy           recipeTestPolicy
	deps             coreops.ServiceDependencies2
	mockHits         []recipeTestMockHit
	mockMisses       []recipeTestMockMiss
	executedNodes    map[string]bool
	artifactContents map[string][]byte
	recordings       map[string]recipePassthroughRecord
}

type recipePassthroughRecord struct {
	Outputs   map[string]interface{}
	NextTask  string
	Artifacts map[string][]byte
}

func newRecipeTestJobContext(projectID project.ID, caseDef recipeTestCase, policy recipeTestPolicy, deps coreops.ServiceDependencies2) *recipeTestJobContext {
	tenantID := strings.TrimSpace(string(projectID))
	if tenantID == "" {
		tenantID = "recipe-tests"
	}
	return &recipeTestJobContext{
		jobKey:           swf.JobKey{TenantId: tenantID, JobId: "recipe-test-job"},
		caseDef:          caseDef,
		policy:           policy,
		deps:             deps,
		executedNodes:    map[string]bool{},
		artifactContents: map[string][]byte{},
		recordings:       map[string]recipePassthroughRecord{},
	}
}

func (j *recipeTestJobContext) AwaitJobs(_ ...string) error        { return nil }
func (j *recipeTestJobContext) GetJobKey() swf.JobKey              { return j.jobKey }
func (j *recipeTestJobContext) Logger() *slog.Logger               { return nil }
func (j *recipeTestJobContext) AwaitDuration(_ swf.Duration) error { return nil }

func (j *recipeTestJobContext) DoTask(_ swf.RunPolicy, taskType string, data swf.TaskData) (swf.TaskData, error) {
	raw, err := data.GetData()
	if err != nil {
		return nil, err
	}
	var inv workerops.ActivityInvocationRequest
	if err := json.Unmarshal(raw, &inv); err != nil {
		return nil, err
	}
	opName := taskType
	if idx := strings.Index(opName, ":"); idx >= 0 {
		opName = opName[:idx]
	}
	nodePath := strings.TrimSpace(inv.GitTaskContext.NodePath)
	j.executedNodes[nodePath] = true

	if j.policy.AllowedOnlyNodePath != "" && nodePath != j.policy.AllowedOnlyNodePath {
		reason := "node outside op_case target scope"
		j.mockMisses = append(j.mockMisses, recipeTestMockMiss{NodePath: nodePath, Op: opName, Reason: reason})
		return nil, fmt.Errorf("%s: %s", opName, reason)
	}

	mock, matched := selectOpMock(j.caseDef.Mocks.Ops, nodePath, opName)
	if !matched {
		reason := "unmocked op"
		if _, blocked := j.policy.BlockedOps[opName]; blocked {
			reason = "blocked by policy"
		} else if j.policy.RequireMocks {
			reason = "policy requires op mock"
		}
		j.mockMisses = append(j.mockMisses, recipeTestMockMiss{NodePath: nodePath, Op: opName, Reason: reason})
		return nil, fmt.Errorf("%s: %s", opName, reason)
	}

	j.mockHits = append(j.mockHits, recipeTestMockHit{NodePath: nodePath, Op: opName, Mode: mock.Behavior.Mode})
	switch mock.Behavior.Mode {
	case "return":
		return j.buildTaskData(mock.Behavior.Outputs, mock.Behavior.Artifacts, "")
	case "fail":
		msg := "mock failure"
		if mock.Behavior.Error != nil && strings.TrimSpace(mock.Behavior.Error.Message) != "" {
			msg = mock.Behavior.Error.Message
		}
		return nil, fmt.Errorf("%s", msg)
	case "passthrough", "record_passthrough":
		record, err := j.runPassthroughTask(taskType, inv)
		if err != nil {
			return nil, err
		}
		if mock.Behavior.Mode == "record_passthrough" {
			key := cassetteKeyForMock(mock, nodePath, opName)
			j.recordings[key] = record
		}
		return j.buildTaskData(record.Outputs, bytesMapToStringMap(record.Artifacts), record.NextTask)
	case "replay":
		key := cassetteKeyForMock(mock, nodePath, opName)
		record, ok := j.recordings[key]
		if !ok {
			return nil, fmt.Errorf("replay cassette not found for key %q", key)
		}
		return j.buildTaskData(record.Outputs, bytesMapToStringMap(record.Artifacts), record.NextTask)
	default:
		return nil, fmt.Errorf("mock mode %q not supported in isolated execution", mock.Behavior.Mode)
	}
}

func (j *recipeTestJobContext) buildTaskData(outputs map[string]interface{}, artifacts map[string]string, nextTask string) (swf.TaskData, error) {
	if outputs == nil {
		outputs = map[string]interface{}{}
	}
	activityOut := workerops.ActivityInvocationOutput{OpOutput: outputs, NextTask: nextTask}
	env, err := coretask.NewOutputEnvelope(coretask.OutputKindActivityInvocationOutput, activityOut)
	if err != nil {
		return nil, err
	}
	artifactList := make([]swf.Artifact, 0, len(artifacts))
	for name, content := range artifacts {
		b := []byte(content)
		j.artifactContents[name] = b
		artifactList = append(artifactList, swf.NewArtifactFromBytes(name, b))
	}
	return swf.NewTaskData(env, artifactList...)
}

func (j *recipeTestJobContext) runPassthroughTask(taskType string, inv workerops.ActivityInvocationRequest) (recipePassthroughRecord, error) {
	opName, stepName := splitTaskType(taskType)
	op, exists := coreops.Get(opName)
	if !exists {
		return recipePassthroughRecord{}, fmt.Errorf("passthrough op not registered: %s", opName)
	}
	chain := op.TaskChain()
	if len(chain) == 0 {
		return recipePassthroughRecord{}, fmt.Errorf("passthrough op has no task steps: %s", opName)
	}

	idx := 0
	if stepName != "" {
		found := false
		for i, st := range chain {
			if st.Name == stepName {
				idx = i
				found = true
				break
			}
		}
		if !found {
			return recipePassthroughRecord{}, fmt.Errorf("passthrough task step not found: %s", taskType)
		}
	}

	inputArtifacts := make([]swf.Artifact, 0, len(inv.ArtifactKeys))
	for _, key := range inv.ArtifactKeys {
		// Recipe tests are stateless; use empty artifacts as presence tokens for passthrough steps.
		inputArtifacts = append(inputArtifacts, swf.NewArtifactFromBytes(key.Name, nil))
	}

	deps := coreops.NewOpDependenciesBuilder().
		WithDatabase(j.deps.Database()).
		WithWorkflowControl(j.deps.WorkflowControl()).
		WithArtifacts(inputArtifacts).
		WithJobTool(j).
		WithWorktreePath("/tmp/recipe-tests/worktree").
		Build()

	out, err := chain[idx].Invoke(deps, context.Background(), inv.Input)
	if err != nil {
		return recipePassthroughRecord{}, fmt.Errorf("passthrough invoke failed for %s: %w", taskType, err)
	}
	nextTask := chain[idx].NextStepTask
	if setter, ok := deps.(interface{ NextTaskType() (string, bool) }); ok {
		if custom, set := setter.NextTaskType(); set {
			nextTask = custom
		}
	}

	artifacts := map[string][]byte{}
	if outDeps, ok := deps.(interface{ GetOutputArtifacts() []swf.Artifact }); ok {
		for _, art := range outDeps.GetOutputArtifacts() {
			if b, err := art.Bytes(context.Background()); err == nil {
				artifacts[art.Name()] = b
				j.artifactContents[art.Name()] = b
			}
		}
	}

	return recipePassthroughRecord{
		Outputs:   out,
		NextTask:  nextTask,
		Artifacts: artifacts,
	}, nil
}

func selectOpMock(mocks []recipeTestOpMock, nodePath string, opName string) (recipeTestOpMock, bool) {
	bestIdx := -1
	bestScore := -1
	for i, m := range mocks {
		nodeMatch := strings.TrimSpace(m.Match.NodePath)
		opMatch := strings.TrimSpace(m.Match.Op)
		score := -1
		switch {
		case nodeMatch != "" && opMatch != "" && nodeMatch == nodePath && opMatch == opName:
			score = 3
		case nodeMatch != "" && opMatch == "" && nodeMatch == nodePath:
			score = 2
		case nodeMatch == "" && opMatch != "" && opMatch == opName:
			score = 1
		}
		if score > bestScore {
			bestIdx = i
			bestScore = score
		}
	}
	if bestIdx < 0 {
		return recipeTestOpMock{}, false
	}
	return mocks[bestIdx], true
}

func splitTaskType(taskType string) (string, string) {
	if idx := strings.Index(taskType, ":"); idx >= 0 {
		return taskType[:idx], taskType[idx+1:]
	}
	return taskType, ""
}

func cassetteKeyForMock(m recipeTestOpMock, nodePath string, opName string) string {
	if strings.TrimSpace(m.Behavior.CassetteKey) != "" {
		return strings.TrimSpace(m.Behavior.CassetteKey)
	}
	return nodePath + "::" + opName
}

func bytesMapToStringMap(in map[string][]byte) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = string(v)
	}
	return out
}

func collectArtifactBytes(ctx context.Context, jc *recipeTestJobContext, artifacts []swf.Artifact) map[string][]byte {
	out := map[string][]byte{}
	for k, v := range jc.artifactContents {
		out[k] = v
	}
	for _, art := range artifacts {
		if b, err := art.Bytes(ctx); err == nil {
			out[art.Name()] = b
		}
	}
	return out
}

func markFailure(resp *recipeTestExecuteResponse, category string, reason string) {
	resp.Status = "failed"
	if resp.FailureCategory == "" {
		resp.FailureCategory = category
		resp.FailureReason = reason
	}
}

func parseTimeout(raw string, fallback time.Duration) time.Duration {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func runRecipeTestAssertions(assertions []recipeTestAssertion, outputs map[string]interface{}, artifacts map[string][]byte, executedNodes map[string]bool, status string) ([]recipeTestAssertionResult, bool) {
	results := make([]recipeTestAssertionResult, 0, len(assertions))
	failed := false
	for _, a := range assertions {
		res := recipeTestAssertionResult{Type: a.Type, Passed: true}
		switch a.Type {
		case "output_equals":
			actual, _ := lookupPath(outputs, a.Path)
			res.Expected, res.Actual = a.Value, actual
			res.Passed = deepEqualJSON(a.Value, actual)
			if !res.Passed {
				res.Message = "output value mismatch"
			}
		case "output_matches":
			actual, _ := lookupPath(outputs, a.Path)
			actualStr := fmt.Sprintf("%v", actual)
			res.Expected, res.Actual = a.Regex, actualStr
			res.Passed = regexMatch(a.Regex, actualStr)
			if !res.Passed {
				res.Message = "output did not match regex"
			}
		case "artifact_exists":
			_, res.Passed = artifacts[a.Path]
			if !res.Passed {
				res.Message = "artifact missing"
			}
		case "artifact_json_equals":
			b, ok := artifacts[a.Path]
			if !ok {
				res.Passed, res.Message = false, "artifact missing"
				break
			}
			var parsed interface{}
			if err := json.Unmarshal(b, &parsed); err != nil {
				res.Passed, res.Message = false, "artifact is not valid json"
				break
			}
			actual, _ := lookupPath(parsed, a.JsonPath)
			res.Expected, res.Actual = a.Value, actual
			res.Passed = deepEqualJSON(a.Value, actual)
			if !res.Passed {
				res.Message = "artifact json value mismatch"
			}
		case "node_executed":
			res.Passed = executedNodes[a.NodePath]
			if !res.Passed {
				res.Message = "expected node to execute"
			}
		case "node_not_executed":
			res.Passed = !executedNodes[a.NodePath]
			if !res.Passed {
				res.Message = "expected node not to execute"
			}
		case "status_is":
			res.Expected, res.Actual = a.Status, status
			res.Passed = a.Status == status
			if !res.Passed {
				res.Message = "status mismatch"
			}
		case "cel_true":
			expr := strings.TrimSpace(a.Expr)
			res.Passed = expr == "" || strings.EqualFold(expr, "true")
			if !res.Passed {
				res.Message = "cel_true currently supports only expression true"
			}
		}
		if !res.Passed {
			failed = true
		}
		results = append(results, res)
	}
	return results, failed
}

func runRecipeTestEvaluations(evals []recipeTestEvaluation, outputs map[string]interface{}, artifacts map[string][]byte, evalModeOverride string) ([]recipeTestEvaluationResult, bool) {
	results := make([]recipeTestEvaluationResult, 0, len(evals))
	enforcedFailed := false
	for _, e := range evals {
		mode := e.Mode
		if mode == "" {
			mode = "enforce"
		}
		if evalModeOverride == "report_only" {
			mode = "report_only"
		}
		res := recipeTestEvaluationResult{Id: e.ID, Type: e.Type, Mode: mode, Passed: true}
		sourceText, srcErr := resolveEvaluationSource(e.Source, outputs, artifacts)
		if srcErr != nil {
			res.Passed = false
			res.ErrorCategory = "evaluator_error"
			res.ErrorMessage = srcErr.Error()
			if mode == "enforce" {
				enforcedFailed = true
			}
			results = append(results, res)
			continue
		}
		switch e.Type {
		case "text_pattern":
			res.Passed, res.Output = evaluateTextPattern(sourceText, e.Config)
		case "llm_judge":
			res.Passed, res.Output = evaluateLLMJudge(sourceText, e.Config)
		default:
			res.Passed = false
			res.ErrorCategory = "evaluator_error"
			res.ErrorMessage = "unsupported evaluation type"
		}
		if !res.Passed && mode == "enforce" {
			enforcedFailed = true
		}
		results = append(results, res)
	}
	return results, enforcedFailed
}

func resolveEvaluationSource(source recipeTestEvalSource, outputs map[string]interface{}, artifacts map[string][]byte) (string, error) {
	switch source.Kind {
	case "artifact":
		b, ok := artifacts[source.Path]
		if !ok {
			return "", fmt.Errorf("artifact source not found: %s", source.Path)
		}
		return string(b), nil
	case "artifact_glob":
		names := make([]string, 0, len(artifacts))
		for n := range artifacts {
			names = append(names, n)
		}
		sort.Strings(names)
		matches := make([]string, 0)
		for _, n := range names {
			if ok, _ := path.Match(source.Glob, n); ok {
				matches = append(matches, string(artifacts[n]))
			}
		}
		if len(matches) == 0 {
			return "", fmt.Errorf("artifact glob matched nothing: %s", source.Glob)
		}
		return strings.Join(matches, "\n"), nil
	case "output_path":
		v, ok := lookupPath(outputs, source.OutputPath)
		if !ok {
			return "", fmt.Errorf("output path not found: %s", source.OutputPath)
		}
		return fmt.Sprintf("%v", v), nil
	default:
		return "", fmt.Errorf("source kind %s is not available in this runtime", source.Kind)
	}
}

func evaluateTextPattern(source string, cfg map[string]interface{}) (bool, map[string]interface{}) {
	passed := true
	out := map[string]interface{}{"require_matches": []string{}, "forbid_matches": []string{}}
	if cfg == nil {
		return true, out
	}
	if require, ok := stringSliceFromAny(cfg["require_regex"]); ok {
		for _, r := range require {
			if regexMatch(r, source) {
				out["require_matches"] = append(out["require_matches"].([]string), r)
			} else {
				passed = false
			}
		}
	}
	if forbid, ok := stringSliceFromAny(cfg["forbid_regex"]); ok {
		for _, r := range forbid {
			if regexMatch(r, source) {
				out["forbid_matches"] = append(out["forbid_matches"].([]string), r)
				passed = false
			}
		}
	}
	out["length"] = len(source)
	return passed, out
}

func evaluateLLMJudge(source string, cfg map[string]interface{}) (bool, map[string]interface{}) {
	verdict, score := "pass", 1.0
	findings := []string{}
	if strings.Contains(strings.ToLower(source), "fail") {
		verdict, score = "fail", 0
		findings = append(findings, "content contained token 'fail'")
	}
	out := map[string]interface{}{"verdict": verdict, "score": score, "findings": findings}
	passWhen := ""
	if cfg != nil {
		if v, ok := cfg["pass_when"].(string); ok {
			passWhen = strings.TrimSpace(v)
		}
	}
	if passWhen == "" {
		return verdict == "pass", out
	}
	return evalPassWhen(passWhen, verdict, score), out
}

func evalPassWhen(expr string, verdict string, score float64) bool {
	e := strings.ReplaceAll(expr, " ", "")
	if strings.Contains(e, "&&") {
		for _, p := range strings.Split(e, "&&") {
			if !evalPassWhen(p, verdict, score) {
				return false
			}
		}
		return true
	}
	switch {
	case e == "verdict=='pass'" || e == `verdict=="pass"`:
		return verdict == "pass"
	case strings.HasPrefix(e, "score>="):
		v, _ := strconv.ParseFloat(strings.TrimPrefix(e, "score>="), 64)
		return score >= v
	case strings.HasPrefix(e, "score>"):
		v, _ := strconv.ParseFloat(strings.TrimPrefix(e, "score>"), 64)
		return score > v
	case strings.HasPrefix(e, "score<="):
		v, _ := strconv.ParseFloat(strings.TrimPrefix(e, "score<="), 64)
		return score <= v
	case strings.HasPrefix(e, "score<"):
		v, _ := strconv.ParseFloat(strings.TrimPrefix(e, "score<"), 64)
		return score < v
	case strings.HasPrefix(e, "score=="):
		v, _ := strconv.ParseFloat(strings.TrimPrefix(e, "score=="), 64)
		return score == v
	default:
		return false
	}
}

func inlineArtifacts(artifacts map[string][]byte, maxBytes int64) map[string]recipeInlineArtifact {
	out := make(map[string]recipeInlineArtifact, len(artifacts))
	names := make([]string, 0, len(artifacts))
	for n := range artifacts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		b := artifacts[n]
		r := recipeInlineArtifact{SizeBytes: int64(len(b))}
		if int64(len(b)) > maxBytes {
			r.Truncated = true
			b = b[:maxBytes]
		}
		r.ContentBase64 = base64.StdEncoding.EncodeToString(b)
		out[n] = r
	}
	return out
}

func lookupPath(root interface{}, p string) (interface{}, bool) {
	if strings.TrimSpace(p) == "" {
		return root, true
	}
	cur := root
	for _, part := range strings.Split(p, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		next, ok := m[part]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func regexMatch(pattern string, value string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(value)
}

func deepEqualJSON(a interface{}, b interface{}) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
	return string(ab) == string(bb)
}

func stringSliceFromAny(v interface{}) ([]string, bool) {
	switch t := v.(type) {
	case []string:
		return t, true
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, it := range t {
			if s, ok := it.(string); ok {
				out = append(out, s)
			}
		}
		return out, true
	default:
		return nil, false
	}
}

func failureCategoryFromError(err error) string {
	if err == nil {
		return ""
	}
	e := strings.ToLower(err.Error())
	switch {
	case strings.Contains(e, "policy") || strings.Contains(e, "blocked") || strings.Contains(e, "target scope") || strings.Contains(e, "requires op mock"):
		return "policy_blocked"
	case strings.Contains(e, "timeout"):
		return "timeout"
	default:
		return "runtime_error"
	}
}
