package services

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vanpelt/catnip/internal/config"
)

// ServiceInfo represents a detected service
type ServiceInfo struct {
	Port        int       `json:"port"`
	ServiceType string    `json:"service_type"`
	Health      string    `json:"health"`
	LastSeen    time.Time `json:"last_seen"`
	Title       string    `json:"title,omitempty"`
	PID         int       `json:"pid,omitempty"`
	Command     string    `json:"command,omitempty"`
	WorkingDir  string    `json:"working_dir,omitempty"`
}

// PortMonitor monitors /proc/net/tcp for port changes and manages service registry
type PortMonitor struct {
	services     map[int]*ServiceInfo
	mutex        sync.RWMutex
	lastTcpState map[int]bool
	stopChan     chan bool
	stopped      bool
}

// NewPortMonitor creates a new port monitor instance
func NewPortMonitor() *PortMonitor {
	pm := &PortMonitor{
		services:     make(map[int]*ServiceInfo),
		lastTcpState: make(map[int]bool),
		stopChan:     make(chan bool),
	}

	// Start monitoring immediately
	go pm.Start()

	return pm
}

// Start begins monitoring for port changes using the appropriate method for the OS
func (pm *PortMonitor) Start() {
	ticker := time.NewTicker(500 * time.Millisecond) // Check every 500ms for fast detection
	defer ticker.Stop()

	var method string
	if config.Runtime.PortMonitorEnabled {
		method = "/proc/net/tcp (Linux)"
	} else {
		method = "netstat/lsof (macOS/other)"
	}
	log.Printf("🔍 Started real-time port monitoring using %s", method)

	for {
		select {
		case <-ticker.C:
			pm.checkPortChanges()
			pm.checkKnownProcesses()
		case <-pm.stopChan:
			log.Printf("🛑 Stopped port monitoring")
			pm.stopped = true
			return
		}
	}
}

// Stop stops the port monitor
func (pm *PortMonitor) Stop() {
	if !pm.stopped {
		close(pm.stopChan)
	}
}

// GetServices returns all currently detected services
func (pm *PortMonitor) GetServices() map[int]*ServiceInfo {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	// Create a copy to avoid concurrent access issues
	services := make(map[int]*ServiceInfo)
	for port, info := range pm.services {
		services[port] = info
	}
	return services
}

// checkPortChanges compares current ports with last known state
func (pm *PortMonitor) checkPortChanges() {
	var currentPorts map[int]*PortWithPID
	var err error

	if config.Runtime.PortMonitorEnabled {
		// Linux: Use /proc/net/tcp
		currentPorts, err = pm.parseProcNetTcp()
		if err != nil {
			log.Printf("❌ Error parsing /proc/net/tcp: %v", err)
			return
		}
	} else {
		// macOS/other: Use netstat + lsof
		currentPorts, err = pm.parseNetstatPorts()
		if err != nil {
			log.Printf("❌ Error parsing netstat output: %v", err)
			return
		}
	}

	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Check for new ports
	for port, portInfo := range currentPorts {
		if !pm.lastTcpState[port] {
			// New port detected
			log.Printf("🔍 New port detected: %d (PID: %d)", port, portInfo.PID)
			pm.addService(port, portInfo.PID)
		}
	}

	// Check for removed ports
	for port := range pm.lastTcpState {
		if _, exists := currentPorts[port]; !exists {
			// Port removed
			log.Printf("🔍 Port removed: %d", port)
			delete(pm.services, port)
		}
	}

	// Update last TCP state to track port existence
	lastTcpState := make(map[int]bool)
	for port := range currentPorts {
		lastTcpState[port] = true
	}
	pm.lastTcpState = lastTcpState
}

// PortWithPID represents a port with its associated PID and inode
type PortWithPID struct {
	Port  int
	PID   int
	Inode int
}

// ProcessInfo represents a process that might be a development server
type ProcessInfo struct {
	PID         int
	Command     string
	WorkingDir  string
	ExpectedPort int
	LastSeen    time.Time
}

