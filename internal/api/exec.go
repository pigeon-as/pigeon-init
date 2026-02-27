// WebSocket exec protocol (matches Fly.io init-snapshot).
//
// Route: GET /v1/ws/exec
//
//	Client → Server:
//	  Text (first):  {"command":["cmd",...], "tty": bool}   Init
//	  Text:          {"cols": N, "rows": N}                 Resize (tty only)
//	  Binary:        raw stdin bytes
//	  Close:         terminate session
//
//	Server → Client:
//	  Binary:        raw stdout bytes (up to 64 KB chunks)
//	  Text:          {"Exit":{"code":N,"signal":N}}         exit notification
//	  Text:          {"Error":{"message":"..."}}            error notification
//	  Close:         session complete
package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

type initMsg struct {
	Command []string `json:"command"`
	TTY     bool     `json:"tty"`
}

type resizeMsg struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func (s *Server) handleExecWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		s.logger.Error("ws exec: accept failed", "err", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer c.CloseNow()

	c.SetReadLimit(128 * 1024)

	// Read init message.
	typ, data, err := c.Read(ctx)
	if err != nil {
		return
	}
	if typ != websocket.MessageText {
		c.Close(websocket.StatusProtocolError, "expected text init message")
		return
	}

	var init initMsg
	if err := json.Unmarshal(data, &init); err != nil || len(init.Command) == 0 {
		c.Close(websocket.StatusProtocolError, "invalid init message")
		return
	}

	s.logger.Debug("ws exec", "command", init.Command, "tty", init.TTY)

	// Build command.
	cmd := exec.Command(init.Command[0], init.Command[1:]...)
	cmd.Env = s.env

	var (
		stdinW  io.Writer // nil in non-tty mode
		stdoutR io.Reader
		ptmx    *os.File // non-nil when tty=true
	)

	// Spawn (hold reap lock to prevent race).
	s.supervisor.Lock()
	if init.TTY {
		cmd.Env = append(append([]string{}, s.env...), "TERM=xterm-256color")
		ptmx, err = pty.Start(cmd)
		if err != nil {
			s.supervisor.Unlock()
			wsError(c, ctx, "spawn: "+err.Error())
			return
		}
		defer ptmx.Close()
		stdinW = ptmx
		stdoutR = ptmx
	} else {
		pr, pw, pErr := os.Pipe()
		if pErr != nil {
			s.supervisor.Unlock()
			wsError(c, ctx, "pipe: "+pErr.Error())
			return
		}
		cmd.Stdout = pw
		cmd.Stderr = os.Stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			s.supervisor.Unlock()
			pr.Close()
			pw.Close()
			wsError(c, ctx, "spawn: "+err.Error())
			return
		}
		pw.Close()
		defer pr.Close()
		stdoutR = pr
	}

	// Register with reap loop (still holding lock).
	exitCh := s.supervisor.RegisterExec(cmd.Process.Pid)
	s.supervisor.Unlock()

	// Best-effort cleanup on any exit path.
	defer func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}()
	defer s.supervisor.UnregisterExec(cmd.Process.Pid)

	// I/O goroutines.
	stdoutDone := make(chan struct{})
	wsDone := make(chan struct{})

	// stdout → websocket
	go func() {
		defer close(stdoutDone)
		buf := make([]byte, 65536)
		for {
			n, rErr := stdoutR.Read(buf)
			if n > 0 {
				if wErr := c.Write(ctx, websocket.MessageBinary, buf[:n]); wErr != nil {
					return
				}
			}
			if rErr != nil {
				return
			}
		}
	}()

	// websocket → stdin + resize
	go func() {
		defer close(wsDone)
		for {
			msgType, msgData, rErr := c.Read(ctx)
			if rErr != nil {
				return
			}
			switch msgType {
			case websocket.MessageBinary:
				if stdinW != nil {
					_, _ = stdinW.Write(msgData)
				}
			case websocket.MessageText:
				var resize resizeMsg
				if json.Unmarshal(msgData, &resize) == nil && resize.Cols > 0 && resize.Rows > 0 && ptmx != nil {
					_ = pty.Setsize(ptmx, &pty.Winsize{Cols: resize.Cols, Rows: resize.Rows})
				}
			}
		}
	}()

	// Wait for child exit or client disconnect.
	select {
	case ws := <-exitCh:
		wsExit(c, ctx, ws)
		// Drain remaining stdout (brief timeout).
		select {
		case <-stdoutDone:
		case <-time.After(time.Second):
		}
	case <-wsDone:
		// Client disconnected; kill handled by defer.
	}

	cancel()
	c.Close(websocket.StatusNormalClosure, "")
}

func wsExit(c *websocket.Conn, ctx context.Context, ws unix.WaitStatus) {
	var code, sig *int
	if ws.Exited() {
		v := ws.ExitStatus()
		code = &v
	}
	if ws.Signaled() {
		v := int(ws.Signal())
		sig = &v
	}
	msg, _ := json.Marshal(map[string]any{
		"Exit": map[string]any{"code": code, "signal": sig},
	})
	_ = c.Write(ctx, websocket.MessageText, msg)
}

func wsError(c *websocket.Conn, ctx context.Context, message string) {
	msg, _ := json.Marshal(map[string]any{
		"Error": map[string]any{"message": message},
	})
	_ = c.Write(ctx, websocket.MessageText, msg)
	c.Close(websocket.StatusInternalError, "")
}
