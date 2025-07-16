package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// SSEClient handles Server-Sent Events connections
type SSEClient struct {
	url      string
	program  *tea.Program
	stopChan chan struct{}
}

// SSEMessage represents Server-Sent Events message types matching the server
type SSEMessage struct {
	Event     AppEvent `json:"event"`
	Timestamp int64    `json:"timestamp"`
	ID        string   `json:"id"`
}

type AppEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// Event type constants
const (
	PortOpenedEvent     = "port:opened"
	PortClosedEvent     = "port:closed"
	GitDirtyEvent       = "git:dirty"
	GitCleanEvent       = "git:clean"
	ProcessStartedEvent = "process:started"
	ProcessStoppedEvent = "process:stopped"
	ContainerStatusEvent = "container:status"
	HeartbeatEvent      = "heartbeat"
)

// SSE event messages for the TUI
type ssePortOpenedMsg struct {
	port int
	service string
}

type ssePortClosedMsg struct {
	port int
}

type sseContainerStatusMsg struct {
	status string
	message string
}

type sseErrorMsg struct {
	err error
}

type sseConnectedMsg struct{}
type sseDisconnectedMsg struct{}

// NewSSEClient creates a new SSE client
func NewSSEClient(url string, program *tea.Program) *SSEClient {
	return &SSEClient{
		url:      url,
		program:  program,
		stopChan: make(chan struct{}),
	}
}

// Start begins listening for SSE events
func (c *SSEClient) Start() {
	go c.connect()
}

// Stop closes the SSE connection
func (c *SSEClient) Stop() {
	close(c.stopChan)
}

func (c *SSEClient) connect() {
	for {
		select {
		case <-c.stopChan:
			return
		default:
			if err := c.handleConnection(); err != nil {
				debugLog("SSE connection error: %v", err)
				if c.program != nil {
					c.program.Send(sseErrorMsg{err: err})
				}
				// Wait before reconnecting
				time.Sleep(5 * time.Second)
			}
		}
	}
}

func (c *SSEClient) handleConnection() error {
	client := &http.Client{
		Timeout: 0, // No timeout for SSE
	}

	req, err := http.NewRequest("GET", c.url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to SSE: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SSE connection failed: %s", resp.Status)
	}

	// Notify connected
	if c.program != nil {
		c.program.Send(sseConnectedMsg{})
	}

	scanner := bufio.NewScanner(resp.Body)
	var eventData strings.Builder

	for scanner.Scan() {
		select {
		case <-c.stopChan:
			return nil
		default:
			line := scanner.Text()

			if strings.HasPrefix(line, "data: ") {
				eventData.WriteString(strings.TrimPrefix(line, "data: "))
			} else if line == "" && eventData.Len() > 0 {
				// End of event, process it
				c.processEvent(eventData.String())
				eventData.Reset()
			} else if strings.HasPrefix(line, ": keepalive") { //nolint:staticcheck // HasPrefix is being used correctly
				// Ignore keepalive messages
				continue
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading SSE stream: %w", err)
	}

	// Notify disconnected
	if c.program != nil {
		c.program.Send(sseDisconnectedMsg{})
	}

	return nil
}

func (c *SSEClient) processEvent(data string) {
	var msg SSEMessage
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		debugLog("Failed to parse SSE message: %v", err)
		return
	}

	// Convert payload to appropriate type based on event type
	switch msg.Event.Type {
	case PortOpenedEvent:
		if payload, ok := msg.Event.Payload.(map[string]interface{}); ok {
			portFloat, _ := payload["port"].(float64)
			service := ""
			if svc, ok := payload["service"].(string); ok {
				service = svc
			}
			
			if c.program != nil {
				c.program.Send(ssePortOpenedMsg{
					port:    int(portFloat),
					service: service,
				})
			}
		}

	case PortClosedEvent:
		if payload, ok := msg.Event.Payload.(map[string]interface{}); ok {
			portFloat, _ := payload["port"].(float64)
			
			if c.program != nil {
				c.program.Send(ssePortClosedMsg{
					port: int(portFloat),
				})
			}
		}

	case ContainerStatusEvent:
		if payload, ok := msg.Event.Payload.(map[string]interface{}); ok {
			status, _ := payload["status"].(string)
			message := ""
			if msg, ok := payload["message"].(string); ok {
				message = msg
			}
			
			if c.program != nil {
				c.program.Send(sseContainerStatusMsg{
					status:  status,
					message: message,
				})
			}
		}

	case HeartbeatEvent:
		// Could use this to update uptime or connection status
		// For now, just log it
		debugLog("SSE heartbeat received")

	default:
		// Log other event types for now
		debugLog("SSE event received: %s", msg.Event.Type)
	}
}