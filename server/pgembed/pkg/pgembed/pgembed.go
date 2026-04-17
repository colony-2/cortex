package pgembed

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/git"
	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type EmbeddedPostgres struct {
	pg  *embeddedpostgres.EmbeddedPostgres
	DB  *gorm.DB
	dsn string
}

func StartEmbeddedPostgres(t testing.TB) *EmbeddedPostgres {
	t.Helper()

	port, err := reservePort()
	if err != nil {
		t.Fatalf("pg_embedded: reserve port failed: %v", err)
	}

	tmpRoot := t.TempDir()
	runtimePath := filepath.Join(tmpRoot, "runtime")
	dataPath := filepath.Join(tmpRoot, "data")

	for _, dir := range []string{runtimePath, dataPath} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("pg_embedded: create dir %s failed: %v", dir, err)
		}
	}

	config := embeddedpostgres.DefaultConfig().
		Port(port).
		RuntimePath(runtimePath).
		DataPath(dataPath).
		StartTimeout(5 * time.Minute).
		Logger(io.Discard)
	pg := embeddedpostgres.NewDatabase(config)
	if err := pg.Start(); err != nil {
		t.Fatalf("pg_embedded: start failed: %v", err)
	}

	dsn := fmt.Sprintf("%s?sslmode=disable", config.GetConnectionURL())
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		_ = pg.Stop()
		t.Fatalf("pg_embedded: connect failed: %v", err)
	}

	return &EmbeddedPostgres{
		pg:  pg,
		DB:  gdb,
		dsn: dsn,
	}
}

func (e *EmbeddedPostgres) Close(t testing.TB) {
	t.Helper()
	if e == nil {
		return
	}
	if e.DB != nil {
		if sqlDB, err := e.DB.DB(); err == nil && sqlDB != nil {
			_ = sqlDB.Close()
		}
	}
	if e.pg != nil {
		if err := e.pg.Stop(); err != nil {
			t.Logf("pg_embedded: stop failed: %v", err)
		}
	}
}

func (e *EmbeddedPostgres) DSN() string {
	if e == nil {
		return ""
	}
	return e.dsn
}

func reservePort() (uint32, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected addr type %T", l.Addr())
	}
	return uint32(addr.Port), nil
}

func MustCloseIterator(t testing.TB, it interface{ Close(context.Context) error }) {
	t.Helper()
	if it == nil {
		return
	}
	if err := it.Close(context.Background()); err != nil {
		t.Fatalf("iterator close failed: %v", err)
	}
}

func NewRealGitRepository() git.Repository {
	return git.NewRepository(git.Config{
		DefaultAuthor: "Test User",
		DefaultEmail:  "test@example.com",
	})
}

func GitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	return cmd
}

func ConfigureGitUser(ctx context.Context, dir string) error {
	cmd := GitCommand(ctx, dir, "config", "user.email", "test@example.com")
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = GitCommand(ctx, dir, "config", "user.name", "Test User")
	return cmd.Run()
}

func CreateGitRepo(ctx context.Context, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := GitCommand(ctx, dir, "init").Run(); err != nil {
		return err
	}
	return ConfigureGitUser(ctx, dir)
}
