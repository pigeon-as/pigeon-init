package process

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/pigeon-as/pigeon-init/internal/user"
)

type Result struct {
	ExitCode  int  `json:"code"`
	OOMKilled bool `json:"oom_killed"`
}

type Supervisor struct {
	cmd *exec.Cmd
	pid int
	pr  *os.File

	result   Result
	resultCh chan struct{}
	mu       sync.Mutex

	SignalCh chan os.Signal

	execMu    sync.Mutex
	execWaits map[int]chan<- unix.WaitStatus

	logger *slog.Logger
}

func New(argv []string, env []string, workDir string, identity *user.Identity, logger *slog.Logger) (*Supervisor, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty argv")
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = env
	cmd.Dir = workDir

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: identity.UID,
			Gid: identity.GID,
		},
		Setsid: true,
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create pipe: %w", err)
	}

	if err := pr.Chown(int(identity.UID), int(identity.GID)); err != nil {
		pr.Close()
		pw.Close()
		return nil, fmt.Errorf("chown pipe reader: %w", err)
	}
	if err := pw.Chown(int(identity.UID), int(identity.GID)); err != nil {
		pr.Close()
		pw.Close()
		return nil, fmt.Errorf("chown pipe writer: %w", err)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = pw
	cmd.Stderr = pw

	return &Supervisor{
		cmd:      cmd,
		pr:       pr,
		resultCh: make(chan struct{}),
		SignalCh: make(chan os.Signal, 16),
		logger:   logger,
	}, nil
}

func (s *Supervisor) Start() error {
	if err := s.cmd.Start(); err != nil {
		s.pr.Close()
		if w, ok := s.cmd.Stdout.(*os.File); ok {
			w.Close()
		}
		return fmt.Errorf("start workload: %w", err)
	}
	s.pid = s.cmd.Process.Pid

	// Close pipe write end in our process.
	if w, ok := s.cmd.Stdout.(*os.File); ok {
		w.Close()
	}

	// Copy pipe output to init stdout.
	go func() {
		_, _ = io.Copy(os.Stdout, s.pr)
		s.pr.Close()
	}()

	s.logger.Info("workload started", "pid", s.pid, "argv", s.cmd.Args)
	return nil
}

func (s *Supervisor) Run() Result {
	sigchld := make(chan os.Signal, 8)
	signal.Notify(sigchld, syscall.SIGCHLD)
	defer signal.Stop(sigchld)

	hostSigs := make(chan os.Signal, 8)
	signal.Notify(hostSigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP)
	defer signal.Stop(hostSigs)

	for {
		select {
		case <-sigchld:
			if s.reap() {
				return s.result
			}

		case sig := <-s.SignalCh:
			s.forwardSignal(sig)

		case sig := <-hostSigs:
			s.forwardSignal(sig)
		}
	}
}

func (s *Supervisor) WaitResult() <-chan struct{} {
	return s.resultCh
}

func (s *Supervisor) GetResult() Result {
	return s.result
}

func (s *Supervisor) Lock() {
	s.mu.Lock()
}

func (s *Supervisor) Unlock() {
	s.mu.Unlock()
}

// RegisterExec registers an exec child PID with the reap loop.
// Must be called while holding Lock().
func (s *Supervisor) RegisterExec(pid int) <-chan unix.WaitStatus {
	ch := make(chan unix.WaitStatus, 1)
	s.execMu.Lock()
	if s.execWaits == nil {
		s.execWaits = make(map[int]chan<- unix.WaitStatus)
	}
	s.execWaits[pid] = ch
	s.execMu.Unlock()
	return ch
}

func (s *Supervisor) UnregisterExec(pid int) {
	s.execMu.Lock()
	delete(s.execWaits, pid)
	s.execMu.Unlock()
}

func (s *Supervisor) reap() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		var ws unix.WaitStatus
		pid, err := unix.Wait4(-1, &ws, unix.WNOHANG, nil)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return false
		}
		if pid <= 0 {
			return false
		}

		if pid == s.pid {
			if ws.Exited() {
				s.result.ExitCode = ws.ExitStatus()
			} else if ws.Signaled() {
				s.result.ExitCode = 128 + int(ws.Signal())
			}
			s.result.OOMKilled = checkOOM(s.pid)
			s.logger.Info("workload exited", "pid", pid, "exit_code", s.result.ExitCode, "oom", s.result.OOMKilled)
			close(s.resultCh)
			return true
		}

		// Check exec session children.
		s.execMu.Lock()
		if ch, ok := s.execWaits[pid]; ok {
			ch <- ws
			delete(s.execWaits, pid)
			s.execMu.Unlock()
			continue
		}
		s.execMu.Unlock()

		// Reaped orphan.
		s.logger.Debug("reaped orphan", "pid", pid)
	}
}

func (s *Supervisor) forwardSignal(sig os.Signal) {
	sysSignal, ok := sig.(syscall.Signal)
	if !ok {
		return
	}
	if err := unix.Kill(-s.pid, sysSignal); err != nil {
		s.logger.Warn("signal forward failed", "signal", sig, "pid", s.pid, "err", err)
	}
}

// checkOOM reads /dev/kmsg looking for OOM kill of the given PID.
func checkOOM(pid int) bool {
	f, err := os.Open("/dev/kmsg")
	if err != nil {
		return false
	}

	needle := fmt.Sprintf("Killed process %d", pid)

	done := make(chan bool, 1)
	go func() {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), needle) {
				done <- true
				return
			}
		}
		done <- false
	}()

	select {
	case oom := <-done:
		return oom
	case <-time.After(10 * time.Millisecond):
		f.Close()
		return false
	}
}
