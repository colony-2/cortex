package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type recipeTestSuite struct {
	Cases []map[string]interface{} `json:"cases" yaml:"cases"`
}

type compiledRecipeTestIR struct {
	TargetRecipe map[string]interface{}   `json:"target_recipe" yaml:"target_recipe"`
	Cases        []map[string]interface{} `json:"cases" yaml:"cases"`
}

type recipeTestRunResult struct {
	CaseID   string                 `json:"case_id"`
	Status   string                 `json:"status"`
	Duration int64                  `json:"duration_ms"`
	Response map[string]interface{} `json:"response"`
	Err      string                 `json:"error,omitempty"`
}

func newRecipeTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test", Short: "Compile, validate, and run recipe test suites"}
	cmd.AddCommand(newRecipeTestCompileCmd(), newRecipeTestValidateCmd(), newRecipeTestRunCmd(), newRecipeTestCaseCmd())
	return cmd
}

func newRecipeTestCaseCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "case", Short: "Validate or run a single case"}
	cmd.AddCommand(newRecipeTestCaseValidateCmd(), newRecipeTestCaseRunCmd())
	return cmd
}

func newRecipeTestCompileCmd() *cobra.Command {
	var flags recipeTestCommonFlags
	var outPath string
	cmd := &cobra.Command{
		Use:   "compile",
		Short: "Compile test suite into canonical IR",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			ir, err := compileRecipeTestSuite(flags)
			if err != nil {
				return err
			}
			if outPath == "" {
				return fmt.Errorf("--out is required")
			}
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				return err
			}
			b, _ := json.MarshalIndent(ir, "", "  ")
			if err := os.WriteFile(outPath, b, 0o644); err != nil {
				return err
			}
			if app.Config.Output == "json" {
				return app.Printer.JSON(ir)
			}
			return app.Printer.Text(fmt.Sprintf("compiled %d case(s) to %s", len(ir.Cases), outPath))
		},
	}
	bindRecipeTestCommonFlags(cmd, &flags)
	cmd.Flags().StringVar(&outPath, "out", "", "Output path for compiled canonical IR")
	return cmd
}

func newRecipeTestValidateCmd() *cobra.Command {
	var flags recipeTestCommonFlags
	var parallelism int
	var failFast bool
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate suite via server per case",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}
			ir, err := compileRecipeTestSuite(flags)
			if err != nil {
				return err
			}
			results := runRecipeTestCases(cmd.Context(), app, ir, parallelism, failFast, false, recipeTestExecutionOpts{})
			invalid := countByStatus(results, "invalid") + countByStatus(results, "error")
			for _, r := range results {
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s (%dms)\n", r.CaseID, r.Status, r.Duration)
			}
			summary := map[string]interface{}{"cases": len(results), "invalid_or_error": invalid}
			if app.Config.Output == "json" {
				return app.Printer.JSON(summary)
			}
			if invalid > 0 {
				return fmt.Errorf("%d case(s) invalid or errored", invalid)
			}
			return nil
		},
	}
	bindRecipeTestCommonFlags(cmd, &flags)
	cmd.Flags().IntVar(&parallelism, "parallelism", 4, "Number of cases to process in parallel")
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "Stop scheduling new cases after first invalid case")
	return cmd
}

