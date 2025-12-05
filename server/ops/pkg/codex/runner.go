package codex

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/colony-2/shai/pkg/shai"
)

type runnerAdapter struct {
	inner shai.Sandbox
}

func (r runnerAdapter) Run(ctx context.Context) error {
	return r.inner.Run(ctx)
}

func (r runnerAdapter) Close() error { return r.inner.Close() }

func defaultRunnerFactory(cfg *shai.SandboxConfig) (Runner, error) {
	runner, err := shai.NewSandbox(*cfg)
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
	if _, err := c.stdout.Write(data); err != nil {
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

// stdoutWriter adapts OnStdout to an io.Writer for shai's non-TTY mode.
func (c *outputCollector) stdoutWriter() io.Writer {
	return writerAdapter(func(p []byte) { c.OnStdout(p) })
}

// stderrWriter adapts OnStderr to an io.Writer for shai's non-TTY mode.
func (c *outputCollector) stderrWriter() io.Writer {
	return writerAdapter(func(p []byte) { c.OnStderr(p) })
}

type writerAdapter func([]byte)

func (w writerAdapter) Write(p []byte) (int, error) {
	if w == nil {
		return len(p), nil
	}
	w(p)
	return len(p), nil
}
