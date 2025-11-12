package alias

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
)

type supervisorConfig struct {
	ManifestPath string
	WorkingDir   string
	ShellPath    string
	Password     string
	SessionID    string
	BindPort     int
	ControlFile  *os.File
	Debug        bool
	ListenAddr   string
}

// RunSupervisor executes the alias supervisor binary mode.
func RunSupervisor() error {
	cfg, err := readSupervisorConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	if cfg.ControlFile != nil {
		go func() {
			defer cancel()
			buf := make([]byte, 1)
			for {
				_, err := cfg.ControlFile.Read(buf)
				if err != nil {
					return
				}
			}
		}()
	}
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	supervisor := newSupervisor(cfg)
	defer func() {
		if cfg.ControlFile != nil {
			cfg.ControlFile.Close()
		}
	}()
	return supervisor.run(ctx)
}

func readSupervisorConfig() (*supervisorConfig, error) {
	getenv := func(key string) (string, error) {
		val := strings.TrimSpace(os.Getenv(key))
		if val == "" {
			return "", fmt.Errorf("missing %s", key)
		}
		return val, nil
	}
	manifest, err := getenv("SHAI_ALIAS_MANIFEST")
	if err != nil {
		return nil, err
	}
	workdir, err := getenv("SHAI_ALIAS_WORKDIR")
	if err != nil {
		return nil, err
	}
	shell, err := getenv("SHAI_ALIAS_SHELL")
	if err != nil {
		return nil, err
	}
	password, err := getenv("SHAI_ALIAS_PASSWORD")
	if err != nil {
		return nil, err
	}
	sessionID, err := getenv("SHAI_ALIAS_SESSION_ID")
	if err != nil {
		return nil, err
	}
	portStr, err := getenv("SHAI_ALIAS_BIND_PORT")
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid alias port: %w", err)
	}

	var controlFile *os.File
	if fdStr := os.Getenv("SHAI_ALIAS_CONTROL_FD"); fdStr != "" {
		fd, err := strconv.Atoi(fdStr)
		if err != nil {
			return nil, fmt.Errorf("invalid control fd: %w", err)
		}
		controlFile = os.NewFile(uintptr(fd), "alias-control")
	}

	listenAddr := os.Getenv("SHAI_ALIAS_LISTEN_ADDR")
	if strings.TrimSpace(listenAddr) == "" {
		listenAddr = "0.0.0.0"
	}

	return &supervisorConfig{
		ManifestPath: manifest,
		WorkingDir:   workdir,
		ShellPath:    shell,
		Password:     password,
		SessionID:    sessionID,
		BindPort:     port,
		ControlFile:  controlFile,
		Debug:        os.Getenv("SHAI_ALIAS_DEBUG") != "",
		ListenAddr:   listenAddr,
	}, nil
}

type supervisor struct {
	cfg    *supervisorConfig
	server *ssh.Server

	mu    sync.Mutex
	procs map[*exec.Cmd]context.CancelFunc
}

func newSupervisor(cfg *supervisorConfig) *supervisor {
	s := &supervisor{
		cfg:   cfg,
		procs: make(map[*exec.Cmd]context.CancelFunc),
	}
	addr := cfg.ListenAddr
	if strings.TrimSpace(addr) == "" {
		addr = "0.0.0.0"
	}
	s.server = &ssh.Server{
		Addr:    fmt.Sprintf("%s:%d", addr, cfg.BindPort),
		Version: "shai-alias",
		PasswordHandler: func(ctx ssh.Context, password string) bool {
			return ctx.User() == aliasUser && password == cfg.Password
		},
		Handler: s.handleSession,
	}
	return s
}

func (s *supervisor) run(ctx context.Context) error {
	s.debugf("starting supervisor on %s manifest=%s", s.server.Addr, s.cfg.ManifestPath)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
		s.terminateAll()
		return nil
	case err := <-errCh:
		s.terminateAll()
		if errors.Is(err, ssh.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *supervisor) handleSession(sess ssh.Session) {
	cmd := sess.Command()
	if len(cmd) == 0 {
		s.exitWithError(sess, 2, "missing command")
		return
	}
	s.debugf("session command=%v", cmd)
	switch cmd[0] {
	case "alias-list":
		s.handleList(sess)
	case "alias-run":
		s.handleRun(sess, cmd)
	default:
		s.exitWithError(sess, 2, fmt.Sprintf("unknown command %q", cmd[0]))
	}
}

func (s *supervisor) handleList(sess ssh.Session) {
	manifest, err := LoadManifest(s.cfg.ManifestPath)
	if err != nil {
		s.exitWithError(sess, 1, fmt.Sprintf("failed to read manifest: %v", err))
		return
	}
	s.debugf("alias-list entries=%d", len(manifest.Entries))
	names := make([]string, 0, len(manifest.Entries))
	for name := range manifest.Entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry := manifest.Entries[name]
		fmt.Fprintf(sess, "%s\t%s\n", name, entry.Command)
	}
	_ = sess.Exit(0)
}