func newRecipeTestRunCmd() *cobra.Command {
	var flags recipeTestCommonFlags
	var parallelism int
	var stopOnFailure bool
	var outDir string
	var jsonlPath string
	var execOpts recipeTestExecutionOpts
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute suite via server per case",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}
			ir, err := compileRecipeTestSuite(flags)
			if err != nil {
				return err
			}
			if outDir == "" {
				outDir = filepath.Join(".c2", "test-results", time.Now().UTC().Format("20060102T150405Z"))
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}

			results := runRecipeTestCases(cmd.Context(), app, ir, parallelism, stopOnFailure, true, execOpts)
			for _, r := range results {
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s (%dms)\n", r.CaseID, r.Status, r.Duration)
			}
			if err := writeRecipeRunArtifacts(outDir, results); err != nil {
				return err
			}
			if strings.TrimSpace(jsonlPath) != "" {
				if err := writeRecipeJSONLEvents(jsonlPath, results); err != nil {
					return err
				}
			}
			failed := countByStatus(results, "failed") + countByStatus(results, "error")
			if failed > 0 {
				return fmt.Errorf("%d case(s) failed", failed)
			}
			return nil
		},
	}
	bindRecipeTestCommonFlags(cmd, &flags)
	cmd.Flags().IntVar(&parallelism, "parallelism", 4, "Number of cases to process in parallel")
	cmd.Flags().BoolVar(&stopOnFailure, "stop-on-failure", false, "Stop scheduling new cases after first failure")
	cmd.Flags().StringVar(&execOpts.Timeout, "case-timeout", "", "Per-case timeout duration")
	cmd.Flags().StringVar(&execOpts.ArtifactMode, "artifact-mode", "none", "Artifact mode: none|inline")
	cmd.Flags().Int64Var(&execOpts.ArtifactMaxBytes, "artifact-max-bytes", 65536, "Max inline artifact bytes per artifact")
	cmd.Flags().StringVar(&outDir, "out-dir", "", "Output directory for local test artifacts")
	cmd.Flags().StringVar(&jsonlPath, "jsonl-events", "", "Optional JSONL event output path")
	cmd.Flags().StringVar(&execOpts.EvaluationMode, "evaluation-mode", "enforce", "Evaluation mode: enforce|report_only")
	_ = cmd.Flags().MarkHidden("judge-timeout")
	_ = cmd.Flags().MarkHidden("judge-max-tokens")
	cmd.Flags().String("judge-timeout", "", "unused")
	cmd.Flags().Int64("judge-max-tokens", 0, "unused")
	return cmd
}

func newRecipeTestCaseValidateCmd() *cobra.Command {
	var flags recipeTestCommonFlags
	var caseID string
	var parallelism int
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate one case ID from suite",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}
			if strings.TrimSpace(caseID) == "" {
				return fmt.Errorf("--case-id is required")
			}
			flags.CaseIDs = []string{caseID}
			ir, err := compileRecipeTestSuite(flags)
			if err != nil {
				return err
			}
			results := runRecipeTestCases(cmd.Context(), app, ir, parallelism, false, false, recipeTestExecutionOpts{})
			invalid := countByStatus(results, "invalid") + countByStatus(results, "error")
			for _, r := range results {
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s (%dms)\n", r.CaseID, r.Status, r.Duration)
			}
			if invalid > 0 {
				return fmt.Errorf("%d case(s) invalid or errored", invalid)
			}
			return nil
		},
	}
	bindRecipeTestCommonFlags(cmd, &flags)
	cmd.Flags().StringVar(&caseID, "case-id", "", "Case ID to validate")
	cmd.Flags().IntVar(&parallelism, "parallelism", 1, "Number of cases to process in parallel")
	return cmd
}

func newRecipeTestCaseRunCmd() *cobra.Command {
	var flags recipeTestCommonFlags
	var caseID string
	var outDir string
	var execOpts recipeTestExecutionOpts
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one case ID from suite",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := fetchApp(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireProject(app.Config.Project); err != nil {
				return err
			}
			if strings.TrimSpace(caseID) == "" {
				return fmt.Errorf("--case-id is required")
			}
			flags.CaseIDs = []string{caseID}
			ir, err := compileRecipeTestSuite(flags)
			if err != nil {
				return err
			}
			if outDir == "" {
				outDir = filepath.Join(".c2", "test-results", time.Now().UTC().Format("20060102T150405Z"))
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}
			results := runRecipeTestCases(cmd.Context(), app, ir, 1, false, true, execOpts)
			for _, r := range results {
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s (%dms)\n", r.CaseID, r.Status, r.Duration)
			}
			if err := writeRecipeRunArtifacts(outDir, results); err != nil {
				return err
			}
			failed := countByStatus(results, "failed") + countByStatus(results, "error")
			if failed > 0 {
				return fmt.Errorf("%d case(s) failed", failed)
			}
			return nil
		},
	}
	bindRecipeTestCommonFlags(cmd, &flags)
	cmd.Flags().StringVar(&caseID, "case-id", "", "Case ID to run")
	cmd.Flags().StringVar(&execOpts.Timeout, "case-timeout", "", "Per-case timeout duration")
	cmd.Flags().StringVar(&execOpts.ArtifactMode, "artifact-mode", "none", "Artifact mode: none|inline")
	cmd.Flags().Int64Var(&execOpts.ArtifactMaxBytes, "artifact-max-bytes", 65536, "Max inline artifact bytes per artifact")
	cmd.Flags().StringVar(&execOpts.EvaluationMode, "evaluation-mode", "enforce", "Evaluation mode: enforce|report_only")
	cmd.Flags().StringVar(&outDir, "out-dir", "", "Output directory for local test artifacts")
	return cmd
}