// parseProcNetTcp parses /proc/net/tcp and returns a map of listening ports with PID info
func (pm *PortMonitor) parseProcNetTcp() (map[int]*PortWithPID, error) {
	file, err := os.Open("/proc/net/tcp")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	listeningPorts := make(map[int]*PortWithPID)
	scanner := bufio.NewScanner(file)

	// Skip header line
	scanner.Scan()

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		if len(fields) < 10 {
			continue
		}

		// Local address is in field 1, format is "IP:PORT" in hex
		localAddr := fields[1]
		parts := strings.Split(localAddr, ":")
		if len(parts) != 2 {
			continue
		}

		// Parse hex port
		portHex := parts[1]
		port, err := strconv.ParseInt(portHex, 16, 32)
		if err != nil {
			continue
		}

		// Check if socket is in listening state (state 0A = 10 = TCP_LISTEN)
		state := fields[3]
		if state == "0A" {
			portInt := int(port)

			// Filter out ports we don't want to proxy:
			// - System ports (< 1024)
			// - Container's own port (8080)
			// - SSH (22), although it should be < 1024 anyway
			if portInt >= 1024 && portInt != 8080 && portInt != 22 {
				// Parse inode from field 9 (0-indexed)
				inode, err := strconv.Atoi(fields[9])
				if err != nil {
					continue
				}

				// Resolve PID from inode
				pid := pm.resolvePIDFromInode(inode)

				listeningPorts[portInt] = &PortWithPID{
					Port:  portInt,
					PID:   pid,
					Inode: inode,
				}
			}
		}
	}

	return listeningPorts, scanner.Err()
}

// parseNetstatPorts parses netstat output for macOS/other Unix systems
func (pm *PortMonitor) parseNetstatPorts() (map[int]*PortWithPID, error) {
	// Use netstat to find listening TCP ports
	cmd := exec.Command("netstat", "-an", "-p", "tcp")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run netstat: %v", err)
	}

	listeningPorts := make(map[int]*PortWithPID)
	lines := strings.Split(string(output), "\n")

	for _, line := range lines[1:] { // Skip header
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		// Check if it's in LISTEN state
		state := fields[5]
		if state != "LISTEN" {
			continue
		}

		// Parse local address (format: *.PORT, IP.PORT, or [::]:PORT)
		localAddr := fields[3]
		var portStr string

		// Handle different address formats
		if strings.Contains(localAddr, ".") {
			// IPv4: 127.0.0.1.8080 or *.8080
			parts := strings.Split(localAddr, ".")
			if len(parts) >= 2 {
				portStr = parts[len(parts)-1]
			}
		} else if strings.Contains(localAddr, ":") {
			// IPv6: [::]:8080 or similar
			parts := strings.Split(localAddr, ":")
			if len(parts) >= 2 {
				portStr = parts[len(parts)-1]
			}
		}

		if portStr == "" {
			continue
		}

		port, err := strconv.Atoi(portStr)
		if err != nil {
			continue
		}

		// Filter out ports we don't want to proxy (same logic as Linux version)
		if port >= 1024 && port != 8080 && port != 22 {
			// For macOS, we'll resolve PID using lsof in a separate step
			listeningPorts[port] = &PortWithPID{
				Port: port,
				PID:  0, // Will be resolved by lsof
			}
		}
	}

	// Resolve PIDs using lsof for each port
	for port, portInfo := range listeningPorts {
		pid := pm.resolvePIDFromPortMacOS(port)
		portInfo.PID = pid
	}

	return listeningPorts, nil
}

// resolvePIDFromPortMacOS uses lsof to find the PID listening on a specific port (macOS/Unix)
func (pm *PortMonitor) resolvePIDFromPortMacOS(port int) int {
	// Use lsof to find process listening on the port
	cmd := exec.Command("lsof", "-i", fmt.Sprintf(":%d", port), "-t")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	// lsof -t returns PIDs, one per line
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) > 0 && lines[0] != "" {
		if pid, err := strconv.Atoi(lines[0]); err == nil {
			return pid
		}
	}

	return 0
}

