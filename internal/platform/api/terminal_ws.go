package api

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/store"
	"golang.org/x/net/websocket"
)

type terminalMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols uint32 `json:"cols,omitempty"`
	Rows uint32 `json:"rows,omitempty"`
}

func (s *Server) websocketHandler() http.Handler {
	return websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()

		req := ws.Request()
		webSession, err := s.auth.ParseSessionCookie(req)
		if err != nil {
			_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: "authentication required"})
			return
		}

		user, err := s.store.GetUserByID(webSession.UserID)
		if err != nil || !user.Enabled {
			_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: "invalid session"})
			return
		}
		if webSession.Version != user.SessionVersion {
			_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: "invalid session"})
			return
		}
		if user.MustChangePassword {
			_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: "password change required"})
			return
		}

		remoteConnections, err := s.syncProjectRuntimeHosts(req.Context(), requestedProject(req))
		if err != nil {
			_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: err.Error()})
			return
		}

		host, ok := s.authorizeHostForUser(user, req.PathValue("stableID"))
		if !ok {
			_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: "host not found"})
			return
		}

		if remoteConnections == nil {
			useRuntime, runtimeErr := s.projectUsesRemoteRuntime(host.Project)
			if runtimeErr != nil {
				_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: runtimeErr.Error()})
				return
			}
			if useRuntime {
				remoteConnections, err = s.syncProjectRuntimeHosts(req.Context(), host.Project)
				if err != nil {
					_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: err.Error()})
					return
				}
			}
		}

		connectionID, err := s.resolveConnectionIDForHost(host, req.URL.Query().Get("connectionId"), remoteConnections)
		if err != nil {
			_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: err.Error()})
			return
		}

		cols := parseUint32(req.URL.Query().Get("cols"), 120)
		rows := parseUint32(req.URL.Query().Get("rows"), 36)
		shell := req.URL.Query().Get("shell")

		useRuntime, err := s.projectUsesRemoteRuntime(host.Project)
		if err != nil {
			_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: err.Error()})
			return
		}

		sessionUID := fmt.Sprintf("web-terminal-%d", time.Now().UnixNano())
		_, _ = s.store.StartSession(store.SessionRecord{
			SessionUID:       sessionUID,
			Type:             "web-terminal",
			Status:           "active",
			Source:           "web",
			Username:         user.Username,
			Role:             user.Role,
			HostStableID:     host.StableID,
			HostConnectionID: connectionID,
			Hostname:         host.Hostname,
			RemoteAddr:       host.RemoteAddr,
			Command:          shell,
			StartedAt:        time.Now(),
		})

		if useRuntime {
			if err := s.proxyRuntimeTerminal(ws, req, user, host, sessionUID, connectionID, cols, rows, shell); err != nil {
				_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: err.Error()})
			}
			return
		}

		session, err := s.rsshService.OpenInteractiveSessionOnConnection(connectionID, cols, rows, shell)
		if err != nil {
			_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: err.Error()})
			_ = s.store.FinishSession(sessionUID, "failed", err.Error(), time.Now())
			return
		}
		defer session.Close()

		outbound := make(chan terminalMessage, 32)
		done := make(chan struct{})
		var once sync.Once

		finish := func(status, errText string) {
			once.Do(func() {
				_ = s.store.FinishSession(sessionUID, status, errText, time.Now())
				_ = s.store.TouchHostActivity(host.StableID, time.Now())
				close(done)
			})
		}

		sendMessage := func(message terminalMessage) bool {
			select {
			case outbound <- message:
				return true
			case <-done:
				return false
			}
		}

		go func() {
			for {
				select {
				case <-done:
					return
				case message := <-outbound:
					if err := websocket.JSON.Send(ws, message); err != nil {
						finish("failed", err.Error())
						return
					}
				}
			}
		}()

		go func() {
			buffer := make([]byte, 4096)
			for {
				n, err := session.Read(buffer)
				if n > 0 {
					if !sendMessage(terminalMessage{Type: "output", Data: string(buffer[:n])}) {
						return
					}
				}
				if err != nil {
					if err == io.EOF {
						_ = sendMessage(terminalMessage{Type: "status", Data: "closed"})
						finish("completed", "")
						return
					}
					_ = sendMessage(terminalMessage{Type: "error", Data: err.Error()})
					finish("failed", err.Error())
					return
				}
			}
		}()

		for {
			select {
			case <-done:
				close(outbound)
				return
			default:
			}

			var message terminalMessage
			if err := websocket.JSON.Receive(ws, &message); err != nil {
				finish("completed", "")
				return
			}

			switch message.Type {
			case "input":
				if _, err := session.Write([]byte(message.Data)); err != nil {
					_ = sendMessage(terminalMessage{Type: "error", Data: err.Error()})
					finish("failed", err.Error())
				}
			case "resize":
				if message.Cols > 0 && message.Rows > 0 {
					if err := session.Resize(message.Cols, message.Rows); err != nil {
						_ = sendMessage(terminalMessage{Type: "error", Data: err.Error()})
						finish("failed", err.Error())
					}
				}
			case "close":
				finish("completed", "")
				return
			}
		}
	})
}

func (s *Server) proxyRuntimeTerminal(ws *websocket.Conn, req *http.Request, user store.WebUser, host store.HostRecord, sessionUID, connectionID string, cols, rows uint32, shell string) error {
	runtimeWS, err := s.runtimes.DialTerminal(req.Context(), host.Project, host.StableID, connectionID, cols, rows, shell)
	if err != nil {
		_ = s.store.FinishSession(sessionUID, "failed", err.Error(), time.Now())
		return err
	}
	defer runtimeWS.Close()

	done := make(chan struct{})
	var once sync.Once

	finish := func(status, errText string) {
		once.Do(func() {
			_ = s.store.FinishSession(sessionUID, status, errText, time.Now())
			_ = s.store.TouchHostActivity(host.StableID, time.Now())
			close(done)
		})
	}

	go func() {
		for {
			var message terminalMessage
			if err := websocket.JSON.Receive(runtimeWS, &message); err != nil {
				finish("completed", "")
				return
			}
			if err := websocket.JSON.Send(ws, message); err != nil {
				finish("failed", err.Error())
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return nil
		default:
		}

		var message terminalMessage
		if err := websocket.JSON.Receive(ws, &message); err != nil {
			finish("completed", "")
			return nil
		}
		if err := websocket.JSON.Send(runtimeWS, message); err != nil {
			finish("failed", err.Error())
			return err
		}
	}
}

func (s *Server) authorizeHostForUser(user store.WebUser, stableID string) (store.HostRecord, bool) {
	host, err := s.store.GetHostByStableID(stableID)
	if err != nil {
		return store.HostRecord{}, false
	}

	if canAccessProject(user, host.Project) {
		return host, true
	}

	return store.HostRecord{}, false
}

func parseUint32(raw string, fallback uint32) uint32 {
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return fallback
	}
	return uint32(value)
}
