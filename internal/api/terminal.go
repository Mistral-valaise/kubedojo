package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/coder/websocket"
)

// CloseTerminalLimit is the WebSocket close code sent when a session already
// has the maximum number of terminals open (application range 4000–4999).
const CloseTerminalLimit websocket.StatusCode = 4429

func (s *Server) terminal(w http.ResponseWriter, r *http.Request) {
	v := s.session(w, r)
	if v == nil {
		return
	}
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.AllowedOrigins})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	tty, err := s.Sessions.Attach(ctx, v.ID, uint(cols), uint(rows))
	if errors.Is(err, session.ErrTerminalLimit) {
		// Browsers hide the HTTP status of a failed WebSocket handshake, so the
		// limit is reported as a close code the UI can tell apart.
		c.Close(CloseTerminalLimit, "terminal limit reached")
		return
	}
	if err != nil {
		_ = c.Write(ctx, websocket.MessageBinary, []byte("\r\n\x1b[31mCould not open terminal: "+err.Error()+"\x1b[0m\r\n"))
		c.Close(websocket.StatusInternalError, "attach failed")
		return
	}
	defer tty.Close()
	if s.OnTerminal != nil {
		s.OnTerminal(1)
		defer s.OnTerminal(-1)
	}

	go func() { // sandbox -> browser
		defer cancel()
		buf := make([]byte, 32*1024)
		for {
			n, err := tty.Read(buf)
			if n > 0 {
				if c.Write(ctx, websocket.MessageBinary, buf[:n]) != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() { // keep-alive through proxies
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, pc := context.WithTimeout(ctx, 10*time.Second)
				err := c.Ping(pctx)
				pc()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	for { // browser -> sandbox
		typ, data, err := c.Read(ctx)
		if err != nil {
			break
		}
		if typ == websocket.MessageText {
			var msg struct {
				Type string `json:"type"`
				Cols uint   `json:"cols"`
				Rows uint   `json:"rows"`
			}
			if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" && msg.Cols > 0 && msg.Rows > 0 {
				_ = tty.Resize(ctx, msg.Cols, msg.Rows)
			}
			continue
		}
		if _, err := tty.Write(data); err != nil {
			break
		}
	}
	c.Close(websocket.StatusNormalClosure, "")
}
