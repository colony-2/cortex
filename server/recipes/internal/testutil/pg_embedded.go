package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

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
		sqlDB, err := e.DB.DB()
		if err == nil && sqlDB != nil {
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

func SQLDB(db *gorm.DB) *sql.DB {
	sqlDB, _ := db.DB()
	return sqlDB
}