type recipeTestCommonFlags struct {
	RecipeName string
	Version    string
	Ref        string
	RecipeFile string

	FilePath string
	Stdin    bool
	Format   string
	CaseIDs  []string
	Strict   bool
}

type recipeTestExecutionOpts struct {
	Timeout          string
	ArtifactMode     string
	ArtifactMaxBytes int64
	EvaluationMode   string
}

func bindRecipeTestCommonFlags(cmd *cobra.Command, flags *recipeTestCommonFlags) {
	cmd.Flags().StringVar(&flags.RecipeName, "recipe", "", "Server recipe name")
	cmd.Flags().StringVar(&flags.Version, "version", "", "Recipe version")
	cmd.Flags().StringVar(&flags.Ref, "ref", "", "Recipe ref")
	cmd.Flags().StringVar(&flags.RecipeFile, "recipe-file", "", "Inline recipe file path")
	cmd.Flags().StringVar(&flags.FilePath, "file", "", "Suite file path")
	cmd.Flags().BoolVar(&flags.Stdin, "stdin", false, "Read suite from stdin")
	cmd.Flags().StringVar(&flags.Format, "format", "", "Suite format")
	cmd.Flags().StringSliceVar(&flags.CaseIDs, "case", nil, "Only include selected case IDs (repeatable)")
	cmd.Flags().BoolVar(&flags.Strict, "strict", false, "Strict local compile")
}

func compileRecipeTestSuite(flags recipeTestCommonFlags) (compiledRecipeTestIR, error) {
	target, err := buildRecipeTestTarget(flags)
	if err != nil {
		return compiledRecipeTestIR{}, err
	}
	raw, format, err := loadSuiteBytes(flags)
	if err != nil {
		return compiledRecipeTestIR{}, err
	}
	suite, err := parseRecipeTestSuite(raw, format)
	if err != nil {
		return compiledRecipeTestIR{}, err
	}
	cases := filterCasesByIDs(suite.Cases, flags.CaseIDs)
	if len(cases) == 0 {
		return compiledRecipeTestIR{}, fmt.Errorf("no cases selected")
	}
	return compiledRecipeTestIR{TargetRecipe: target, Cases: cases}, nil
}

func buildRecipeTestTarget(flags recipeTestCommonFlags) (map[string]interface{}, error) {
	if strings.TrimSpace(flags.RecipeName) != "" && strings.TrimSpace(flags.RecipeFile) != "" {
		return nil, fmt.Errorf("--recipe and --recipe-file are mutually exclusive")
	}
	if strings.TrimSpace(flags.RecipeName) == "" && strings.TrimSpace(flags.RecipeFile) == "" {
		return nil, fmt.Errorf("one of --recipe or --recipe-file is required")
	}
	if strings.TrimSpace(flags.RecipeFile) != "" {
		if strings.TrimSpace(flags.Version) != "" || strings.TrimSpace(flags.Ref) != "" {
			return nil, fmt.Errorf("--version/--ref cannot be used with --recipe-file")
		}
		b, err := readData(flags.RecipeFile)
		if err != nil {
			return nil, err
		}
		format := inferFormat(flags.RecipeFile, "")
		if format == "canonical_json" {
			format = "json"
		} else {
			format = "yaml"
		}
		return map[string]interface{}{"mode": "inline_recipe", "format": format, "content": string(b)}, nil
	}
	hasVersion := strings.TrimSpace(flags.Version) != ""
	hasRef := strings.TrimSpace(flags.Ref) != ""
	if hasVersion == hasRef {
		return nil, fmt.Errorf("exactly one of --version or --ref is required with --recipe")
	}
	target := map[string]interface{}{"mode": "server_ref", "name": flags.RecipeName}
	if hasVersion {
		target["version"] = flags.Version
	} else {
		target["ref"] = flags.Ref
	}
	return target, nil
}