// addService adds a new service to the registry with health checking
func (pm *PortMonitor) addService(port int, pid int) {
	// Get command name from PID
	command := pm.getCommandFromPID(pid)
	// Get working directory from PID
	workingDir := pm.getWorkingDirFromPID(pid)

	// In native mode, filter ports by working directory
	if config.Runtime.IsNative() && !pm.shouldTrackPort(workingDir) {
		// Skip ports that aren't running from our workspace or current repo
		return
	}

	service := &ServiceInfo{
		Port:        port,
		ServiceType: "unknown",
		Health:      "unknown",
		LastSeen:    time.Now(),
		PID:         pid,
		Command:     command,
		WorkingDir:  workingDir,
	}

	// Try to determine service type and health
	go pm.healthCheckService(service)

	pm.services[port] = service
}

// healthCheckService attempts to determine service type and health status
func (pm *PortMonitor) healthCheckService(service *ServiceInfo) {
	// Give the service a moment to fully start
	time.Sleep(100 * time.Millisecond)

	// Try HTTP health check
	httpResult := pm.checkHTTPHealth(service)
	if httpResult.IsHTTP {
		pm.mutex.Lock()
		if existingService, exists := pm.services[service.Port]; exists {
			existingService.ServiceType = "http"
			if httpResult.IsHealthy {
				existingService.Health = "healthy"
				log.Printf("✅ Port %d: HTTP service detected and healthy", service.Port)
			} else {
				existingService.Health = "unhealthy"
				log.Printf("⚠️  Port %d: HTTP service detected but unhealthy", service.Port)
			}
			existingService.LastSeen = time.Now()
		}
		pm.mutex.Unlock()
		return
	}

	// Try TCP health check
	if pm.checkTCPHealth(service) {
		pm.mutex.Lock()
		if existingService, exists := pm.services[service.Port]; exists {
			existingService.ServiceType = "tcp"
			existingService.Health = "healthy"
			existingService.LastSeen = time.Now()
		}
		pm.mutex.Unlock()
		log.Printf("✅ Port %d: TCP service detected and healthy", service.Port)
		return
	}

	// Mark as unhealthy if all checks fail
	pm.mutex.Lock()
	if existingService, exists := pm.services[service.Port]; exists {
		existingService.Health = "unhealthy"
		existingService.LastSeen = time.Now()
	}
	pm.mutex.Unlock()
	log.Printf("❌ Port %d: Service detected but unhealthy", service.Port)
}

// HTTPHealthResult contains the result of HTTP health check
type HTTPHealthResult struct {
	IsHTTP    bool
	IsHealthy bool
	URL       string
}

// checkHTTPHealth checks if the service responds to HTTP requests
// Returns IsHTTP=true if any HTTP headers are received (indicating HTTP service)
// Returns IsHealthy=true if status code < 500 (indicating healthy service)
func (pm *PortMonitor) checkHTTPHealth(service *ServiceInfo) HTTPHealthResult {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	var lastError error
	// Try both http and https
	for _, scheme := range []string{"http", "https"} {
		url := fmt.Sprintf("%s://localhost:%d", scheme, service.Port)

		resp, err := client.Get(url)
		if err != nil {
			lastError = err
			continue
		}
		resp.Body.Close()

		// If we got any HTTP response (even error), it's an HTTP service
		result := HTTPHealthResult{
			IsHTTP:    true,
			IsHealthy: resp.StatusCode < 500,
			URL:       url,
		}

		// Extract title from response if it's HTML and healthy
		if result.IsHealthy {
			pm.extractTitle(service, url)
		}

		return result
	}

	// Log the failure reason for better debugging
	if lastError != nil {
		log.Printf("⚠️  Port %d HTTP health check failed: %v (command: %s, working dir: %s)", 
			service.Port, lastError, service.Command, service.WorkingDir)
	}

	return HTTPHealthResult{
		IsHTTP:    false,
		IsHealthy: false,
	}
}