func (s *supervisor) handleRun(sess ssh.Session, parts []string) {
	if len(parts) < 2 {
		s.exitWithError(sess, 2, "missing alias")
		return
	}
	aliasName := parts[1]
	argsEncoded := ""
	if len(parts) >= 3 {
		argsEncoded = parts[2]
	}
	argBytes := []byte{}
	if argsEncoded != "" {
		var err error
		argBytes, err = base64.StdEncoding.DecodeString(argsEncoded)
		if err != nil {
			s.exitWithError(sess, 2, fmt.Sprintf("invalid args payload: %v", err))
			return
		}
	}
	argString := string(argBytes)
	manifest, err := LoadManifest(s.cfg.ManifestPath)
	if err != nil {
		s.exitWithError(sess, 1, fmt.Sprintf("failed to read manifest: %v", err))
		return
	}
	entry, ok := manifest.Entries[aliasName]
	if !ok {
		s.exitWithError(sess, 2, fmt.Sprintf("alias %q not found", aliasName))
		return
	}
	if err := entry.ValidateArgs(argString); err != nil {
		s.exitWithError(sess, 2, err.Error())
		return
	}
	commandLine := entry.Command
	if strings.TrimSpace(argString) != "" {
		commandLine = commandLine + " " + argString
	}
	s.debugf("alias-run alias=%s args=%q command=%s", aliasName, argString, commandLine)
	s.executeCommand(sess, commandLine)
}

func (s *supervisor) executeCommand(sess ssh.Session, commandLine string) {
	ctx, cancel := context.WithCancel(sess.Context())
	cmd := exec.CommandContext(ctx, s.cfg.ShellPath, "-lc", commandLine)
	cmd.Dir = s.cfg.WorkingDir
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	s.trackProcess(cmd, cancel)
	defer s.untrackProcess(cmd)

	var err error
	if ptyReq, winCh, ok := sess.Pty(); ok {
		err = s.runWithPTY(sess, cmd, ptyReq, winCh)
	} else {
		err = s.runWithPipes(sess, cmd)
	}

	if err == nil {
		_ = sess.Exit(0)
		return
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		_ = sess.Exit(exitErr.ExitCode())
		return
	}
	s.exitWithError(sess, 1, fmt.Sprintf("command failed: %v", err))
}

func (s *supervisor) runWithPTY(sess ssh.Session, cmd *exec.Cmd, ptyReq ssh.Pty, winCh <-chan ssh.Window) error {
	ptmx, err := pty.StartWithAttrs(cmd, &pty.Winsize{
		Rows: uint16(ptyReq.Window.Height),
		Cols: uint16(ptyReq.Window.Width),
	}, cmd.SysProcAttr)
	if err != nil {
		return err
	}
	defer ptmx.Close()

	done := make(chan struct{})
	go func() {
		io.Copy(ptmx, sess)
		ptmx.Close()
		close(done)
	}()
	go func() {
		for win := range winCh {
			_ = pty.Setsize(ptmx, &pty.Winsize{
				Rows: uint16(win.Height),
				Cols: uint16(win.Width),
			})
		}
	}()
	s.forwardSignals(sess, cmd)
	_, copyErr := io.Copy(sess, ptmx)
	<-done
	waitErr := cmd.Wait()
	if copyErr == io.EOF {
		copyErr = nil
	}
	if waitErr != nil {
		s.debugf("command exit error=%v", waitErr)
		return waitErr
	}
	s.debugf("command exit success")
	return copyErr
}

func (s *supervisor) runWithPipes(sess ssh.Session, cmd *exec.Cmd) error {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_, _ = io.Copy(stdin, sess)
		stdin.Close()
	}()
	go io.Copy(sess, stdout)
	go io.Copy(sess.Stderr(), stderr)
	s.forwardSignals(sess, cmd)
	err = cmd.Wait()
	if err != nil {
		s.debugf("command exit error=%v", err)
	} else {
		s.debugf("command exit success")
	}
	return err
}

func (s *supervisor) forwardSignals(sess ssh.Session, cmd *exec.Cmd) {
	sigCh := make(chan ssh.Signal, 4)
	sess.Signals(sigCh)
	go func() {
		defer sess.Signals(nil)
		for sig := range sigCh {
			if mapped, ok := mapSignal(sig); ok {
				_ = syscall.Kill(-cmd.Process.Pid, mapped)
			}
		}
	}()
}

func mapSignal(sig ssh.Signal) (syscall.Signal, bool) {
	switch sig {
	case ssh.SIGINT:
		return syscall.SIGINT, true
	case ssh.SIGTERM:
		return syscall.SIGTERM, true
	case ssh.SIGKILL:
		return syscall.SIGKILL, true
	case ssh.SIGQUIT:
		return syscall.SIGQUIT, true
	default:
		return 0, false
	}
}

func (s *supervisor) trackProcess(cmd *exec.Cmd, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.procs[cmd] = cancel
}

func (s *supervisor) untrackProcess(cmd *exec.Cmd) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.procs, cmd)
}

func (s *supervisor) terminateAll() {
	s.mu.Lock()
	cmds := make(map[*exec.Cmd]context.CancelFunc, len(s.procs))
	for cmd, cancel := range s.procs {
		cmds[cmd] = cancel
	}
	s.procs = make(map[*exec.Cmd]context.CancelFunc)
	s.mu.Unlock()

	for cmd, cancel := range cmds {
		cancel()
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		}
	}
	time.Sleep(500 * time.Millisecond)
	for cmd := range cmds {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}
}

func (s *supervisor) exitWithError(sess ssh.Session, code int, msg string) {
	s.debugf("exit error code=%d msg=%s", code, msg)
	if msg != "" {
		fmt.Fprintf(sess.Stderr(), "%s\n", msg)
	}
	_ = sess.Exit(code)
}

func (s *supervisor) debugf(format string, args ...interface{}) {
	if s.cfg == nil || !s.cfg.Debug {
		return
	}
	fmt.Fprintf(os.Stderr, "[alias-supervisor] "+format+"\n", args...)
}
