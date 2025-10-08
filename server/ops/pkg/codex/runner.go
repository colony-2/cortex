package codex

import (
	"context"
	"fmt"
	"io"
	"strings"

	shai "github.com/divisive-ai/vibethis/server/container/pkg/shai"
)

type runnerAdapter struct {
	inner *shai.EphemeralRunner
}

func (r runnerAdapter) Run(ctx context.Context) error {
	return r.inner.Run(ctx)
}

func (r runnerAdapter) Close() error { return r.inner.Close() }

func defaultRunnerFactory(cfg *shai.EphemeralConfig) (Runner, error) {
	runner, err := shai.NewEphemeralRunner(*cfg)
	if err != nil {
		return nil, err
	}
	return runnerAdapter{inner: runner}, nil
}

type outputCollector struct {
	stdout io.Writer
	stderr *strings.Builder
}

func newOutputCollector(stdout io.Writer) *outputCollector {
	return &outputCollector{stdout: stdout, stderr: &strings.Builder{}}
}

func (c *outputCollector) OnStdout(data []byte) {
	if len(data) == 0 {
		return
	}
	if _, err := c.stdout.Write(append(data, '\n')); err != nil {
		// best-effort: capture error in stderr buffer so callers can inspect
		c.stderr.WriteString(fmt.Sprintf("write stdout: %v\n", err))
	}
}

func (c *outputCollector) OnStderr(data []byte) {
	if len(data) == 0 {
		return
	}
	if c.stderr.Len() > 0 {
		c.stderr.WriteByte('\n')
	}
	c.stderr.WriteString(string(data))
}

func (c *outputCollector) stderrString() string {
	return c.stderr.String()
}

var _ shai.OutputSink = (*outputCollector)(nil)
