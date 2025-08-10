package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/models"
	"github.com/vanpelt/catnip/internal/services"
)

// EventType represents the type of event that can be sent via SSE
type EventType string

// Event type constants that match the frontend TypeScript definitions
const (
	PortOpenedEvent            EventType = "port:opened"
	PortClosedEvent            EventType = "port:closed"
	GitDirtyEvent              EventType = "git:dirty"
	GitCleanEvent              EventType = "git:clean"
	ProcessStartedEvent        EventType = "process:started"
	ProcessStoppedEvent        EventType = "process:stopped"
	ContainerStatusEvent       EventType = "container:status"
	PortMappedEvent            EventType = "port:mapped"
	HeartbeatEvent             EventType = "heartbeat"
	WorktreeStatusUpdatedEvent EventType = "worktree:status_updated"
	WorktreeBatchUpdatedEvent  EventType = "worktree:batch_updated"
	WorktreeDirtyEvent         EventType = "worktree:dirty"
	WorktreeCleanEvent         EventType = "worktree:clean"
	WorktreeUpdatedEvent       EventType = "worktree:updated"
	WorktreeCreatedEvent       EventType = "worktree:created"
	WorktreeDeletedEvent       EventType = "worktree:deleted"
	WorktreeTodosUpdatedEvent  EventType = "worktree:todos_updated"
	SessionTitleUpdatedEvent   EventType = "session:title_updated"
)

type AppEvent struct {
	Type    EventType `json:"type"`
	Payload any       `json:"payload"`
}

type PortPayload struct {
	Port       int     `json:"port"`
	Service    *string `json:"service,omitempty"`
	Protocol   *string `json:"protocol,omitempty"`
	Title      *string `json:"title,omitempty"`
	PID        *int    `json:"pid,omitempty"`
	Command    *string `json:"command,omitempty"`
	WorkingDir *string `json:"working_dir,omitempty"`
}

// PortMappedPayload describes a host mapping for a container port
type PortMappedPayload struct {
	Port     int `json:"port"`      // container port
	HostPort int `json:"host_port"` // mapped host port
}

type GitPayload struct {
	Workspace string   `json:"workspace"`
	Files     []string `json:"files,omitempty"`
}

type ProcessPayload struct {
	PID       int     `json:"pid"`
	Command   string  `json:"command"`
	Workspace *string `json:"workspace,omitempty"`
}

type ProcessStoppedPayload struct {
	PID      int `json:"pid"`
	ExitCode int `json:"exitCode"`
}

type ContainerStatusPayload struct {
	Status     string  `json:"status"`
	Message    *string `json:"message,omitempty"`
	SSHEnabled bool    `json:"sshEnabled"`
}

type HeartbeatPayload struct {
	Timestamp int64 `json:"timestamp"`
	Uptime    int64 `json:"uptime"`
}

type WorktreeStatusPayload struct {
	WorktreeID string                         `json:"worktree_id"`
	Status     *services.CachedWorktreeStatus `json:"status"`
}

type WorktreeBatchPayload struct {
	Updates map[string]*services.CachedWorktreeStatus `json:"updates"`
}

type WorktreeDirtyPayload struct {
	WorktreeID   string   `json:"worktree_id"`
	WorktreeName string   `json:"worktree_name"`
	Files        []string `json:"files,omitempty"`
}

type WorktreeUpdatedPayload struct {
	WorktreeID string                 `json:"worktree_id"`
	Updates    map[string]interface{} `json:"updates"`
}

type WorktreeCreatedPayload struct {
	Worktree interface{} `json:"worktree"`
}

type WorktreeDeletedPayload struct {
	WorktreeID   string `json:"worktree_id"`
	WorktreeName string `json:"worktree_name"`
}

type WorktreeTodosUpdatedPayload struct {
	WorktreeID string        `json:"worktree_id"`
	Todos      []models.Todo `json:"todos"`
}

