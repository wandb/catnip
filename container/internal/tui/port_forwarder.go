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
	m.forwardsMu.Lock()
	if f, ok := m.forwards[containerPort]; ok {
		m.forwardsMu.Unlock()
		return f.hostPort
	}
	m.forwardsMu.Unlock()

	// Choose a host port (prefer same number)
	hostPort := m.findAvailableHostPort(containerPort)
	if hostPort == 0 {
		return 0
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", hostPort))
	if err != nil {
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
	_ = m.postMapping(containerPort, hostPort)

	go m.acceptLoop(fwd)
	return hostPort
}

// StopForward stops forwarding for a container port
func (m *PortForwardManager) StopForward(containerPort int) {
	m.forwardsMu.Lock()
	f, ok := m.forwards[containerPort]
	if ok {
		delete(m.forwards, containerPort)
	}
	m.forwardsMu.Unlock()

	if ok {
		_ = f.listener.Close()
		close(f.closed)
		_ = m.deleteMapping(containerPort)
	}
}

// StopAll stops all active forwards
func (m *PortForwardManager) StopAll() {
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

func (m *PortForwardManager) ensureSSH() (*ssh.Client, error) {
	m.clientMu.Lock()
	defer m.clientMu.Unlock()
	if m.client != nil {
		return m.client, nil
	}

	key, err := os.ReadFile(m.keyPath)
	if err != nil {
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
		return nil, err
	}
	m.client = client
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
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			select {
			case <-f.closed:
				return
			default:
				// transient error
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		go func(c net.Conn) {
			sshClient, err := m.ensureSSH()
			if err != nil {
				_ = c.Close()
				return
			}
			remote, err := sshClient.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", f.containerPort))
			if err != nil {
				_ = c.Close()
				// if ssh tunnel failed, reset client to force reconnect next time
				m.closeSSH()
				return
			}

			// bidirectional copy
			go func() { _, _ = io.Copy(remote, c); _ = remote.Close() }()
			go func() { _, _ = io.Copy(c, remote); _ = c.Close() }()
		}(conn)
	}
}

func (m *PortForwardManager) findAvailableHostPort(preferred int) int {
	// try preferred first if not standard reserved
	if preferred > 0 && preferred != 8080 && preferred != 2222 {
		if isFreePort(preferred) {
			return preferred
		}
	}
	// scan fun ranges 3000..9999, then 10000..61000
	for _, base := range []int{3000, 4000, 5000, 6000, 7000, 8000, 9000} {
		for off := 0; off < 1000; off++ {
			p := base + off
			if p == 8080 || p == 2222 {
				continue
			}
			if isFreePort(p) {
				return p
			}
		}
	}
	for p := 10000; p < 61000; p++ {
		if p == 8080 || p == 2222 {
			continue
		}
		if isFreePort(p) {
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
	return nil
}
