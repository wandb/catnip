package services

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

// PortAllocationService manages port allocation for PTY sessions
type PortAllocationService struct {
	// Map of session ID to allocated ports
	allocatedPorts map[string]*SessionPorts
	// Track which ports are in use
	usedPorts map[int]bool
	// Start port range for allocation
	startPort int
	// Mutex to protect concurrent access
	mu sync.RWMutex
}

// SessionPorts represents the ports allocated to a session
type SessionPorts struct {
	SessionID string `json:"session_id"`
	PORT      int    `json:"port"`  // Main port for the session
	PORTZ     []int  `json:"portz"` // Additional ports array
}

// NewPortAllocationService creates a new port allocation service
func NewPortAllocationService() *PortAllocationService {
	return &PortAllocationService{
		allocatedPorts: make(map[string]*SessionPorts),
		usedPorts:      make(map[int]bool),
		startPort:      3000, // Start allocating from port 3000
	}
}

// AllocatePortsForSession allocates ports for a session
func (p *PortAllocationService) AllocatePortsForSession(sessionID string) (*SessionPorts, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if session already has ports allocated
	if existing, exists := p.allocatedPorts[sessionID]; exists {
		return existing, nil
	}

	// Allocate main PORT
	mainPort, err := p.findAvailablePort()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate main port: %v", err)
	}
	p.usedPorts[mainPort] = true

	// Allocate 6 additional ports for PORTZ
	portz := make([]int, 6)
	for i := 0; i < 6; i++ {
		port, err := p.findAvailablePort()
		if err != nil {
			// Clean up already allocated ports
			p.usedPorts[mainPort] = false
			for j := 0; j < i; j++ {
				p.usedPorts[portz[j]] = false
			}
			return nil, fmt.Errorf("failed to allocate port %d for PORTZ: %v", i+1, err)
		}
		portz[i] = port
		p.usedPorts[port] = true
	}

	sessionPorts := &SessionPorts{
		SessionID: sessionID,
		PORT:      mainPort,
		PORTZ:     portz,
	}

	p.allocatedPorts[sessionID] = sessionPorts
	return sessionPorts, nil
}

// ReleasePortsForSession releases ports allocated to a session
func (p *PortAllocationService) ReleasePortsForSession(sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	ports, exists := p.allocatedPorts[sessionID]
	if !exists {
		return nil // No ports allocated, nothing to release
	}

	// Release main port
	p.usedPorts[ports.PORT] = false

	// Release PORTZ ports
	for _, port := range ports.PORTZ {
		p.usedPorts[port] = false
	}

	// Remove from allocated ports
	delete(p.allocatedPorts, sessionID)
	return nil
}

// GetPortsForSession returns the ports allocated to a session
func (p *PortAllocationService) GetPortsForSession(sessionID string) (*SessionPorts, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ports, exists := p.allocatedPorts[sessionID]
	return ports, exists
}

// findAvailablePort finds an available port starting from startPort
func (p *PortAllocationService) findAvailablePort() (int, error) {
	// Start from a fun port range, mixing it up
	basePorts := []int{3000, 4000, 5000, 6000, 7000, 8000, 9000}

	for _, base := range basePorts {
		for offset := 0; offset < 1000; offset++ {
			port := base + offset

			// Skip if already used by our service
			if p.usedPorts[port] {
				continue
			}

			// Check if port is actually available on the system
			if p.isPortAvailable(port) {
				return port, nil
			}
		}
	}

	return 0, fmt.Errorf("no available ports found")
}

// isPortAvailable checks if a port is available on the system
func (p *PortAllocationService) isPortAvailable(port int) bool {
	// Try to listen on the port
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	defer listener.Close()
	return true
}

// GetEnvironmentVariables returns environment variables for the session
func (p *PortAllocationService) GetEnvironmentVariables(sessionID string) ([]string, error) {
	ports, exists := p.GetPortsForSession(sessionID)
	if !exists {
		return nil, fmt.Errorf("no ports allocated for session %s", sessionID)
	}

	// Convert PORTZ to JSON
	portzJSON, err := json.Marshal(ports.PORTZ)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal PORTZ: %v", err)
	}

	return []string{
		fmt.Sprintf("PORT=%d", ports.PORT),
		fmt.Sprintf("PORTZ=%s", string(portzJSON)),
	}, nil
}

// ListAllAllocatedPorts returns all currently allocated ports
func (p *PortAllocationService) ListAllAllocatedPorts() map[string]*SessionPorts {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string]*SessionPorts)
	for k, v := range p.allocatedPorts {
		result[k] = v
	}
	return result
}