type SessionTitleUpdatedPayload struct {
	WorkspaceDir        string              `json:"workspace_dir"`
	WorktreeID          string              `json:"worktree_id,omitempty"`
	SessionTitle        *models.TitleEntry  `json:"session_title"`
	SessionTitleHistory []models.TitleEntry `json:"session_title_history"`
}

type ClaudeActivityStateChangedPayload struct {
	WorktreePath string                     `json:"worktree_path"`
	State        models.ClaudeActivityState `json:"state"`
}

type SSEMessage struct {
	Event     AppEvent `json:"event"`
	Timestamp int64    `json:"timestamp"`
	ID        string   `json:"id"`
}

type EventsHandler struct {
	portMonitor        *services.PortMonitor
	gitService         *services.GitService
	clients            map[string]chan SSEMessage
	clientsMux         sync.RWMutex
	clientConnectTimes map[string]time.Time
	startTime          time.Time
	stopChan           chan bool
	lastPortCheck      time.Time
	lastPortCheckMux   sync.RWMutex
	// host port mappings for container ports
	portMappings   map[int]int
	portMappingMux sync.RWMutex
}

func NewEventsHandler(portMonitor *services.PortMonitor, gitService *services.GitService) *EventsHandler {
	h := &EventsHandler{
		portMonitor:        portMonitor,
		gitService:         gitService,
		clients:            make(map[string]chan SSEMessage),
		clientConnectTimes: make(map[string]time.Time),
		startTime:          time.Now(),
		stopChan:           make(chan bool),
		portMappings:       make(map[int]int),
	}

	// Start listening for port changes
	go h.monitorPorts()

	return h
}

