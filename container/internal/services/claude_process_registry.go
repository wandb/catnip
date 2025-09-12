package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/logger"
	"github.com/vanpelt/catnip/internal/models"
)

// ActiveClaudeProcess represents a running Claude CLI process
type ActiveClaudeProcess struct {
	WorkingDirectory string
	Process          *exec.Cmd
	StartTime        time.Time
	LastAccessed     time.Time
	Options          *ClaudeSubprocessOptions

	// Process control
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// Client management
	clientsMutex sync.RWMutex
	clients      map[string]chan []byte // client ID -> output channel
}

// ClaudeProcessRegistry manages persistent Claude processes
type ClaudeProcessRegistry struct {
	processesMutex sync.RWMutex
	processes      map[string]*ActiveClaudeProcess // working directory -> process

	// Cleanup management
	cleanupInterval time.Duration
	processTimeout  time.Duration
	stopCleanup     chan struct{}
	cleanupWg       sync.WaitGroup
}

// NewClaudeProcessRegistry creates a new process registry
func NewClaudeProcessRegistry() *ClaudeProcessRegistry {
	registry := &ClaudeProcessRegistry{
		processes:       make(map[string]*ActiveClaudeProcess),
		cleanupInterval: 1 * time.Minute,  // Check every minute
		processTimeout:  10 * time.Minute, // Kill processes after 10 minutes of inactivity
		stopCleanup:     make(chan struct{}),
	}

	// Start cleanup goroutine
	registry.cleanupWg.Add(1)
	go registry.cleanupLoop()

	return registry
}

// GetOrCreateProcess gets an existing process or creates a new one
func (r *ClaudeProcessRegistry) GetOrCreateProcess(opts *ClaudeSubprocessOptions, wrapper *ClaudeSubprocessWrapper) (*ActiveClaudeProcess, bool, error) {
	r.processesMutex.Lock()
	defer r.processesMutex.Unlock()

	workingDir := opts.WorkingDirectory

	// Check if process already exists and is still running
	if existing, exists := r.processes[workingDir]; exists {
		if existing.Process != nil && existing.Process.Process != nil {
			// Update last accessed time
			existing.LastAccessed = time.Now()
			logger.Infof("🔄 Reconnecting to existing Claude process for %s (PID: %d)", workingDir, existing.Process.Process.Pid)
			return existing, false, nil // false = not newly created
		} else {
			// Process is dead, remove it
			logger.Infof("🧹 Removing dead Claude process for %s", workingDir)
			delete(r.processes, workingDir)
		}
	}

	// Create new process
	logger.Infof("🚀 Creating new Claude process for %s", workingDir)
	process, err := r.createProcess(opts, wrapper)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create Claude process: %w", err)
	}

	r.processes[workingDir] = process
	return process, true, nil // true = newly created
}

// createProcess creates a new persistent Claude process
func (r *ClaudeProcessRegistry) createProcess(opts *ClaudeSubprocessOptions, wrapper *ClaudeSubprocessWrapper) (*ActiveClaudeProcess, error) {
	// Use background context so process persists beyond HTTP request
	ctx, cancel := context.WithCancel(context.Background())

	process := &ActiveClaudeProcess{
		WorkingDirectory: opts.WorkingDirectory,
		StartTime:        time.Now(),
		LastAccessed:     time.Now(),
		Options:          opts,
		ctx:              ctx,
		cancel:           cancel,
		done:             make(chan struct{}),
		clients:          make(map[string]chan []byte),
	}

	// Start the Claude process using the existing wrapper logic but with persistent context
	cmd, err := r.startClaudeProcess(ctx, opts, wrapper, process)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start Claude process: %w", err)
	}

	process.Process = cmd

	// Start goroutine to monitor process completion
	go func() {
		defer close(process.done)
		defer cancel()

		if err := cmd.Wait(); err != nil {
			logger.Debugf("🔚 Claude process exited for %s: %v", opts.WorkingDirectory, err)
		} else {
			logger.Debugf("✅ Claude process completed successfully for %s", opts.WorkingDirectory)
		}

		// Remove from registry when process completes
		r.processesMutex.Lock()
		if r.processes[opts.WorkingDirectory] == process {
			delete(r.processes, opts.WorkingDirectory)
		}
		r.processesMutex.Unlock()

		// Close all client channels
		process.clientsMutex.Lock()
		for clientID, ch := range process.clients {
			close(ch)
			logger.Debugf("📡 Closed client channel for %s in %s", clientID, opts.WorkingDirectory)
		}
		process.clientsMutex.Unlock()
	}()

	return process, nil
}

