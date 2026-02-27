package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/mdlayher/vsock"

	"github.com/pigeon-as/pigeon-init/internal/process"
)

const (
	vsockPort = 10000
)

type Server struct {
	supervisor *process.Supervisor
	env        []string
	mux        *http.ServeMux
	logger     *slog.Logger
}

func NewServer(sup *process.Supervisor, env []string, logger *slog.Logger) *Server {
	s := &Server{
		supervisor: sup,
		env:        env,
		mux:        http.NewServeMux(),
		logger:     logger,
	}

	s.mux.HandleFunc("GET /v1/status", s.handleStatus)
	s.mux.HandleFunc("GET /v1/exit_code", s.handleExitCode)
	s.mux.HandleFunc("POST /v1/signals", s.handleSignal)
	s.mux.HandleFunc("POST /v1/exec", s.handleExec)
	s.mux.HandleFunc("GET /v1/ws/exec", s.handleExecWS)

	return s
}

func (s *Server) Serve(ctx context.Context) error {
	ln, err := vsock.Listen(vsockPort, nil)
	if err != nil {
		return fmt.Errorf("vsock listen: %w", err)
	}

	srv := &http.Server{
		Handler: s.mux,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	go func() {
		<-ctx.Done()
		srv.Close()
	}()

	s.logger.Info("vsock API listening", "port", vsockPort)
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("vsock serve: %w", err)
	}
	return nil
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleExitCode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	select {
	case <-s.supervisor.WaitResult():
		writeJSON(w, http.StatusOK, s.supervisor.GetResult())
	case <-ctx.Done():
		http.Error(w, "timeout", http.StatusGatewayTimeout)
	}
}

func (s *Server) handleSignal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Signal int `json:"signal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	sig := syscall.Signal(req.Signal)
	if sig < 1 || sig > 64 {
		http.Error(w, "invalid signal number", http.StatusBadRequest)
		return
	}

	s.supervisor.SignalCh <- sig
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cmd []string `json:"cmd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Cmd) == 0 {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	s.supervisor.Lock()
	defer s.supervisor.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, req.Cmd[0], req.Cmd[1:]...)
	cmd.Env = s.env
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	stdout, err := cmd.Output()

	var exitCode int
	var exitSignal int

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				exitSignal = int(ws.Signal())
			}
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"exit_code":   exitCode,
		"exit_signal": exitSignal,
		"stdout":      string(stdout),
		"stderr":      stderrBuf.String(),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func BuildArgv(execOverride []string, entrypoint []string, cmd []string, cmdOverride *string) []string {
	if len(execOverride) > 0 {
		return execOverride
	}

	var argv []string
	argv = append(argv, entrypoint...)

	if cmdOverride != nil {
		argv = append(argv, *cmdOverride)
	} else {
		argv = append(argv, cmd...)
	}

	return argv
}

func BuildEnv(imageEnv []string, extraEnv map[string]string, homeDir string) []string {
	env := make(map[string]string)

	for _, e := range imageEnv {
		if k, v, ok := parseEnvVar(e); ok {
			env[k] = v
		}
	}

	for k, v := range extraEnv {
		env[k] = v
	}

	if _, ok := env["HOME"]; !ok {
		env["HOME"] = homeDir
	}

	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}

func parseEnvVar(s string) (string, string, bool) {
	i := strings.IndexByte(s, '=')
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}