// HandleSSE handles Server-Sent Events connections
// @Summary Server-Sent Events endpoint for real-time container events
// @Description Streams real-time events in Server-Sent Events format. Events include port changes, git status, processes, and container status updates.
// @Description
// @Description ## Event Types
// @Description
// @Description ### Port Events
// @Description - **port:opened**: Fired when a new port is detected
// @Description   - `port` (int): Port number
// @Description   - `service` (string): Service type (http, tcp)
// @Description   - `protocol` (string): Protocol used
// @Description   - `title` (string): Service title/name if detected
// @Description - **port:closed**: Fired when a port is no longer available
// @Description   - `port` (int): Port number that was closed
// @Description
// @Description ### Git Events
// @Description - **git:dirty**: Fired when git workspace has uncommitted changes
// @Description   - `workspace` (string): Workspace path
// @Description   - `files` ([]string): List of modified files
// @Description - **git:clean**: Fired when git workspace becomes clean
// @Description   - `workspace` (string): Workspace path
// @Description
// @Description ### Process Events
// @Description - **process:started**: Fired when a new process starts
// @Description   - `pid` (int): Process ID
// @Description   - `command` (string): Command that was executed
// @Description   - `workspace` (string): Workspace where process started
// @Description - **process:stopped**: Fired when a process terminates
// @Description   - `pid` (int): Process ID that stopped
// @Description   - `exitCode` (int): Exit code of the process
// @Description
// @Description ### Container Events
// @Description - **container:status**: Fired when container status changes
// @Description   - `status` (string): Container status (running, stopped, error)
// @Description   - `message` (string): Optional status message
// @Description
// @Description ### System Events
// @Description - **heartbeat**: Sent every 5 seconds to keep connection alive
// @Description   - `timestamp` (int64): Current timestamp in milliseconds
// @Description   - `uptime` (int64): Server uptime in milliseconds
// @Description
// @Description ## Message Format
// @Description Each SSE message is a JSON object with:
// @Description - `event`: Event object containing `type` and `payload`
// @Description - `timestamp`: Event timestamp in milliseconds
// @Description - `id`: Unique event identifier
// @Description
// @Description ## Connection Behavior
// @Description - Auto-reconnects on disconnection
// @Description - Sends current state on initial connection
// @Description - Heartbeat every 5 seconds
// @Description - Rate limited to prevent spam
// @Tags events
// @Accept text/event-stream
// @Produce text/event-stream
// @Success 200 {object} SSEMessage "SSE stream of events"
// @Router /v1/events [get]
// HandleSSE streams container / port / git / process events to the browser.
// GET /v1/events
func (h *EventsHandler) HandleSSE(c *fiber.Ctx) error {
	// --------------------------------------------------------------------
	// 1.  Reject non-SSE clients up-front
	// --------------------------------------------------------------------
	if ah := c.Get("Accept"); ah != "" && !strings.Contains(ah, "text/event-stream") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "This endpoint only accepts Server-Sent Events (text/event-stream)",
		})
	}

	//--------------------------------------------------------------------
	// 2.  Prepare HTTP headers once
	//--------------------------------------------------------------------
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no") // disable nginx buffering

	//--------------------------------------------------------------------
	// 3.  Register client
	//--------------------------------------------------------------------
	clientID := uuid.New().String()
	clientType := c.Query("client", "unknown")
	ch := make(chan SSEMessage, 100)

	h.addClient(clientID, ch)
	logger.Infof("SSE client connected: %s (%s) from %s", clientID, clientType, c.IP())

	//--------------------------------------------------------------------
	// 4.  Stream writer
	//--------------------------------------------------------------------
	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		flushOrDie := func() bool {
			// ??? || c.Context().IsConnectionClosed()
			if err := w.Flush(); err != nil {
				return false
			}
			return true
		}

		send := func(msg SSEMessage) bool {
			if msg.Event.Type == "" { // guard against empty events
				return true
			}
			b, _ := json.Marshal(msg)
			if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
				return false
			}
			return flushOrDie()
		}

		// ---------------- initial state ----------------
		if !send(h.makeHeartbeat()) {
			return
		}
		if !send(h.makeContainerStatus()) {
			return
		}
		for _, p := range h.portMonitor.GetServices() {
			if !send(h.makePortOpened(p)) {
				return
			}
		}

		// Send current host port mappings
		h.portMappingMux.RLock()
		for cport, hport := range h.portMappings {
			msg := SSEMessage{
				Event: AppEvent{
					Type: PortMappedEvent,
					Payload: PortMappedPayload{
						Port:     cport,
						HostPort: hport,
					},
				},
				Timestamp: time.Now().UnixMilli(),
				ID:        uuid.New().String(),
			}
			if !send(msg) {
				h.portMappingMux.RUnlock()
				return
			}
		}
		h.portMappingMux.RUnlock()

		// ---------------- main loop --------------------
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()

		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					logger.Warnf("Event client %s is closed somehow!", clientID)
				}
				if !ok || !send(msg) {
					return
				}
			case <-tick.C:
				send(h.makeHeartbeat())
				if !flushOrDie() {
					return
				}
			}
		}
	}))

	return nil
}

func (h *EventsHandler) addClient(id string, ch chan SSEMessage) {
	h.clientsMux.Lock()
	h.clients[id] = ch
	h.clientConnectTimes[id] = time.Now()
	logger.Debugf("Added event client %s", id)
	h.clientsMux.Unlock()
}

func (h *EventsHandler) removeClient(id string) {
	h.clientsMux.Lock()
	if ch, ok := h.clients[id]; ok {
		logger.Debugf("Removing eventclient %s", id)
		close(ch)
		delete(h.clients, id)
	}
	delete(h.clientConnectTimes, id)
	h.clientsMux.Unlock()
}

// --- small builders to keep main handler tiny ---
func (h *EventsHandler) makeHeartbeat() SSEMessage {
	return SSEMessage{
		Event: AppEvent{
			Type: HeartbeatEvent,
			Payload: HeartbeatPayload{
				Timestamp: time.Now().UnixMilli(),
				Uptime:    time.Since(h.startTime).Milliseconds(),
			},
		},
		Timestamp: time.Now().UnixMilli(),
		ID:        uuid.New().String(),
	}
}

