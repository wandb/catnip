package tui

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// PortForwardManager manages SSH port forwarders from host -> container
// It listens on 127.0.0.1:hostPort and forwards to container localhost:containerPort over SSH.
type PortForwardManager struct {
	backendBaseURL string

	sshUser    string
	sshAddress string // host:port, typically 127.0.0.1:2222
	keyPath    string // ~/.ssh/catnip_remote

	clientMu sync.Mutex
	client   *ssh.Client

	forwardsMu sync.Mutex
	forwards   map[int]*activeForward // containerPort -> forward

	httpClient *http.Client
}

type activeForward struct {
	containerPort int
	hostPort      int
	listener      net.Listener
	closed        chan struct{}
}

func NewPortForwardManager(backendBaseURL string) *PortForwardManager {
	// Derive ssh parameters
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME")
	}
	keyPath := filepath.Join(os.Getenv("HOME"), ".ssh", "catnip_remote")

	debugLog("PFM: init backend=%s", backendBaseURL)
	return &PortForwardManager{
		backendBaseURL: backendBaseURL,
		sshUser:        user,
		sshAddress:     "127.0.0.1:2222",
		keyPath:        keyPath,
		forwards:       make(map[int]*activeForward),
		httpClient:     &http.Client{Timeout: 2 * time.Second},
	}
}

// EnsureForward starts a forward for the given container port if not running.
// Returns the selected hostPort, or 0 on failure.
func (m *PortForwardManager) EnsureForward(containerPort int) int {
	debugLog("PFM: EnsureForward containerPort=%d", containerPort)
	m.forwardsMu.Lock()
	if f, ok := m.forwards[containerPort]; ok {
		debugLog("PFM: already forwarding containerPort=%d hostPort=%d", containerPort, f.hostPort)
		m.forwardsMu.Unlock()
		return f.hostPort
	}
	m.forwardsMu.Unlock()

	// Choose a host port (prefer same number)
	hostPort := m.findAvailableHostPort(containerPort)
	if hostPort == 0 {
		debugLog("PFM: failed to find available host port for %d", containerPort)
		return 0
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", hostPort))
	if err != nil {
		debugLog("PFM: listen failed hostPort=%d err=%v", hostPort, err)
		return 0
	}

	fwd := &activeForward{
		containerPort: containerPort,
		hostPort:      hostPort,
		listener:      ln,
		closed:        make(chan struct{}),
	}

	m.forwardsMu.Lock()
	m.forwards[containerPort] = fwd
	m.forwardsMu.Unlock()

	// Notify backend mapping
	if err := m.postMapping(containerPort, hostPort); err != nil {
		debugLog("PFM: postMapping failed cport=%d hport=%d err=%v", containerPort, hostPort, err)
	} else {
		debugLog("PFM: postMapping ok cport=%d hport=%d", containerPort, hostPort)
	}

	go m.acceptLoop(fwd)
	debugLog("PFM: forwarding started cport=%d -> 127.0.0.1:%d", containerPort, hostPort)
	return hostPort
}

// StopForward stops forwarding for a container port
func (m *PortForwardManager) StopForward(containerPort int) {
	debugLog("PFM: StopForward containerPort=%d", containerPort)
	m.forwardsMu.Lock()
	f, ok := m.forwards[containerPort]
	if ok {
		delete(m.forwards, containerPort)
	}
	m.forwardsMu.Unlock()

	if ok {
		_ = f.listener.Close()
		close(f.closed)
		if err := m.deleteMapping(containerPort); err != nil {
			debugLog("PFM: deleteMapping failed cport=%d err=%v", containerPort, err)
		} else {
			debugLog("PFM: deleteMapping ok cport=%d", containerPort)
		}
	}
}

// StopAll stops all active forwards
func (m *PortForwardManager) StopAll() {
	debugLog("PFM: StopAll")
	m.forwardsMu.Lock()
	ports := make([]int, 0, len(m.forwards))
	for p := range m.forwards {
		ports = append(ports, p)
	}
	m.forwardsMu.Unlock()
	for _, p := range ports {
		m.StopForward(p)
	}
	m.closeSSH()
}

// ReannounceMappings posts all current containerPort->hostPort mappings to backend.
// Useful after backend restarts to repopulate in-memory state.
func (m *PortForwardManager) ReannounceMappings() {
	m.forwardsMu.Lock()
	// create snapshot to avoid holding lock during network calls
	snapshot := make(map[int]int, len(m.forwards))
	for cport, f := range m.forwards {
		snapshot[cport] = f.hostPort
	}
	m.forwardsMu.Unlock()

	for cport, hport := range snapshot {
		if err := m.postMapping(cport, hport); err != nil {
			debugLog("PFM: reannounce postMapping failed cport=%d hport=%d err=%v", cport, hport, err)
		} else {
			debugLog("PFM: reannounce postMapping ok cport=%d hport=%d", cport, hport)
		}
	}
}

func (m *PortForwardManager) ensureSSH() (*ssh.Client, error) {
	m.clientMu.Lock()
	defer m.clientMu.Unlock()
	if m.client != nil {
		return m.client, nil
	}

	debugLog("PFM: ensureSSH user=%s addr=%s key=%s", m.sshUser, m.sshAddress, m.keyPath)
	key, err := os.ReadFile(m.keyPath)
	if err != nil {
		debugLog("PFM: read key failed: %v", err)
		return nil, err
	}
	var signer ssh.Signer
	if bytes.Contains(key, []byte("BEGIN OPENSSH PRIVATE KEY")) {
		signer, err = ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, err
		}
	} else {
		// Try PEM (PKCS1/PKCS8) for safety
		block, _ := pem.Decode(key)
		if block == nil {
			return nil, fmt.Errorf("failed to parse private key")
		}
		parsed, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		signer, err = ssh.NewSignerFromKey(parsed)
		if err != nil {
			return nil, err
		}
	}

	// NOTE: We skip host key verification for local dev convenience.
	// The connection targets 127.0.0.1:2222 within the developer's machine.
	// Consider configuring known_hosts verification in the future.
	cfg := &ssh.ClientConfig{
		User:            m.sshUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // Local-only dev convenience (127.0.0.1:2222)
		Timeout:         5 * time.Second,
	}
	client, err := ssh.Dial("tcp", m.sshAddress, cfg)
	if err != nil {
		debugLog("PFM: ssh.Dial failed: %v", err)
		return nil, err
	}
	m.client = client
	debugLog("PFM: ssh connected")
	return client, nil
}

