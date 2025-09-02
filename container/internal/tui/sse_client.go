package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// SSEClient handles Server-Sent Events connections
type SSEClient struct {
	url                    string
	program                *tea.Program
	httpClient             *http.Client // Optional HTTP client for authenticated requests
	stopChan               chan struct{}
	connected              bool
	onEvent                func(AppEvent)
	onWorktreeUpdateWithID func(worktreeID string, updates map[string]interface{}) // Callback for worktree updates with ID
	onWorktreeCreated      func(worktree map[string]interface{})                   // Callback for worktree creation
	onConnected            func()                                                  // Callback when connection is established
	notificationHistory    map[string]time.Time                                    // Title -> timestamp for deduplication
	notificationHistoryMu  sync.RWMutex
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
	PortOpenedEvent           = "port:opened"
	PortClosedEvent           = "port:closed"
	GitDirtyEvent             = "git:dirty"
	GitCleanEvent             = "git:clean"
	ProcessStartedEvent       = "process:started"
	ProcessStoppedEvent       = "process:stopped"
	ContainerStatusEvent      = "container:status"
	PortMappedEvent           = "port:mapped"
	HeartbeatEvent            = "heartbeat"
	NotificationEvent         = "notification:show"
	WorktreeUpdatedEvent      = "worktree:updated"
	WorktreeBatchUpdatedEvent = "worktree:batch_updated"
	WorktreeCreatedEvent      = "worktree:created"
)

// SSE event messages are defined in messages.go

// NewSSEClient creates a new SSE client
func NewSSEClient(url string, program *tea.Program) *SSEClient {
	return &SSEClient{
		url:                 url,
		program:             program,
		httpClient:          nil, // Will be set later if needed
		stopChan:            make(chan struct{}),
		notificationHistory: make(map[string]time.Time),
	}
}

// SetHTTPClient sets a custom HTTP client for authenticated requests
func (c *SSEClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
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
	retryCount := 0
	for {
		select {
		case <-c.stopChan:
			return
		default:
			if err := c.handleConnection(); err != nil {
				debugLog("TUI SSE: Connection error (attempt %d): %v", retryCount+1, err)
				if c.program != nil {
					c.program.Send(sseErrorMsg{err: err})
				}
				// Only mark as disconnected if we were previously connected
				if c.connected {
					c.connected = false
					debugLog("TUI SSE: Marking as disconnected")
					if c.program != nil {
						c.program.Send(sseDisconnectedMsg{})
					}
				}
				// Exponential backoff with max delay of 30 seconds
				retryCount++
				delay := time.Duration(retryCount) * 2 * time.Second
				if delay > 30*time.Second {
					delay = 30 * time.Second
				}
				debugLog("TUI SSE: Retrying in %v", delay)
				time.Sleep(delay)
			} else {
				// Reset retry count on successful connection
				debugLog("TUI SSE: handleConnection returned without error")
				retryCount = 0
			}
		}
	}
}

