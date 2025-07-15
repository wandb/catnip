package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/vanpelt/catnip/internal/services"
)

// Event types that match the frontend TypeScript definitions
type EventType string

const (
	PortOpenedEvent     EventType = "port:opened"
	PortClosedEvent     EventType = "port:closed"
	GitDirtyEvent       EventType = "git:dirty"
	GitCleanEvent       EventType = "git:clean"
	ProcessStartedEvent EventType = "process:started"
	ProcessStoppedEvent EventType = "process:stopped"
	ContainerStatusEvent EventType = "container:status"
	HeartbeatEvent      EventType = "heartbeat"
)

type AppEvent struct {
	Type    EventType `json:"type"`
	Payload any       `json:"payload"`
}

type PortPayload struct {
	Port     int     `json:"port"`
	Service  *string `json:"service,omitempty"`
	Protocol *string `json:"protocol,omitempty"`
	Title    *string `json:"title,omitempty"`
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
	Status  string  `json:"status"`
	Message *string `json:"message,omitempty"`
}

type HeartbeatPayload struct {
	Timestamp int64 `json:"timestamp"`
	Uptime    int64 `json:"uptime"`
}

type SSEMessage struct {
	Event     AppEvent `json:"event"`
	Timestamp int64    `json:"timestamp"`
	ID        string   `json:"id"`
}

type EventsHandler struct {
	portMonitor  *services.PortMonitor
	gitService   *services.GitService
	clients      map[string]chan SSEMessage
	clientsMux   sync.RWMutex
	startTime    time.Time
	stopChan     chan bool
	lastPortCheck time.Time
	lastPortCheckMux sync.RWMutex
}

func NewEventsHandler(portMonitor *services.PortMonitor, gitService *services.GitService) *EventsHandler {
	h := &EventsHandler{
		portMonitor: portMonitor,
		gitService:  gitService,
		clients:     make(map[string]chan SSEMessage),
		startTime:   time.Now(),
		stopChan:    make(chan bool),
	}

	// Start listening for port changes
	go h.monitorPorts()
	
	// Start heartbeat
	go h.heartbeat()

	return h
}