// startClaudeProcess starts the actual Claude CLI process (extracted from existing code)
func (r *ClaudeProcessRegistry) startClaudeProcess(ctx context.Context, opts *ClaudeSubprocessOptions, wrapper *ClaudeSubprocessWrapper, process *ActiveClaudeProcess) (*exec.Cmd, error) {
	// This is basically the same logic as CreateStreamingCompletion but with persistent context
	// and output handling for multiple clients

	args := []string{"-p"}
	args = append(args, "--output-format=stream-json")
	args = append(args, "--input-format=stream-json")
	args = append(args, "--verbose")
	args = append(args, "--dangerously-skip-permissions")

	if opts.SystemPrompt != "" {
		args = append(args, "--system-prompt", opts.SystemPrompt)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", opts.MaxTurns))
	}
	if opts.Resume {
		args = append(args, "--continue")
	}

	cmd := exec.CommandContext(ctx, wrapper.claudePath, args...)
	cmd.Dir = opts.WorkingDirectory

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start command with retry logic
	if err := wrapper.retryClaudeCommand(ctx, cmd, "persistent-streaming"); err != nil {
		return nil, err
	}

	// Send initial prompt
	go func() {
		defer stdin.Close()
		message := map[string]interface{}{
			"type": "user",
			"message": map[string]string{
				"role":    "user",
				"content": opts.Prompt,
			},
		}

		if messageJSON, err := json.Marshal(message); err == nil {
			if _, writeErr := stdin.Write(messageJSON); writeErr != nil {
				logger.Errorf("Failed to write message to stdin: %v", writeErr)
				return
			}
			if _, writeErr := stdin.Write([]byte("\n")); writeErr != nil {
				logger.Errorf("Failed to write newline to stdin: %v", writeErr)
				return
			}
		} else {
			logger.Errorf("Failed to marshal message: %v", err)
		}
	}()

	// Start output broadcaster goroutine
	go process.broadcastOutput(stdout)

	// Handle stderr
	go func() {
		// Same as existing stderr handling
		io.Copy(io.Discard, stderr) // For now just discard stderr
	}()

	return cmd, nil
}

// AddClient adds a client to receive process output
func (p *ActiveClaudeProcess) AddClient(clientID string) <-chan []byte {
	p.clientsMutex.Lock()
	defer p.clientsMutex.Unlock()

	ch := make(chan []byte, 100) // Buffered channel for output
	p.clients[clientID] = ch
	p.LastAccessed = time.Now()

	logger.Debugf("📡 Added client %s to Claude process for %s", clientID, p.WorkingDirectory)
	return ch
}

// RemoveClient removes a client from receiving process output
func (p *ActiveClaudeProcess) RemoveClient(clientID string) {
	p.clientsMutex.Lock()
	defer p.clientsMutex.Unlock()

	if ch, exists := p.clients[clientID]; exists {
		close(ch)
		delete(p.clients, clientID)
		logger.Debugf("📡 Removed client %s from Claude process for %s", clientID, p.WorkingDirectory)
	}
}