func (m *PortForwardManager) closeSSH() {
	m.clientMu.Lock()
	defer m.clientMu.Unlock()
	if m.client != nil {
		_ = m.client.Close()
		m.client = nil
	}
}

func (m *PortForwardManager) acceptLoop(f *activeForward) {
	debugLog("PFM: acceptLoop start cport=%d hport=%d", f.containerPort, f.hostPort)
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			select {
			case <-f.closed:
				debugLog("PFM: acceptLoop closed cport=%d", f.containerPort)
				return
			default:
				// transient error
				debugLog("PFM: accept error: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		go func(c net.Conn) {
			sshClient, err := m.ensureSSH()
			if err != nil {
				_ = c.Close()
				debugLog("PFM: ensureSSH in conn failed: %v", err)
				return
			}
			remote, err := sshClient.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", f.containerPort))
			if err != nil {
				_ = c.Close()
				// if ssh tunnel failed, reset client to force reconnect next time
				m.closeSSH()
				debugLog("PFM: sshClient.Dial to container failed: %v", err)
				return
			}

			// bidirectional copy
			go func() {
				_, _ = io.Copy(remote, c)
				_ = remote.Close()
				debugLog("PFM: upstream copy done cport=%d", f.containerPort)
			}()
			go func() {
				_, _ = io.Copy(c, remote)
				_ = c.Close()
				debugLog("PFM: downstream copy done cport=%d", f.containerPort)
			}()
		}(conn)
	}
}

func (m *PortForwardManager) findAvailableHostPort(preferred int) int {
	debugLog("PFM: findAvailableHostPort preferred=%d", preferred)
	// try preferred first if not standard reserved
	if preferred > 0 && preferred != 6369 && preferred != 2222 {
		if isFreePort(preferred) {
			debugLog("PFM: using preferred host port %d", preferred)
			return preferred
		}
	}
	// scan fun ranges 3000..9999, then 10000..61000
	for _, base := range []int{3000, 4000, 5000, 6000, 7000, 8000, 9000} {
		for off := 0; off < 1000; off++ {
			p := base + off
			if p == 6369 || p == 2222 {
				continue
			}
			if isFreePort(p) {
				debugLog("PFM: selected host port %d", p)
				return p
			}
		}
	}
	for p := 10000; p < 61000; p++ {
		if p == 6369 || p == 2222 {
			continue
		}
		if isFreePort(p) {
			debugLog("PFM: selected host port %d", p)
			return p
		}
	}
	return 0
}

func isFreePort(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func (m *PortForwardManager) postMapping(containerPort, hostPort int) error {
	body := fmt.Sprintf(`{"port":%d,"host_port":%d}`, containerPort, hostPort)
	req, err := http.NewRequest("POST", m.backendBaseURL+"/v1/ports/mappings", bytes.NewBufferString(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	debugLog("PFM: postMapping response status=%s", resp.Status)
	return nil
}

func (m *PortForwardManager) deleteMapping(containerPort int) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/v1/ports/mappings/%d", m.backendBaseURL, containerPort), nil)
	if err != nil {
		return err
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	debugLog("PFM: deleteMapping response status=%s", resp.Status)
	return nil
}
