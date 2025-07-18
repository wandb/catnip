package tui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"

	"github.com/gorilla/websocket"
)

type PTYClient struct {
	conn      *websocket.Conn
	sessionID string
	mu        sync.Mutex
	onMessage func([]byte)
	onError   func(error)
	done      chan struct{}
}

type ResizeMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func NewPTYClient(sessionID string) *PTYClient {
	return &PTYClient{
		sessionID: sessionID,
		done:      make(chan struct{}),
	}
}

func (p *PTYClient) Connect(baseURL string) error {
	u, err := url.Parse(baseURL)
	if err != nil {
		return err
	}

	// Change scheme to ws/wss
	//nolint:staticcheck // Simple if-else is clearer than switch for two cases
	if u.Scheme == "http" {
		u.Scheme = "ws"
	} else if u.Scheme == "https" {
		u.Scheme = "wss"
	}

	u.Path = "/v1/pty"
	q := u.Query()
	q.Set("session", p.sessionID)
	u.RawQuery = q.Encode()

	p.mu.Lock()
	defer p.mu.Unlock()

	p.conn, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect to PTY: %w", err)
	}

	// Start read loop
	go p.readLoop()

	return nil
}

func (p *PTYClient) readLoop() {
	defer close(p.done)

	for {
		messageType, message, err := p.conn.ReadMessage()
		if err != nil {
			// Log all errors for better debugging
			if p.onError != nil {
				p.onError(err)
			}
			return
		}

		if messageType == websocket.BinaryMessage || messageType == websocket.TextMessage {
			if p.onMessage != nil {
				p.onMessage(message)
			}
		}
	}
}

func (p *PTYClient) Send(data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn == nil {
		return fmt.Errorf("not connected")
	}

	return p.conn.WriteMessage(websocket.TextMessage, data)
}

func (p *PTYClient) Resize(cols, rows int) error {
	msg := ResizeMessage{
		Type: "resize",
		Cols: cols,
		Rows: rows,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return p.Send(data)
}

func (p *PTYClient) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn != nil {
		err := p.conn.Close()
		p.conn = nil
		return err
	}
	return nil
}

func (p *PTYClient) SetMessageHandler(handler func([]byte)) {
	p.onMessage = handler
}

func (p *PTYClient) SetErrorHandler(handler func(error)) {
	p.onError = handler
}

func (p *PTYClient) Wait() {
	<-p.done
}