// broadcastOutput reads from stdout and broadcasts to all connected clients
func (p *ActiveClaudeProcess) broadcastOutput(stdout io.Reader) {
	logger.Debugf("📡 Started output broadcaster for %s", p.WorkingDirectory)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse JSON line
		var jsonData map[string]interface{}
		if err := json.Unmarshal([]byte(line), &jsonData); err != nil {
			continue // Skip invalid JSON lines
		}

		// Look for assistant messages and broadcast them
		if msgType, ok := jsonData["type"].(string); ok && msgType == "assistant" {
			// Parse and extract just the text content
			var responseText string
			if message, ok := jsonData["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].([]interface{}); ok && len(content) > 0 {
					if textBlock, ok := content[0].(map[string]interface{}); ok {
						if text, ok := textBlock["text"].(string); ok {
							responseText = text
						}
					}
				}
			}

			if responseText == "" {
				responseText = "No text content found in assistant response"
			}

			// Create a clean response chunk
			response := &models.CreateCompletionResponse{
				Response: responseText,
				IsChunk:  true,
				IsLast:   false,
			}

			// Marshal and broadcast to all clients
			responseJSON, err := json.Marshal(response)
			if err != nil {
				continue // Skip this chunk if we can't marshal it
			}

			outputBytes := append(responseJSON, '\n')
			p.broadcastToClients(outputBytes)
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Errorf("📡 Error reading stdout: %v", err)
	}

	logger.Debugf("📡 Output broadcaster finished for %s", p.WorkingDirectory)
}

// broadcastToClients sends output to all connected clients
func (p *ActiveClaudeProcess) broadcastToClients(data []byte) {
	p.clientsMutex.RLock()
	defer p.clientsMutex.RUnlock()

	for clientID, ch := range p.clients {
		select {
		case ch <- data:
			// Successfully sent to client
		default:
			// Channel buffer is full, skip this client
			logger.Warnf("📡 Skipping client %s, buffer full", clientID)
		}
	}
}

// IsRunning checks if the process is still running
func (p *ActiveClaudeProcess) IsRunning() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// Stop gracefully stops the process
func (p *ActiveClaudeProcess) Stop() {
	logger.Infof("🛑 Stopping Claude process for %s", p.WorkingDirectory)
	p.cancel()
}

// cleanupLoop periodically cleans up stale processes
func (r *ClaudeProcessRegistry) cleanupLoop() {
	defer r.cleanupWg.Done()

	ticker := time.NewTicker(r.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.cleanupStaleProcesses()
		case <-r.stopCleanup:
			return
		}
	}
}

// cleanupStaleProcesses removes processes that haven't been accessed recently
func (r *ClaudeProcessRegistry) cleanupStaleProcesses() {
	r.processesMutex.Lock()
	defer r.processesMutex.Unlock()

	now := time.Now()
	for workingDir, process := range r.processes {
		if now.Sub(process.LastAccessed) > r.processTimeout {
			logger.Infof("🧹 Cleaning up stale Claude process for %s (last accessed: %v)", workingDir, process.LastAccessed)
			process.Stop()
			delete(r.processes, workingDir)
		}
	}
}

// Shutdown gracefully shuts down the registry
func (r *ClaudeProcessRegistry) Shutdown() {
	logger.Infof("🔚 Shutting down Claude process registry...")

	// Stop cleanup goroutine
	close(r.stopCleanup)
	r.cleanupWg.Wait()

	// Stop all processes
	r.processesMutex.Lock()
	defer r.processesMutex.Unlock()

	for workingDir, process := range r.processes {
		logger.Infof("🛑 Stopping Claude process for %s", workingDir)
		process.Stop()
	}
}

// GetActiveProcesses returns a list of currently active processes
func (r *ClaudeProcessRegistry) GetActiveProcesses() map[string]*ActiveClaudeProcess {
	r.processesMutex.RLock()
	defer r.processesMutex.RUnlock()

	result := make(map[string]*ActiveClaudeProcess)
	for k, v := range r.processes {
		if v.IsRunning() {
			result[k] = v
		}
	}
	return result
}