// checkTCPHealth checks if the service accepts TCP connections
func (pm *PortMonitor) checkTCPHealth(service *ServiceInfo) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", service.Port), 2*time.Second)
	if err != nil {
		log.Printf("⚠️  Port %d TCP health check failed: %v (command: %s, working dir: %s)", 
			service.Port, err, service.Command, service.WorkingDir)
		return false
	}
	conn.Close()
	return true
}

// resolvePIDFromInode finds the PID that owns a socket inode by scanning /proc/*/fd/*
func (pm *PortMonitor) resolvePIDFromInode(inode int) int {
	inodeStr := fmt.Sprintf("socket:[%d]", inode)

	// Walk through all PIDs in /proc
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Check if directory name is numeric (PID)
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		// Check /proc/PID/fd directory
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fdEntries, err := os.ReadDir(fdDir)
		if err != nil {
			continue // Process may have exited or we don't have permission
		}

		for _, fdEntry := range fdEntries {
			fdPath := filepath.Join(fdDir, fdEntry.Name())

			// Read the symlink target
			target, err := os.Readlink(fdPath)
			if err != nil {
				continue
			}

			// Check if this fd points to our socket inode
			if target == inodeStr {
				return pid
			}
		}
	}

	return 0 // PID not found
}

// getCommandFromPID extracts the command name from a PID (cross-platform)
func (pm *PortMonitor) getCommandFromPID(pid int) string {
	if pid == 0 {
		return ""
	}

	if config.Runtime.PortMonitorEnabled {
		// Linux: Use /proc filesystem
		return pm.getCommandFromPIDLinux(pid)
	} else {
		// macOS/other: Use ps command
		return pm.getCommandFromPIDMacOS(pid)
	}
}

// getCommandFromPIDLinux extracts command name using /proc (Linux)
func (pm *PortMonitor) getCommandFromPIDLinux(pid int) string {
	// Try to read /proc/PID/cmdline first (full command line)
	cmdlinePath := filepath.Join("/proc", strconv.Itoa(pid), "cmdline")
	if data, err := os.ReadFile(cmdlinePath); err == nil {
		// cmdline is null-separated, take first argument
		cmdline := string(data)
		if len(cmdline) > 0 {
			// Split by null bytes and take the first part
			parts := strings.Split(cmdline, "\x00")
			if len(parts) > 0 && parts[0] != "" {
				// Extract just the command name from the full path
				return filepath.Base(parts[0])
			}
		}
	}

	// Fall back to /proc/PID/comm (just the command name)
	commPath := filepath.Join("/proc", strconv.Itoa(pid), "comm")
	if data, err := os.ReadFile(commPath); err == nil {
		comm := strings.TrimSpace(string(data))
		if comm != "" {
			return comm
		}
	}

	return ""
}

// getCommandFromPIDMacOS extracts command name using ps (macOS/Unix)
func (pm *PortMonitor) getCommandFromPIDMacOS(pid int) string {
	// Use ps to get command name
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	command := strings.TrimSpace(string(output))
	if command != "" {
		// Extract just the command name from the full path
		return filepath.Base(command)
	}

	return ""
}

// getWorkingDirFromPID extracts the working directory from a PID (cross-platform)
func (pm *PortMonitor) getWorkingDirFromPID(pid int) string {
	if pid == 0 {
		return ""
	}

	if config.Runtime.PortMonitorEnabled {
		// Linux: Use /proc filesystem
		return pm.getWorkingDirFromPIDLinux(pid)
	} else {
		// macOS/other: Use lsof or pwdx if available
		return pm.getWorkingDirFromPIDMacOS(pid)
	}
}

// getWorkingDirFromPIDLinux extracts working directory using /proc (Linux)
func (pm *PortMonitor) getWorkingDirFromPIDLinux(pid int) string {
	// Read the cwd symlink from /proc/PID/cwd
	cwdPath := filepath.Join("/proc", strconv.Itoa(pid), "cwd")
	if workingDir, err := os.Readlink(cwdPath); err == nil {
		return workingDir
	}
	return ""
}

