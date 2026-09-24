package cortexcli

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colony-2/colony2/server/internal/cortex"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/toy"
	"github.com/spf13/cobra"
)

func cleanEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"C2J_JOBDB", "JOBDB_URL", "CORTEX_TENANT_ID", "CORTEX_ADDR", "CORTEX_WORKING_DIR", "CORTEX_CORS_ORIGINS"} {
		t.Setenv(name, "")
	}
}

func configDir(t *testing.T, config string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".c2j"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".c2j", "config.yaml"), []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestConnectionPrecedenceAndCobraCommands(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		env, want string
	}{
		{"root flag", []string{"--jobdb", "http://localhost:9047/flag"}, "http://localhost:9047/env", "flag"},
		{"serve flag", []string{"serve", "--jobdb=http://localhost:9047/flag"}, "", "flag"},
		{"inherited flag", []string{"--jobdb", "http://localhost:9047/flag", "serve"}, "", "flag"},
		{"environment", []string{"serve"}, "http://localhost:9047/env", "env"},
		{"project config", nil, "", "config"},
		{"legacy URI", []string{"--jobdb-url", "http://localhost:9047/c2"}, "", "c2"},
		{"legacy root", []string{"--jobdb-url", "http://localhost:9047", "--tenant-id", "old"}, "", "old"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanEnv(t)
			root := configDir(t, "jobdb: http://localhost:9047/config\n")
			t.Setenv("C2J_JOBDB", tc.env)
			called := false
			cmd := newCommand(Options{}, func(_ *cobra.Command, cfg cortex.Config) error {
				called = true
				if cfg.JobDBURL != "http://localhost:9047" || cfg.DefaultTenantID != tc.want {
					t.Fatalf("resolved config: %+v", cfg)
				}
				return nil
			})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(append(tc.args, "--working-dir", root))
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("server was not started")
			}
		})
	}
}

func TestUsesC2JConfigDiscoveryAndCommandResolution(t *testing.T) {
	cleanEnv(t)
	root := configDir(t, "jobdb:\n  command: cat connection.txt\n")
	if err := os.WriteFile(filepath.Join(root, "connection.txt"), []byte("http://localhost:9047/from-command\n"), 0644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested", "directory")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	cfg, err := resolveConnection(context.Background(), cortex.Config{WorkingDir: nested}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultTenantID != "from-command" {
		t.Fatalf("tenant = %q", cfg.DefaultTenantID)
	}
}

func TestOverridesDoNotEvaluateConfigCommands(t *testing.T) {
	cleanEnv(t)
	root := configDir(t, "jobdb:\n  command: exit 99\n")
	for _, explicit := range []string{"http://localhost:9047/flag", ""} {
		t.Setenv("C2J_JOBDB", "http://localhost:9047/env")
		if _, err := resolveConnection(context.Background(), cortex.Config{WorkingDir: root}, explicit, ""); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("C2J_JOBDB", "")
	if _, err := resolveConnection(context.Background(), cortex.Config{WorkingDir: root}, "", ""); err == nil {
		t.Fatal("config command failure was ignored")
	}
}

func TestInvalidTargetsAndFlagsFailBeforeServing(t *testing.T) {
	for _, args := range [][]string{
		{"--jobdb", "http://localhost:9047"}, {"--jobdb", "http://localhost:9047/a/b"},
		{"--jobdb", "http://localhost:9047/a%2Fb"}, {"--jobdb", "http://user:secret@localhost/c2"},
		{"--jobdb", "http://localhost/c2?token=secret"}, {"--jobdb", "http://localhost/c2#fragment"},
		{"--jobdb", "embed:///"}, {"--jobdb", "ftp://localhost/c2"},
		{"-jobdb", "http://localhost/c2"}, {"serve", "unexpected"}, {"unknown"},
		{"--jobdb", "http://localhost/c2", "--jobdb-url", "http://localhost"},
		{"--jobdb", "http://localhost/c2", "--tenant-id", "other"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cleanEnv(t)
			cmd := newCommand(Options{}, func(_ *cobra.Command, _ cortex.Config) error { t.Fatal("started with invalid flags"); return nil })
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestHelpAndVersionDoNotRequireConfig(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"serve", "--help"}, {"version"}, {"--version"}} {
		cleanEnv(t)
		var out bytes.Buffer
		cmd := newCommand(Options{Version: "v0.0.5"}, func(_ *cobra.Command, _ cortex.Config) error { t.Fatal("unexpected server start"); return nil })
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "cortex") {
			t.Fatalf("output: %s", out.String())
		}
	}
}

func TestTenantURIReachesRealJobDBEndpoints(t *testing.T) {
	cleanEnv(t)
	jobdb := httptest.NewServer(remote.NewServer(toy.New()))
	defer jobdb.Close()
	cmd := newCommand(Options{}, func(_ *cobra.Command, cfg cortex.Config) error {
		server, err := cortex.NewRemoteServer(cfg)
		if err != nil {
			return err
		}
		for _, path := range []string{"/api/projects", "/api/projects/c2/jobs", "/api/projects/c2/user-inputs/pending"} {
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != 200 {
				t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
			}
			if path == "/api/projects" && !strings.Contains(response.Body.String(), `"tenant_id":"c2"`) {
				t.Fatalf("default tenant missing: %s", response.Body.String())
			}
		}
		return nil
	})
	cmd.SetArgs([]string{"--jobdb", jobdb.URL + "/c2"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}