// HandleSSE handles Server-Sent Events connections
// @Summary Server-Sent Events endpoint
// @Description Streams real-time events about ports, git status, processes, and container status
// @Tags events
// @Accept text/event-stream
// @Produce text/event-stream
// @Success 200 {string} string "SSE stream"
// @Router /v1/events [get]
func (h *EventsHandler) HandleSSE(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("Access-Control-Allow-Headers", "Cache-Control")
	c.Set("X-Accel-Buffering", "no") // Disable nginx buffering

	clientID := uuid.New().String()
	clientChan := make(chan SSEMessage, 100)
	
	log.Printf("SSE client connected: %s", clientID)
	
	h.clientsMux.Lock()
	h.clients[clientID] = clientChan
	h.clientsMux.Unlock()

	defer func() {
		log.Printf("SSE client disconnected: %s", clientID)
		h.clientsMux.Lock()
		delete(h.clients, clientID)
		// Don't close the channel here - let the StreamWriter handle it
		h.clientsMux.Unlock()
	}()

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		log.Printf("SSE stream writer started for client: %s", clientID)
		defer func() {
			log.Printf("SSE stream writer ended for client: %s", clientID)
			// Close the channel when StreamWriter actually ends
			close(clientChan)
		}()
		
		// Send initial container status
		containerStatusMsg := SSEMessage{
			Event: AppEvent{
				Type: ContainerStatusEvent,
				Payload: ContainerStatusPayload{
					Status:  "running",
					Message: nil,
				},
			},
			Timestamp: time.Now().UnixMilli(),
			ID:        uuid.New().String(),
		}
		
		data, err := json.Marshal(containerStatusMsg)
		if err == nil {
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			w.Flush()
		}
		
		// Send current port status
		services := h.portMonitor.GetServices()
		for _, serviceInfo := range services {
			var service *string
			var protocol *string
			var title *string
			if serviceInfo.ServiceType != "" {
				service = &serviceInfo.ServiceType
			}
			protocol = &serviceInfo.ServiceType // Use service type as protocol for now
			if serviceInfo.Title != "" {
				title = &serviceInfo.Title
			}

			portMsg := SSEMessage{
				Event: AppEvent{
					Type: PortOpenedEvent,
					Payload: PortPayload{
						Port:     serviceInfo.Port,
						Service:  service,
						Protocol: protocol,
						Title:    title,
					},
				},
				Timestamp: time.Now().UnixMilli(),
				ID:        uuid.New().String(),
			}
			
			data, err := json.Marshal(portMsg)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", string(data))
				w.Flush()
			}
		}
		
		// Send initial heartbeat
		heartbeatMsg := SSEMessage{
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
		
		data, err = json.Marshal(heartbeatMsg)
		if err == nil {
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			w.Flush()
		}
		
		log.Printf("Entering main streaming loop for client: %s", clientID)
		
		// Main streaming loop - no separate initial message processing
		for {
			select {
			case msg, ok := <-clientChan:
				if !ok {
					log.Printf("Client channel closed for: %s", clientID)
					return
				}
				
				// Additional validation to prevent empty messages
				if msg.Event.Type == "" {
					log.Printf("Skipping empty event type for client: %s", clientID)
					continue
				}
				
				log.Printf("Sending SSE message to client %s: %s", clientID, msg.Event.Type)
				
				data, err := json.Marshal(msg)
				if err != nil {
					log.Printf("Error marshaling SSE message: %v", err)
					continue
				}

				if _, err := fmt.Fprintf(w, "data: %s\n\n", string(data)); err != nil {
					log.Printf("Error writing SSE message: %v", err)
					return
				}
				
				if err := w.Flush(); err != nil {
					log.Printf("Error flushing SSE message: %v", err)
					return
				}
				
			case <-time.After(30 * time.Second):
				// Send keepalive
				log.Printf("Sending keepalive to client: %s", clientID)
				if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
					log.Printf("Error writing keepalive: %v", err)
					return
				}
				if err := w.Flush(); err != nil {
					log.Printf("Error flushing keepalive: %v", err)
					return
				}
			}
		}
	}))

	return nil
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
					var service *string
					var protocol *string
					var title *string
					if serviceInfo.ServiceType != "" {
						service = &serviceInfo.ServiceType
					}
					protocol = &serviceInfo.ServiceType // Use service type as protocol for now
					if serviceInfo.Title != "" {
						title = &serviceInfo.Title
					}

					log.Printf("Port opened: %d (%s) - %s", portNum, serviceInfo.ServiceType, serviceInfo.Title)
					h.broadcastEvent(AppEvent{
						Type: PortOpenedEvent,
						Payload: PortPayload{
							Port:     serviceInfo.Port,
							Service:  service,
							Protocol: protocol,
							Title:    title,
						},
					})
				}
			}

			// Check for closed ports
			for portNum := range lastPorts {
				if _, exists := currentPorts[portNum]; !exists {
					log.Printf("Port closed: %d", portNum)
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
		log.Printf("Warning: Attempting to broadcast event with empty type")
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
			// Client channel is full or closed, mark for removal
			clientsToRemove = append(clientsToRemove, clientID)
		}
	}
	h.clientsMux.RUnlock()

	// Remove disconnected clients
	if len(clientsToRemove) > 0 {
		h.clientsMux.Lock()
		for _, clientID := range clientsToRemove {
			if clientChan, exists := h.clients[clientID]; exists {
				close(clientChan)
				delete(h.clients, clientID)
			}
		}
		h.clientsMux.Unlock()
	}
}


// PublicAPI methods for other services to emit events
func (h *EventsHandler) EmitGitDirty(workspace string, files []string) {
	h.broadcastEvent(AppEvent{
		Type: GitDirtyEvent,
		Payload: GitPayload{
			Workspace: workspace,
			Files:     files,
		},
	})
}

func (h *EventsHandler) EmitGitClean(workspace string) {
	h.broadcastEvent(AppEvent{
		Type: GitCleanEvent,
		Payload: GitPayload{
			Workspace: workspace,
		},
	})
}

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

func (h *EventsHandler) EmitProcessStopped(pid int, exitCode int) {
	h.broadcastEvent(AppEvent{
		Type: ProcessStoppedEvent,
		Payload: ProcessStoppedPayload{
			PID:      pid,
			ExitCode: exitCode,
		},
	})
}

func (h *EventsHandler) EmitContainerStatus(status string, message *string) {
	h.broadcastEvent(AppEvent{
		Type: ContainerStatusEvent,
		Payload: ContainerStatusPayload{
			Status:  status,
			Message: message,
		},
	})
}

func (h *EventsHandler) heartbeat() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopChan:
			return
		case <-ticker.C:
			// Only send heartbeat if we have connected clients
			h.clientsMux.RLock()
			clientCount := len(h.clients)
			h.clientsMux.RUnlock()
			
			if clientCount > 0 {
				h.broadcastEvent(AppEvent{
					Type: HeartbeatEvent,
					Payload: HeartbeatPayload{
						Timestamp: time.Now().UnixMilli(),
						Uptime:    time.Since(h.startTime).Milliseconds(),
					},
				})
			}
		}
	}
}

// Stop stops the events handler and cleans up resources
func (h *EventsHandler) Stop() {
	close(h.stopChan)
	
	h.clientsMux.Lock()
	defer h.clientsMux.Unlock()
	
	for _, clientChan := range h.clients {
		close(clientChan)
	}
	h.clients = make(map[string]chan SSEMessage)
}