func (c *SSEClient) handleConnection() error {
	client := c.httpClient
	if client == nil {
		client = &http.Client{
			// No timeout for SSE connections - they should be long-lived
		}
	}

	// Add query parameter to identify TUI client
	sseURL := c.url + "?client=tui"
	debugLog("TUI SSE: Attempting connection to %s", sseURL)

	req, err := http.NewRequest("GET", sseURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		debugLog("TUI SSE: Connection failed: %v", err)
		return fmt.Errorf("connecting to SSE: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		debugLog("TUI SSE: Bad status code: %s", resp.Status)
		return fmt.Errorf("SSE connection failed: %s", resp.Status)
	}

	debugLog("TUI SSE: Successfully connected to SSE endpoint")

	// Notify connected only if we weren't already connected
	if !c.connected {
		c.connected = true
		debugLog("TUI SSE: Successfully connected")
		if c.program != nil {
			c.program.Send(sseConnectedMsg{})
		}
		// Call onConnected callback if set
		if c.onConnected != nil {
			c.onConnected()
		}
	} else {
		debugLog("TUI SSE: Already connected, continuing stream")
	}

	scanner := bufio.NewScanner(resp.Body)
	var eventData strings.Builder

	debugLog("TUI SSE: Starting to read from stream")
	for scanner.Scan() {
		select {
		case <-c.stopChan:
			return nil
		default:
			line := scanner.Text()
			debugLog("TUI SSE: Received line: %s", line)

			if strings.HasPrefix(line, "data: ") {
				eventData.WriteString(strings.TrimPrefix(line, "data: "))
			} else if line == "" && eventData.Len() > 0 {
				// End of event, process it
				debugLog("TUI SSE: Processing event data: %s", eventData.String())
				c.processEvent(eventData.String())
				eventData.Reset()
			} else if strings.HasPrefix(line, ": ") || strings.HasPrefix(line, "event: ") || strings.HasPrefix(line, "id: ") { //nolint:staticcheck
				// Ignore SSE metadata lines (comments, event types, IDs)
				continue
			}
		}
	}

	if err := scanner.Err(); err != nil {
		debugLog("TUI SSE: Scanner error: %v", err)
		return fmt.Errorf("reading SSE stream: %w", err)
	}

	// Connection ended normally (server closed or network issue)
	debugLog("TUI SSE: Stream ended normally")
	return fmt.Errorf("SSE stream ended")
}

func (c *SSEClient) processEvent(data string) {
	var msg SSEMessage
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		debugLog("TUI SSE: Failed to parse SSE message: %v", err)
		return
	}

	debugLog("TUI SSE: Received event: %s", msg.Event.Type)

	// Notify external event hook if configured
	if c.onEvent != nil {
		// Non-blocking, avoid deadlocks
		go c.onEvent(msg.Event)
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
			title := ""
			if t, ok := payload["title"].(string); ok {
				title = t
			}
			protocol := ""
			if p, ok := payload["protocol"].(string); ok {
				protocol = p
			}

			if c.program != nil {
				c.program.Send(ssePortOpenedMsg{
					port:     int(portFloat),
					service:  service,
					title:    title,
					protocol: protocol,
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
	case PortMappedEvent:
		if payload, ok := msg.Event.Payload.(map[string]interface{}); ok {
			// We do not need to surface mappings in TUI for now; ignore
			_ = payload
		}
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
		// Heartbeat confirms connection is still alive
		// No need to log every heartbeat to avoid spam
		// debugLog("SSE heartbeat received")

	case NotificationEvent:
		if payload, ok := msg.Event.Payload.(map[string]interface{}); ok {
			title, _ := payload["title"].(string)
			body, _ := payload["body"].(string)
			subtitle, _ := payload["subtitle"].(string)
			url, _ := payload["url"].(string)

			// Send native notification if supported
			if IsNotificationSupported() {
				// Check for duplicate notifications (5-second window per title)
				c.notificationHistoryMu.Lock()
				now := time.Now()
				if lastSent, exists := c.notificationHistory[title]; exists && now.Sub(lastSent) < 5*time.Second {
					debugLog("TUI SSE: Skipping duplicate notification: %s", title)
					c.notificationHistoryMu.Unlock()
					return
				}

				// Clean up old entries (older than 5 seconds) to prevent memory leak
				for oldTitle, timestamp := range c.notificationHistory {
					if now.Sub(timestamp) >= 5*time.Second {
						delete(c.notificationHistory, oldTitle)
					}
				}

				// Record this notification
				c.notificationHistory[title] = now
				c.notificationHistoryMu.Unlock()

				// Send notification async for server usage (no wait needed)
				if err := SendNativeNotificationAsync(title, body, subtitle, url); err != nil {
					debugLog("TUI SSE: Failed to send notification: %v", err)
				} else {
					debugLog("TUI SSE: Sent notification: %s", title)
				}
			}
		}

	case WorktreeUpdatedEvent:
		// Single worktree update
		if payload, ok := msg.Event.Payload.(map[string]interface{}); ok {
			worktreeID, _ := payload["worktree_id"].(string)
			if updates, ok := payload["updates"].(map[string]interface{}); ok {
				activityState, _ := updates["claude_activity_state"].(string)

				debugLog("TUI SSE: Worktree updated: %s -> %s", worktreeID, activityState)

				// Call the new worktree update callback with ID and updates
				if c.onWorktreeUpdateWithID != nil {
					c.onWorktreeUpdateWithID(worktreeID, updates)
				}
			}
		}

	case WorktreeBatchUpdatedEvent:
		// Multiple worktrees updated - handle each one individually
		if payload, ok := msg.Event.Payload.(map[string]interface{}); ok {
			if updates, ok := payload["updates"].(map[string]interface{}); ok {
				debugLog("TUI SSE: Batch worktree update: %d worktrees", len(updates))

				// Process each worktree update individually
				for worktreeID, updateData := range updates {
					if updateMap, ok := updateData.(map[string]interface{}); ok {
						if c.onWorktreeUpdateWithID != nil {
							c.onWorktreeUpdateWithID(worktreeID, updateMap)
						}
					}
				}
			}
		}

	case WorktreeCreatedEvent:
		// New worktree created
		if payload, ok := msg.Event.Payload.(map[string]interface{}); ok {
			if worktree, ok := payload["worktree"].(map[string]interface{}); ok {
				worktreeID, _ := worktree["id"].(string)
				worktreePath, _ := worktree["path"].(string)

				debugLog("TUI SSE: Worktree created: %s -> %s", worktreeID, worktreePath)

				// Call the worktree created callback
				if c.onWorktreeCreated != nil {
					c.onWorktreeCreated(worktree)
				}
			}
		}

	default:
		// Log other event types for now
		debugLog("SSE event received: %s", msg.Event.Type)
	}
}