func (h *EventsHandler) makeContainerStatus() SSEMessage {
	sshEnabled := os.Getenv("CATNIP_SSH_ENABLED") == "true"
	return SSEMessage{
		Event: AppEvent{
			Type: ContainerStatusEvent,
			Payload: ContainerStatusPayload{
				Status:     "running",
				SSHEnabled: sshEnabled,
			},
		},
		Timestamp: time.Now().UnixMilli(),
		ID:        uuid.New().String(),
	}
}

func (h *EventsHandler) makePortOpened(s *services.ServiceInfo) SSEMessage {
	// fill optional pointers exactly as before
	return SSEMessage{
		Event: AppEvent{
			Type: PortOpenedEvent,
			Payload: PortPayload{
				Port:     s.Port,
				Service:  &s.ServiceType,
				Protocol: &s.ServiceType,
				Title: func() *string {
					if s.Title != "" {
						return &s.Title
					}
					return nil
				}(),
				PID: func() *int {
					if s.PID != 0 {
						return &s.PID
					}
					return nil
				}(),
				Command: func() *string {
					if s.Command != "" {
						return &s.Command
					}
					return nil
				}(),
				WorkingDir: func() *string {
					if s.WorkingDir != "" {
						return &s.WorkingDir
					}
					return nil
				}(),
			},
		},
		Timestamp: time.Now().UnixMilli(),
		ID:        uuid.New().String(),
	}
}

func (h *EventsHandler) monitorPorts() {
	ticker := time.NewTicker(2 * time.Second) // Reduced frequency
	defer ticker.Stop()

	lastPorts := make(map[int]*services.ServiceInfo)

	for {
		select {
		case <-h.stopChan:
			return
		case <-ticker.C:
			// Rate limit port checking
			h.lastPortCheckMux.RLock()
			timeSinceLastCheck := time.Since(h.lastPortCheck)
			h.lastPortCheckMux.RUnlock()

			if timeSinceLastCheck < 1*time.Second {
				continue
			}

			h.lastPortCheckMux.Lock()
			h.lastPortCheck = time.Now()
			h.lastPortCheckMux.Unlock()

			currentPorts := h.portMonitor.GetServices()

			// Check for new ports
			for portNum, serviceInfo := range currentPorts {
				if _, exists := lastPorts[portNum]; !exists {
					logger.Debugf("Port opened: %d (%s) - %s [PID: %d, Command: %s, Dir: %s]", portNum, serviceInfo.ServiceType, serviceInfo.Title, serviceInfo.PID, serviceInfo.Command, serviceInfo.WorkingDir)
					h.broadcastEvent(h.makePortOpened(serviceInfo).Event)
				}
			}

			// Check for closed ports
			for portNum := range lastPorts {
				if _, exists := currentPorts[portNum]; !exists {
					logger.Debugf("Port closed: %d", portNum)
					h.broadcastEvent(AppEvent{
						Type: PortClosedEvent,
						Payload: PortPayload{
							Port: portNum,
						},
					})
				}
			}

			// Copy current ports to last ports
			lastPorts = make(map[int]*services.ServiceInfo, len(currentPorts))
			maps.Copy(lastPorts, currentPorts)
		}
	}
}

func (h *EventsHandler) broadcastEvent(event AppEvent) {
	// Validate event before broadcasting
	if event.Type == "" {
		logger.Warnf("Attempting to broadcast event with empty type")
		return
	}

	message := SSEMessage{
		Event:     event,
		Timestamp: time.Now().UnixMilli(),
		ID:        uuid.New().String(),
	}

	h.clientsMux.RLock()
	clientsToRemove := []string{}

	for clientID, clientChan := range h.clients {
		select {
		case clientChan <- message:
		default:
			// Check if client is in grace period (first 2 seconds after connection)
			connectTime, exists := h.clientConnectTimes[clientID]
			gracePeriod := 2 * time.Second

			if exists && time.Since(connectTime) < gracePeriod {
				// Client is in grace period, don't remove yet
				logger.Debugf("Client %s in grace period, not removing (connected %v ago)", clientID, time.Since(connectTime))
			} else {
				// Client channel is full or closed, mark for removal
				clientsToRemove = append(clientsToRemove, clientID)
			}
		}
	}
	h.clientsMux.RUnlock()

	// Remove disconnected clients
	if len(clientsToRemove) > 0 {
		for _, clientID := range clientsToRemove {
			h.removeClient(clientID)
		}
	}
}