// getWorkingDirFromPIDMacOS extracts working directory using lsof (macOS/Unix)
func (pm *PortMonitor) getWorkingDirFromPIDMacOS(pid int) string {
	// Use lsof to get the current working directory
	cmd := exec.Command("lsof", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// lsof -Fn output format: n<path>
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "n") && len(line) > 1 {
			return line[1:] // Remove the 'n' prefix
		}
	}

	return ""
}

// shouldTrackPort determines if we should track a port based on its working directory
func (pm *PortMonitor) shouldTrackPort(workingDir string) bool {
	if workingDir == "" {
		return false
	}

	// Always track ports from our workspace directory
	if strings.HasPrefix(workingDir, config.Runtime.WorkspaceDir) {
		return true
	}

	// Track ports from the current repository if we're running from one
	if config.Runtime.CurrentRepo != "" {
		// Get current working directory (where catnip serve was started)
		if cwd, err := os.Getwd(); err == nil {
			if strings.HasPrefix(workingDir, cwd) {
				return true
			}
		}
	}

	return false
}

// checkKnownProcesses scans for known development servers that should be listening but aren't
func (pm *PortMonitor) checkKnownProcesses() {
	if !config.Runtime.PortMonitorEnabled {
		return // Only works on Linux for now
	}
	
	// Map of known development server processes and their expected ports
	knownServers := map[string][]int{
		"mintlify":    {3000},
		"next-server": {3000, 8080},
		"vite":        {3000, 5173},
		"webpack-dev-server": {3000, 8080},
		"serve":       {3000, 5000, 8080},
		"http-server": {8080},
		"node":        {3000, 8000, 8080}, // Generic Node.js processes
	}
	
	// Find all running processes
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	
	currentProcesses := make(map[int]*ProcessInfo)
	
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		
		// Check if directory name is numeric (PID)
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		
		command := pm.getCommandFromPIDLinux(pid)
		workingDir := pm.getWorkingDirFromPIDLinux(pid)
		
		// Check if this is a known development server
		if expectedPorts, isKnownServer := knownServers[command]; isKnownServer {
			// In native mode, only track processes from our workspace
			if config.Runtime.IsNative() && !pm.shouldTrackPort(workingDir) {
				continue
			}
			
			for _, expectedPort := range expectedPorts {
				processInfo := &ProcessInfo{
					PID:          pid,
					Command:      command,
					WorkingDir:   workingDir,
					ExpectedPort: expectedPort,
					LastSeen:     time.Now(),
				}
				currentProcesses[pid] = processInfo
				
				// Check if this process should be listening on a port but isn't
				pm.mutex.RLock()
				_, isListening := pm.services[expectedPort]
				pm.mutex.RUnlock()
				
				if !isListening {
					// Check if we've already reported this issue recently
					if lastProcess, wasReported := pm.lastProcessState[pid]; !wasReported || 
						time.Since(lastProcess.LastSeen) > 30*time.Second {
						log.Printf("⚠️  Process %s (PID %d) in %s appears to be a dev server but isn't listening on expected port %d", 
							command, pid, workingDir, expectedPort)
					}
				}
				break // Only check the first expected port for each process
			}
		}
	}
	
	// Update the last process state
	pm.lastProcessState = currentProcesses
}

// extractTitle attempts to extract the title from an HTML response
func (pm *PortMonitor) extractTitle(service *ServiceInfo, url string) {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	// Only process HTML responses
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		return
	}

	// Read first 4KB to find title
	buffer := make([]byte, 4096)
	n, _ := resp.Body.Read(buffer)
	content := string(buffer[:n])

	// Extract title using regex
	titleRegex := regexp.MustCompile(`<title[^>]*>(.*?)</title>`)
	matches := titleRegex.FindStringSubmatch(content)
	if len(matches) > 1 {
		title := strings.TrimSpace(matches[1])
		if title != "" {
			pm.mutex.Lock()
			if existingService, exists := pm.services[service.Port]; exists {
				// If we have a command name, append it to the title
				if existingService.Command != "" {
					existingService.Title = fmt.Sprintf("%s (%s)", title, existingService.Command)
				} else {
					existingService.Title = title
				}
			}
			pm.mutex.Unlock()
		}
	}
}