func loadSuiteBytes(flags recipeTestCommonFlags) ([]byte, string, error) {
	if flags.Stdin {
		b, err := io.ReadAll(bufio.NewReader(os.Stdin))
		if err != nil {
			return nil, "", err
		}
		return b, resolveSuiteFormat(flags.Format, ""), nil
	}
	if strings.TrimSpace(flags.FilePath) == "" {
		return nil, "", fmt.Errorf("one of --file or --stdin is required")
	}
	b, err := readData(flags.FilePath)
	if err != nil {
		return nil, "", err
	}
	return b, resolveSuiteFormat(flags.Format, flags.FilePath), nil
}

func resolveSuiteFormat(explicit string, filePath string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	return inferFormat(filePath, "canonical_yaml")
}

func inferFormat(filePath string, def string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".json":
		return "canonical_json"
	case ".md", ".markdown":
		return "scenario_md"
	case ".yaml", ".yml":
		return "canonical_yaml"
	default:
		if def != "" {
			return def
		}
		return "canonical_yaml"
	}
}

func parseRecipeTestSuite(raw []byte, format string) (recipeTestSuite, error) {
	var suite recipeTestSuite
	switch format {
	case "canonical_json":
		if err := json.Unmarshal(raw, &suite); err != nil {
			return suite, err
		}
	case "canonical_yaml", "compact_yaml":
		if err := yaml.Unmarshal(raw, &suite); err != nil {
			return suite, err
		}
	case "scenario_md":
		block := extractFencedBlock(string(raw))
		if block == "" {
			return suite, fmt.Errorf("scenario markdown must include a fenced yaml/json block")
		}
		if strings.HasPrefix(strings.TrimSpace(block), "{") {
			if err := json.Unmarshal([]byte(block), &suite); err != nil {
				return suite, err
			}
		} else {
			if err := yaml.Unmarshal([]byte(block), &suite); err != nil {
				return suite, err
			}
		}
	default:
		return suite, fmt.Errorf("unsupported format %q", format)
	}
	return suite, nil
}

func extractFencedBlock(md string) string {
	lines := strings.Split(md, "\n")
	inBlock := false
	var out []string
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			if inBlock {
				break
			}
			inBlock = true
			continue
		}
		if inBlock {
			out = append(out, l)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func filterCasesByIDs(cases []map[string]interface{}, ids []string) []map[string]interface{} {
	if len(ids) == 0 {
		return cases
	}
	want := map[string]struct{}{}
	for _, id := range ids {
		want[id] = struct{}{}
	}
	out := make([]map[string]interface{}, 0, len(cases))
	for _, c := range cases {
		id, _ := c["id"].(string)
		if _, ok := want[id]; ok {
			out = append(out, c)
		}
	}
	return out
}

func runRecipeTestCases(ctx context.Context, app *App, ir compiledRecipeTestIR, parallelism int, stopOnFailure bool, execute bool, execOpts recipeTestExecutionOpts) []recipeTestRunResult {
	if parallelism <= 0 {
		parallelism = 1
	}
	type workItem struct {
		idx     int
		caseObj map[string]interface{}
	}
	jobs := make(chan workItem)
	results := make(chan recipeTestRunResult, len(ir.Cases))
	var stop int32

	var wg sync.WaitGroup
	for i := 0; i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				if atomic.LoadInt32(&stop) == 1 {
					continue
				}
				start := time.Now()
				caseID, _ := item.caseObj["id"].(string)
				resp, err := callRecipeTestEndpoint(ctx, app, ir.TargetRecipe, item.caseObj, execute, execOpts)
				result := recipeTestRunResult{CaseID: caseID, Duration: time.Since(start).Milliseconds(), Response: map[string]interface{}{}}
				if err != nil {
					result.Status = "error"
					result.Err = err.Error()
				} else {
					result.Response = resp
					if execute {
						if s, ok := resp["status"].(string); ok {
							result.Status = s
						} else {
							result.Status = "error"
						}
					} else {
						if valid, ok := resp["valid"].(bool); ok && valid {
							result.Status = "valid"
						} else {
							result.Status = "invalid"
						}
					}
				}
				if stopOnFailure && (result.Status == "failed" || result.Status == "error" || result.Status == "invalid") {
					atomic.StoreInt32(&stop, 1)
				}
				results <- result
			}
		}()
	}

	go func() {
		for i, c := range ir.Cases {
			if atomic.LoadInt32(&stop) == 1 {
				break
			}
			jobs <- workItem{idx: i, caseObj: c}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	out := make([]recipeTestRunResult, 0, len(ir.Cases))
	for r := range results {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CaseID < out[j].CaseID })
	return out
}