// SetPortMapping records and broadcasts a host mapping for a container port
func (h *EventsHandler) SetPortMapping(containerPort, hostPort int) {
	h.portMappingMux.Lock()
	h.portMappings[containerPort] = hostPort
	h.portMappingMux.Unlock()

	h.broadcastEvent(AppEvent{
		Type: PortMappedEvent,
		Payload: PortMappedPayload{
			Port:     containerPort,
			HostPort: hostPort,
		},
	})
}

// ClearPortMapping removes mapping and broadcasts update (hostPort=0 means cleared)
func (h *EventsHandler) ClearPortMapping(containerPort int) {
	h.portMappingMux.Lock()
	delete(h.portMappings, containerPort)
	h.portMappingMux.Unlock()

	h.broadcastEvent(AppEvent{
		Type: PortMappedEvent,
		Payload: PortMappedPayload{
			Port:     containerPort,
			HostPort: 0,
		},
	})
}

// EmitGitDirty broadcasts a git dirty event to all connected clients
func (h *EventsHandler) EmitGitDirty(workspace string, files []string) {
	h.broadcastEvent(AppEvent{
		Type: GitDirtyEvent,
		Payload: GitPayload{
			Workspace: workspace,
			Files:     files,
		},
	})
}

// EmitGitClean broadcasts a git clean event to all connected clients
func (h *EventsHandler) EmitGitClean(workspace string) {
	h.broadcastEvent(AppEvent{
		Type: GitCleanEvent,
		Payload: GitPayload{
			Workspace: workspace,
		},
	})
}

// EmitProcessStarted broadcasts a process started event to all connected clients
func (h *EventsHandler) EmitProcessStarted(pid int, command string, workspace *string) {
	h.broadcastEvent(AppEvent{
		Type: ProcessStartedEvent,
		Payload: ProcessPayload{
			PID:       pid,
			Command:   command,
			Workspace: workspace,
		},
	})
}

// EmitProcessStopped broadcasts a process stopped event to all connected clients
func (h *EventsHandler) EmitProcessStopped(pid int, exitCode int) {
	h.broadcastEvent(AppEvent{
		Type: ProcessStoppedEvent,
		Payload: ProcessStoppedPayload{
			PID:      pid,
			ExitCode: exitCode,
		},
	})
}

// EmitContainerStatus broadcasts a container status event to all connected clients
func (h *EventsHandler) EmitContainerStatus(status string, message *string) {
	sshEnabled := os.Getenv("CATNIP_SSH_ENABLED") == "true"
	h.broadcastEvent(AppEvent{
		Type: ContainerStatusEvent,
		Payload: ContainerStatusPayload{
			Status:     status,
			Message:    message,
			SSHEnabled: sshEnabled,
		},
	})
}

// EmitWorktreeStatusUpdated broadcasts a single worktree status update to all connected clients
func (h *EventsHandler) EmitWorktreeStatusUpdated(worktreeID string, status *services.CachedWorktreeStatus) {
	h.broadcastEvent(AppEvent{
		Type: WorktreeStatusUpdatedEvent,
		Payload: WorktreeStatusPayload{
			WorktreeID: worktreeID,
			Status:     status,
		},
	})
}

// EmitWorktreeBatchUpdated broadcasts multiple worktree status updates to all connected clients
func (h *EventsHandler) EmitWorktreeBatchUpdated(updates map[string]*services.CachedWorktreeStatus) {
	h.broadcastEvent(AppEvent{
		Type: WorktreeBatchUpdatedEvent,
		Payload: WorktreeBatchPayload{
			Updates: updates,
		},
	})
}

