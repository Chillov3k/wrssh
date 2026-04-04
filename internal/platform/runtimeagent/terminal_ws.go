package runtimeagent

import (
	"io"
	"strconv"
	"strings"

	"golang.org/x/net/websocket"
)

type terminalMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols uint32 `json:"cols,omitempty"`
	Rows uint32 `json:"rows,omitempty"`
}

func (s *Server) handleTerminalWebsocket(ws *websocket.Conn) {
	defer ws.Close()

	req := ws.Request()
	stableID := strings.TrimSpace(req.PathValue("stableID"))
	connectionID := strings.TrimSpace(req.URL.Query().Get("connectionId"))
	cols := parseUint32(req.URL.Query().Get("cols"), 120)
	rows := parseUint32(req.URL.Query().Get("rows"), 36)
	shell := req.URL.Query().Get("shell")

	var (
		session interface {
			Read([]byte) (int, error)
			Write([]byte) (int, error)
			Resize(uint32, uint32) error
			Close() error
		}
		err error
	)

	if connectionID != "" {
		session, err = s.service.OpenInteractiveSessionOnConnection(connectionID, cols, rows, shell)
	} else {
		session, err = s.service.OpenInteractiveSession(stableID, cols, rows, shell)
	}
	if err != nil {
		_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: err.Error()})
		return
	}
	defer session.Close()

	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := session.Read(buffer)
			if n > 0 {
				if sendErr := websocket.JSON.Send(ws, terminalMessage{Type: "output", Data: string(buffer[:n])}); sendErr != nil {
					return
				}
			}
			if err != nil {
				if err == io.EOF {
					_ = websocket.JSON.Send(ws, terminalMessage{Type: "status", Data: "closed"})
				} else {
					_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: err.Error()})
				}
				return
			}
		}
	}()

	for {
		var message terminalMessage
		if err := websocket.JSON.Receive(ws, &message); err != nil {
			return
		}

		switch message.Type {
		case "input":
			if _, err := session.Write([]byte(message.Data)); err != nil {
				_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: err.Error()})
				return
			}
		case "resize":
			if message.Cols > 0 && message.Rows > 0 {
				if err := session.Resize(message.Cols, message.Rows); err != nil {
					_ = websocket.JSON.Send(ws, terminalMessage{Type: "error", Data: err.Error()})
					return
				}
			}
		case "close":
			return
		}
	}
}

func parseUint32(raw string, fallback uint32) uint32 {
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return fallback
	}
	return uint32(value)
}
