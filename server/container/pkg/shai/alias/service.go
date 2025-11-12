package alias

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const (
	aliasUser     = "shai"
	manifestName  = ".shai-cmds"
	containerRoot = "/src"
)

// Config contains inputs required to start the alias subsystem.
type Config struct {
	WorkingDir     string
	ShellPath      string
	ExecutablePath string
	ListenAddr     string
}

// MountSpec describes a file mount required for alias helpers.
type MountSpec struct {
	Source   string
	Target   string
	ReadOnly bool
}

// Service manages the lifecycle of the alias supervisor and helper assets.
type Service struct {
	env            []string
	mounts         []MountSpec
	controlWriter  *os.File
	supervisorCmd  *exec.Cmd
	supervisorDone chan error
	assetsDir      string
	closeOnce      sync.Once
}

// MaybeStart initializes the alias system if .shai-cmds exists. When the
// manifest is absent, the returned service is nil.
func MaybeStart(cfg Config) (*Service, error) {
	manifestPath := filepath.Join(cfg.WorkingDir, manifestName)
	if _, err := os.Stat(manifestPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat manifest: %w", err)
	}

	exePath := cfg.ExecutablePath
	if exePath == "" {
		exePath = os.Getenv("SHAI_ALIAS_SUPERVISOR_BIN")
	}
	if exePath == "" {
		path, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve shai executable: %w", err)
		}
		exePath = path
	}
	if !filepath.IsAbs(exePath) {
		abs, err := filepath.Abs(exePath)
		if err != nil {
			return nil, fmt.Errorf("resolve supervisor binary %q: %w", exePath, err)
		}
		exePath = abs
	}
	if _, err := os.Stat(exePath); err != nil {
		return nil, fmt.Errorf("supervisor binary %s invalid: %w", exePath, err)
	}

	shellPath := cfg.ShellPath
	if shellPath == "" {
		shellPath = os.Getenv("SHELL")
	}
	if shellPath == "" {
		shellPath = "/bin/bash"
	}

	port, err := allocatePort()
	if err != nil {
		return nil, fmt.Errorf("allocate alias port: %w", err)
	}
	listenAddr := cfg.ListenAddr
	if listenAddr == "" {
		if envAddr := os.Getenv("SHAI_ALIAS_LISTEN_ADDR"); envAddr != "" {
			listenAddr = envAddr
		} else {
			listenAddr = "0.0.0.0"
		}
	}

	password, err := randomToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate password: %w", err)
	}
	sessionID, err := randomToken(16)
	if err != nil {
		return nil, fmt.Errorf("generate session id: %w", err)
	}

	assetsDir, scriptPath, err := materializeAssets()
	if err != nil {
		return nil, err
	}

	cmd, controlWriter, err := launchSupervisor(supervisorLaunchConfig{
		ExecutablePath: exePath,
		ManifestPath:   manifestPath,
		WorkingDir:     cfg.WorkingDir,
		ShellPath:      shellPath,
		Password:       password,
		SessionID:      sessionID,
		Port:           port,
		ListenAddr:     listenAddr,
	})
	if err != nil {
		os.RemoveAll(assetsDir)
		return nil, err
	}

	service := &Service{
		env: []string{
			fmt.Sprintf("SHAI_ALIAS_SSH_HOSTPORT=%s:%d", containerHostAlias(), port),
			fmt.Sprintf("SHAI_ALIAS_SSH_USER=%s", aliasUser),
			fmt.Sprintf("SHAI_ALIAS_SSH_PASS=%s", password),
			fmt.Sprintf("SHAI_ALIAS_SESSION_ID=%s", sessionID),
		},
		mounts: []MountSpec{
			{Source: scriptPath, Target: "/usr/local/bin/shai-alias", ReadOnly: true},
			{Source: "/dev/null", Target: filepath.Join(containerRoot, manifestName), ReadOnly: true},
		},
		controlWriter:  controlWriter,
		supervisorCmd:  cmd,
		supervisorDone: make(chan error, 1),
		assetsDir:      assetsDir,
	}

	go func() {
		service.supervisorDone <- cmd.Wait()
	}()

	return service, nil
}

// Env returns environment variables to inject into the container.
func (s *Service) Env() []string {
	out := make([]string, len(s.env))
	copy(out, s.env)
	return out
}

// Mounts returns helper mounts.
func (s *Service) Mounts() []MountSpec {
	out := make([]MountSpec, len(s.mounts))
	copy(out, s.mounts)
	return out
}

// Close terminates the supervisor and cleans up temporary assets.
func (s *Service) Close() {
	s.closeOnce.Do(func() {
		if s.controlWriter != nil {
			_ = s.controlWriter.Close()
		}
		select {
		case <-s.supervisorDone:
			// already exited
		case <-time.After(2 * time.Second):
			if s.supervisorCmd != nil && s.supervisorCmd.Process != nil {
				_ = s.supervisorCmd.Process.Kill()
			}
		}
		_ = os.RemoveAll(s.assetsDir)
	})
}

func containerHostAlias() string {
	// Docker Desktop on macOS already injects host.docker.internal.
	// On Linux we add it via ExtraHosts.
	return "host.docker.internal"
}

func allocatePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected addr type %T", l.Addr())
	}
	return addr.Port, nil
}

func randomToken(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func materializeAssets() (dir, script string, err error) {
	dir, err = os.MkdirTemp("", "shai-alias-assets-")
	if err != nil {
		return
	}
	write := func(name string, data []byte) (string, error) {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o755); err != nil {
			return "", err
		}
		return path, nil
	}
	script, err = write("shai-alias.sh", shaiAliasScript)
	return
}

type supervisorLaunchConfig struct {
	ExecutablePath string
	ManifestPath   string
	WorkingDir     string
	ShellPath      string
	Password       string
	SessionID      string
	Port           int
	ListenAddr     string
}

func launchSupervisor(cfg supervisorLaunchConfig) (*exec.Cmd, *os.File, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}

	cmd := exec.Command(cfg.ExecutablePath)
	cmd.Env = append(os.Environ(),
		"SHAI_ALIAS_SUPERVISOR=1",
		fmt.Sprintf("SHAI_ALIAS_MANIFEST=%s", cfg.ManifestPath),
		fmt.Sprintf("SHAI_ALIAS_WORKDIR=%s", cfg.WorkingDir),
		fmt.Sprintf("SHAI_ALIAS_SHELL=%s", cfg.ShellPath),
		fmt.Sprintf("SHAI_ALIAS_PASSWORD=%s", cfg.Password),
		fmt.Sprintf("SHAI_ALIAS_SESSION_ID=%s", cfg.SessionID),
		fmt.Sprintf("SHAI_ALIAS_BIND_PORT=%d", cfg.Port),
		fmt.Sprintf("SHAI_ALIAS_LISTEN_ADDR=%s", cfg.ListenAddr),
		"SHAI_ALIAS_CONTROL_FD=3",
	)
	cmd.ExtraFiles = []*os.File{reader}
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		reader.Close()
		writer.Close()
		return nil, nil, fmt.Errorf("start alias supervisor: %w", err)
	}
	reader.Close()
	return cmd, writer, nil
}