func callRecipeTestEndpoint(ctx context.Context, app *App, target map[string]interface{}, caseObj map[string]interface{}, execute bool, execOpts recipeTestExecutionOpts) (map[string]interface{}, error) {
	endpoint := "/api/projects/" + app.Config.Project + "/recipe-tests/cases/validate"
	payload := map[string]interface{}{"target_recipe": target, "case": caseObj}
	if execute {
		endpoint = "/api/projects/" + app.Config.Project + "/recipe-tests/cases/execute"
		payload["execution"] = map[string]interface{}{
			"timeout":            execOpts.Timeout,
			"artifact_mode":      execOpts.ArtifactMode,
			"artifact_max_bytes": execOpts.ArtifactMaxBytes,
			"evaluation_mode":    execOpts.EvaluationMode,
		}
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(app.Config.APIURL, "/")+endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(app.Config.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+app.Config.Token)
	}
	httpClient := &http.Client{Timeout: app.Config.Timeout}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out map[string]interface{}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &out)
	}
	if out == nil {
		out = map[string]interface{}{}
	}
	if resp.StatusCode >= 400 {
		return out, fmt.Errorf("http %d", resp.StatusCode)
	}
	return out, nil
}

func writeRecipeRunArtifacts(outDir string, results []recipeTestRunResult) error {
	summaryPath := filepath.Join(outDir, "summary.json")
	summary := map[string]interface{}{"cases": len(results), "results": results}
	b, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(summaryPath, b, 0o644); err != nil {
		return err
	}

	md := &strings.Builder{}
	md.WriteString("# Recipe Test Summary\n\n")
	for _, r := range results {
		md.WriteString(fmt.Sprintf("- %s: %s (%dms)\n", r.CaseID, r.Status, r.Duration))
	}
	if err := os.WriteFile(filepath.Join(outDir, "summary.md"), []byte(md.String()), 0o644); err != nil {
		return err
	}

	for _, r := range results {
		caseDir := filepath.Join(outDir, "cases", sanitizeCaseID(r.CaseID))
		if err := os.MkdirAll(caseDir, 0o755); err != nil {
			return err
		}
		caseJSON, _ := json.MarshalIndent(r, "", "  ")
		if err := os.WriteFile(filepath.Join(caseDir, "result.json"), caseJSON, 0o644); err != nil {
			return err
		}
		if evals, ok := r.Response["evaluations"]; ok {
			eb, _ := json.MarshalIndent(evals, "", "  ")
			if err := os.WriteFile(filepath.Join(caseDir, "evaluations.json"), eb, 0o644); err != nil {
				return err
			}
		}
		if arts, ok := r.Response["artifacts"].(map[string]interface{}); ok {
			artDir := filepath.Join(caseDir, "artifacts")
			if err := os.MkdirAll(artDir, 0o755); err != nil {
				return err
			}
			for name, v := range arts {
				entry, ok := v.(map[string]interface{})
				if !ok {
					continue
				}
				enc, _ := entry["content_base64"].(string)
				if enc == "" {
					continue
				}
				decoded, err := base64.StdEncoding.DecodeString(enc)
				if err != nil {
					continue
				}
				full := filepath.Join(artDir, name)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(full, decoded, 0o644); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func writeRecipeJSONLEvents(path string, results []recipeTestRunResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range results {
		evt := map[string]interface{}{"event": "case_completed", "case_id": r.CaseID, "status": r.Status, "duration_ms": r.Duration}
		if err := enc.Encode(evt); err != nil {
			return err
		}
	}
	summary := map[string]interface{}{"event": "summary", "cases": len(results)}
	return enc.Encode(summary)
}

func sanitizeCaseID(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unknown"
	}
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	return s
}

func countByStatus(results []recipeTestRunResult, status string) int {
	n := 0
	for _, r := range results {
		if r.Status == status {
			n++
		}
	}
	return n
}