// EmitWorktreeDirty broadcasts a worktree dirty event to all connected clients
func (h *EventsHandler) EmitWorktreeDirty(worktreeID, worktreeName string, files []string) {
	h.broadcastEvent(AppEvent{
		Type: WorktreeDirtyEvent,
		Payload: WorktreeDirtyPayload{
			WorktreeID:   worktreeID,
			WorktreeName: worktreeName,
			Files:        files,
		},
	})
}

// EmitWorktreeClean broadcasts a worktree clean event to all connected clients
func (h *EventsHandler) EmitWorktreeClean(worktreeID, worktreeName string) {
	h.broadcastEvent(AppEvent{
		Type: WorktreeCleanEvent,
		Payload: WorktreeDirtyPayload{
			WorktreeID:   worktreeID,
			WorktreeName: worktreeName,
		},
	})
}

// EmitWorktreeUpdated broadcasts a worktree updated event to all connected clients
func (h *EventsHandler) EmitWorktreeUpdated(worktreeID string, updates map[string]interface{}) {
	h.broadcastEvent(AppEvent{
		Type: WorktreeUpdatedEvent,
		Payload: WorktreeUpdatedPayload{
			WorktreeID: worktreeID,
			Updates:    updates,
		},
	})
}

// EmitWorktreeCreated broadcasts a worktree created event to all connected clients
func (h *EventsHandler) EmitWorktreeCreated(worktree *models.Worktree) {
	h.broadcastEvent(AppEvent{
		Type: WorktreeCreatedEvent,
		Payload: WorktreeCreatedPayload{
			Worktree: worktree,
		},
	})
}

// EmitWorktreeDeleted broadcasts a worktree deleted event to all connected clients
func (h *EventsHandler) EmitWorktreeDeleted(worktreeID, worktreeName string) {
	h.broadcastEvent(AppEvent{
		Type: WorktreeDeletedEvent,
		Payload: WorktreeDeletedPayload{
			WorktreeID:   worktreeID,
			WorktreeName: worktreeName,
		},
	})
}

// EmitWorktreeTodosUpdated broadcasts a worktree todos updated event to all connected clients
func (h *EventsHandler) EmitWorktreeTodosUpdated(worktreeID string, todos []models.Todo) {
	h.broadcastEvent(AppEvent{
		Type: WorktreeTodosUpdatedEvent,
		Payload: WorktreeTodosUpdatedPayload{
			WorktreeID: worktreeID,
			Todos:      todos,
		},
	})
}

// EmitSessionTitleUpdated broadcasts a session title updated event to all connected clients
func (h *EventsHandler) EmitSessionTitleUpdated(workspaceDir, worktreeID string, sessionTitle *models.TitleEntry, sessionTitleHistory []models.TitleEntry) {
	h.broadcastEvent(AppEvent{
		Type: SessionTitleUpdatedEvent,
		Payload: SessionTitleUpdatedPayload{
			WorkspaceDir:        workspaceDir,
			WorktreeID:          worktreeID,
			SessionTitle:        sessionTitle,
			SessionTitleHistory: sessionTitleHistory,
		},
	})
}

// EmitClaudeActivityStateChanged broadcasts a Claude activity state changed event to all connected clients
func (h *EventsHandler) EmitClaudeActivityStateChanged(worktreePath string, state models.ClaudeActivityState) {
	h.broadcastEvent(AppEvent{
		Type: ClaudeActivityStateChangedEvent,
		Payload: ClaudeActivityStateChangedPayload{
			WorktreePath: worktreePath,
			State:        state,
		},
	})
}

// Stop stops the events handler and cleans up resources
func (h *EventsHandler) Stop() {
	close(h.stopChan)
	logger.Info("Stopping events handler...")
	h.clientsMux.Lock()
	defer h.clientsMux.Unlock()

	for _, clientChan := range h.clients {
		close(clientChan)
	}
	h.clients = make(map[string]chan SSEMessage)
	h.clientConnectTimes = make(map[string]time.Time)
}
