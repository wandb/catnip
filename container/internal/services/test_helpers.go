package services

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/creack/pty"
)

// TestProxy is a simple HTTP/HTTPS proxy for testing OAuth flows
type TestProxy struct {
	listener    net.Listener
	server      *http.Server
	interceptor *OAuthInterceptor
	mu          sync.RWMutex
	connections []net.Conn
	cert        tls.Certificate // Self-signed cert for MITM
	logFunc     func(string, ...interface{})
}

// OAuthInterceptor handles OAuth code exchange interception
type OAuthInterceptor struct {
	mu                 sync.RWMutex
	shouldFailExchange bool
	exchangeDelay      time.Duration
	codeExchangeCount  int
	lastCode           string
	captureMode        bool // If true, forward to real API and capture response
	logFunc            func(string, ...interface{})
}

// PTYTestHelper manages a Claude PTY session for testing
type PTYTestHelper struct {
	cmd          *exec.Cmd
	ptyFile      *os.File
	homeDir      string
	outputBuffer *bytes.Buffer
	outputMu     sync.RWMutex
	stopChan     chan struct{}
}

// NewTestProxyForCapture creates a test proxy for standalone capture programs
func NewTestProxyForCapture() (*TestProxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	// Generate self-signed certificate for MITM
	cert, err := generateSelfSignedCert()
	if err != nil {
		return nil, fmt.Errorf("failed to generate cert: %w", err)
	}

	proxy := &TestProxy{
		listener: listener,
		interceptor: &OAuthInterceptor{
			logFunc: func(format string, args ...interface{}) {
				fmt.Printf(format+"\n", args...)
			},
		},
		cert: cert,
		logFunc: func(format string, args ...interface{}) {
			fmt.Printf(format+"\n", args...)
		},
	}

	proxy.server = &http.Server{
		Handler:           proxy,
		ReadHeaderTimeout: 30 * time.Second,
	}

	go func() { _ = proxy.server.Serve(listener) }()

	return proxy, nil
}

// NewPTYTestHelperForCapture creates a PTY helper for standalone programs
func NewPTYTestHelperForCapture(proxyAddr string) (*PTYTestHelper, error) {
	// Create temporary home directory
	homeDir, err := os.MkdirTemp("", "claude-test-home-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp home: %w", err)
	}

	// Create XDG directories to simulate Linux environment
	_ = os.MkdirAll(homeDir+"/.config", 0755)
	_ = os.MkdirAll(homeDir+"/.local/share", 0755)
	_ = os.MkdirAll(homeDir+"/.runtime", 0755)

	// Pre-create Claude config directory
	_ = os.MkdirAll(homeDir+"/.claude", 0755)
	_ = os.MkdirAll(homeDir+"/.config/claude", 0755)

	// Create a dummy credentials file to prevent keychain access attempts
	// This makes Claude think credentials are already stored
	credFile := homeDir + "/.config/claude/credentials.json"
	_ = os.WriteFile(credFile, []byte(`{"version":"1","credentials":[]}`), 0600)

	// Create fake bin directory and security command stub
	// This prevents Claude from accessing macOS keychain by making the 'security' command fail
	binDir := homeDir + "/bin"
	_ = os.MkdirAll(binDir, 0755)
	securityScript := binDir + "/security"
	// Create a script that immediately exits with error
	_ = os.WriteFile(securityScript, []byte("#!/bin/sh\nexit 1\n"), 0755)

	// Start Claude in PTY
	claudePath := getClaudePath()
	cmd := exec.Command(claudePath, "--dangerously-skip-permissions")
	cmd.Dir = homeDir

	// Create a minimal environment that prevents browser opening and keychain access
	// Create a fake PATH that excludes the 'security' command to force plaintext storage
	fakePath := homeDir + "/bin:" + os.Getenv("PATH")

	cmd.Env = []string{
		"HOME=" + homeDir,
		"HTTPS_PROXY=http://" + proxyAddr,
		"HTTP_PROXY=http://" + proxyAddr,
		"PATH=" + fakePath, // Use modified PATH that will have fake 'security' command
		"BROWSER=true",     // Set to true to prevent browser opening
		"DISPLAY=",         // Disable X11 to prevent browser
		"TERM=" + os.Getenv("TERM"),
		"NODE_TLS_REJECT_UNAUTHORIZED=0", // Disable TLS validation to allow MITM proxy
		"DISABLE_TELEMETRY=1",            // Disable telemetry in tests
		// XDG directories for credential storage
		"XDG_CONFIG_HOME=" + homeDir + "/.config",
		"XDG_DATA_HOME=" + homeDir + "/.local/share",
		"XDG_RUNTIME_DIR=" + homeDir + "/.runtime",
	}

	ptyFile, err := pty.Start(cmd)
	if err != nil {
		os.RemoveAll(homeDir)
		return nil, fmt.Errorf("failed to start PTY: %w", err)
	}

	helper := &PTYTestHelper{
		cmd:          cmd,
		ptyFile:      ptyFile,
		homeDir:      homeDir,
		outputBuffer: &bytes.Buffer{},
		stopChan:     make(chan struct{}),
	}

	return helper, nil
}

// getClaudePath returns the path to the claude executable
func getClaudePath() string {
	// Try PATH first
	if path, err := exec.LookPath("claude"); err == nil {
		return path
	}

	// Try common locations
	homeDir := os.Getenv("HOME")
	candidates := []string{
		homeDir + "/.claude/local/claude",
		"/usr/local/bin/claude",
		"/opt/homebrew/bin/claude",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return "claude" // fallback
}

// GetPTYFile returns the PTY file
func (h *PTYTestHelper) GetPTYFile() *os.File {
	return h.ptyFile
}

// GetCmd returns the command
func (h *PTYTestHelper) GetCmd() *exec.Cmd {
	return h.cmd
}

// GetHomeDir returns the home directory
func (h *PTYTestHelper) GetHomeDir() string {
	return h.homeDir
}

// Close cleans up the PTY helper
func (h *PTYTestHelper) Close() error {
	// Kill the process if it's still running
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
	}

	// Close PTY file
	if h.ptyFile != nil {
		h.ptyFile.Close()
	}

	// Clean up temp directory
	if h.homeDir != "" {
		os.RemoveAll(h.homeDir)
	}

	return nil
}

// DumpOutput returns the buffered PTY output
func (h *PTYTestHelper) DumpOutput() string {
	h.outputMu.RLock()
	defer h.outputMu.RUnlock()
	return h.outputBuffer.String()
}

// Addr returns the proxy address
func (p *TestProxy) Addr() string {
	return p.listener.Addr().String()
}

// SetCaptureMode enables/disables capture mode (forward to real API and log response)
func (p *TestProxy) SetCaptureMode(enabled bool) {
	p.interceptor.mu.Lock()
	defer p.interceptor.mu.Unlock()
	p.interceptor.captureMode = enabled
	if enabled {
		p.logFunc("🎯 CAPTURE MODE ENABLED - Will forward OAuth to real API and log response")
	}
}

// SetShouldFailExchange configures whether OAuth token exchange should fail
func (p *TestProxy) SetShouldFailExchange(shouldFail bool) {
	p.interceptor.mu.Lock()
	defer p.interceptor.mu.Unlock()
	p.interceptor.shouldFailExchange = shouldFail
}

// SetExchangeDelay sets a delay for OAuth token exchange responses
func (p *TestProxy) SetExchangeDelay(delay time.Duration) {
	p.interceptor.mu.Lock()
	defer p.interceptor.mu.Unlock()
	p.interceptor.exchangeDelay = delay
}

// Close stops the proxy server
func (p *TestProxy) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Close all tracked connections
	for _, conn := range p.connections {
		conn.Close()
	}

	if p.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = p.server.Shutdown(ctx)
	}

	if p.listener != nil {
		return p.listener.Close()
	}

	return nil
}

// ServeHTTP implements the http.Handler interface for the proxy
func (p *TestProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.logFunc("🔍 Proxy request: %s %s", r.Method, r.URL.String())

	// Track connection for cleanup
	if r.Body != nil {
		defer r.Body.Close()
	}

	// Check for CONNECT requests (used for HTTPS tunneling)
	if r.Method == http.MethodConnect {
		p.handleMITM(w, r)
		return
	}

	// Handle regular HTTP requests (non-CONNECT)
	p.forwardRequest(w, r)
}

// handleMITM performs Man-in-the-Middle SSL interception
func (p *TestProxy) handleMITM(w http.ResponseWriter, r *http.Request) {
	p.logFunc("🕵️ Performing MITM for %s", r.Host)

	// Hijack the connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer clientConn.Close()

	// Send HTTP 200 Connection Established
	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		p.logFunc("❌ Failed to send connection established: %v", err)
		return
	}

	// Perform TLS handshake with client using our cert
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{p.cert},
		MinVersion:   tls.VersionTLS12,
	}
	tlsClientConn := tls.Server(clientConn, tlsConfig)
	defer tlsClientConn.Close()

	if err := tlsClientConn.Handshake(); err != nil {
		p.logFunc("❌ TLS handshake failed: %v", err)
		return
	}

	p.logFunc("✅ TLS handshake successful, now intercepting requests")

	// Read and handle HTTP requests from the decrypted connection
	reader := io.Reader(tlsClientConn)
	requestNum := 0
	for {
		requestNum++
		req, err := http.ReadRequest(bufio.NewReader(reader))
		if err != nil {
			if err != io.EOF {
				p.logFunc("⚠️ Error reading request: %v", err)
			}
			return
		}

		// Reconstruct full URL
		req.URL.Scheme = "https"
		req.URL.Host = r.Host

		p.logFunc("🔍 MITM request #%d: %s %s (Host: %s)", requestNum, req.Method, req.URL.Path, r.Host)

		// Create a response writer that writes to the TLS connection
		resp := &mitmResponseWriter{
			conn:   tlsClientConn,
			header: make(http.Header),
		}

		// Mock CHANGELOG.md to avoid external GitHub dependency
		if strings.Contains(req.URL.Path, "/CHANGELOG.md") {
			p.logFunc("🎯 Mocking CHANGELOG.md request")
			resp.Header().Set("Content-Type", "text/plain")
			resp.WriteHeader(http.StatusOK)
			_, _ = resp.Write([]byte("# Changelog\n\n## v2.0.25\n- Test version\n"))
			return
		}

		// Mock health check endpoints
		if strings.Contains(req.URL.Path, "/api/hello") || strings.Contains(req.URL.Path, "/v1/oauth/hello") {
			p.logFunc("🎯 Mocking health check: %s", req.URL.Path)
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(http.StatusOK)
			_, _ = resp.Write([]byte(`{"status":"ok"}`))
			return
		}

		// Check if this is an OAuth token exchange
		if strings.Contains(req.URL.Path, "/oauth/token") || strings.Contains(req.URL.Path, "/api/auth/token") {
			p.logFunc("🎯 Intercepted OAuth token exchange: %s", req.URL.Path)
			p.interceptor.handleTokenExchange(resp, req)
			return
		}

		// Check if this is an OAuth profile request
		if strings.Contains(req.URL.Path, "/api/oauth/profile") {
			p.logFunc("🎯 Intercepted OAuth profile request: %s", req.URL.Path)
			p.interceptor.handleProfileRequest(resp, req)
			return
		}

		// Check if this is a Claude CLI roles request
		if strings.Contains(req.URL.Path, "/api/oauth/claude_cli/roles") {
			p.logFunc("🎯 Intercepted Claude CLI roles request: %s", req.URL.Path)
			p.interceptor.handleRolesRequest(resp, req)
			return
		}

		// Log and forward other requests to real API
		p.logFunc("⚠️ Forwarding unhandled request to real API: %s %s", req.Method, req.URL.Path)
		p.forwardMITMRequest(tlsClientConn, req)
	}
}

// mitmResponseWriter implements http.ResponseWriter for MITM responses
type mitmResponseWriter struct {
	conn          net.Conn
	header        http.Header
	statusCode    int
	wroteHeader   bool
	contentLength int
}

func (w *mitmResponseWriter) Header() http.Header {
	return w.header
}

func (w *mitmResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.statusCode = statusCode
	w.wroteHeader = true
}

func (w *mitmResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	// Write HTTP response
	if w.contentLength == 0 {
		// First write - send status line and headers
		_, _ = fmt.Fprintf(w.conn, "HTTP/1.1 %d %s\r\n", w.statusCode, http.StatusText(w.statusCode))
		_ = w.header.Write(w.conn)
		_, _ = fmt.Fprintf(w.conn, "\r\n")
	}

	n, err := w.conn.Write(data)
	w.contentLength += n
	return n, err
}

// handleTokenExchange intercepts and handles OAuth token exchange
func (i *OAuthInterceptor) handleTokenExchange(w http.ResponseWriter, r *http.Request) {
	i.mu.Lock()
	i.codeExchangeCount++
	i.mu.Unlock()

	i.logFunc("🔐 Intercepting OAuth token exchange (attempt %d)", i.codeExchangeCount)

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request", http.StatusInternalServerError)
		return
	}

	i.logFunc("📤 OAuth Request Body: %s", string(body))

	// Parse the request body to extract code
	values, err := url.ParseQuery(string(body))
	if err == nil {
		if code := values.Get("code"); code != "" {
			i.logFunc("📝 Captured OAuth code: %s", code[:minInt(len(code), 30)]+"...")
			i.mu.Lock()
			i.lastCode = code
			i.mu.Unlock()
		}
	}

	// Check if we should forward to real API (capture mode)
	i.mu.RLock()
	captureMode := i.captureMode
	i.mu.RUnlock()

	if captureMode {
		i.logFunc("🌐 CAPTURE MODE: Forwarding to real console.anthropic.com")
		realURL := "https://console.anthropic.com/v1/oauth/token"
		i.forwardToRealAPI(w, r, body, realURL)
		return
	}

	// Handle delay if configured
	i.mu.RLock()
	delay := i.exchangeDelay
	i.mu.RUnlock()
	if delay > 0 {
		i.logFunc("⏱️ Delaying OAuth response by %v", delay)
		time.Sleep(delay)
	}

	// Check if we should fail the exchange
	i.mu.RLock()
	shouldFail := i.shouldFailExchange
	i.mu.RUnlock()

	if shouldFail {
		i.logFunc("❌ Returning OAuth error (configured to fail)")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"error": "invalid_grant", "error_description": "Invalid authentication code"}`)
		return
	}

	// Return mock success response (matching real API structure)
	i.logFunc("✅ Returning mock OAuth token")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{
		"token_type": "Bearer",
		"access_token": "sk-ant-oat01-nNWB7BsoGHX5njIHyRqwPDeHJ85Y4IHPUi3a5BWRG3Wex7yYEl_-8wPnbmxcUx0031s3YZYvU_t3Kp3BpTYEMw-TestMockAA",
		"expires_in": 28800,
		"refresh_token": "sk-ant-ort01-pd6YwRMEfrkLfNDJPxawCxzUdyC1tIxrKa9aDEj54quHQaDOjb15-IYPONyMjzE-hHJS-oxH9GYqZmhlnmftpQ-TestMockAA",
		"scope": "user:inference user:profile",
		"organization": {
			"uuid": "00000000-0000-0000-0000-000000000001",
			"name": "Test Organization"
		},
		"account": {
			"uuid": "00000000-0000-0000-0000-000000000002",
			"email_address": "test@example.com"
		}
	}`)
}

// handleProfileRequest intercepts and handles OAuth profile requests
func (i *OAuthInterceptor) handleProfileRequest(w http.ResponseWriter, r *http.Request) {
	i.logFunc("🔐 Intercepting OAuth profile request")

	// Read request body if present
	body := []byte{}
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request", http.StatusInternalServerError)
			return
		}
	}

	// Check if we should forward to real API (capture mode)
	i.mu.RLock()
	captureMode := i.captureMode
	i.mu.RUnlock()

	if captureMode {
		i.logFunc("🌐 CAPTURE MODE: Forwarding profile request to real api.anthropic.com")
		realURL := "https://api.anthropic.com/api/oauth/profile"
		i.forwardToRealAPI(w, r, body, realURL)
		return
	}

	// Return mock profile response (matching real API structure)
	i.logFunc("✅ Returning mock OAuth profile")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{
		"account": {
			"uuid": "00000000-0000-0000-0000-000000000002",
			"full_name": "Test User",
			"display_name": "Test",
			"email": "test@example.com",
			"has_claude_max": true,
			"has_claude_pro": false
		},
		"organization": {
			"uuid": "00000000-0000-0000-0000-000000000001",
			"name": "Test Organization",
			"organization_type": "claude_max",
			"billing_type": "stripe_subscription",
			"rate_limit_tier": "default_claude_max_5x"
		}
	}`)
}

// decompressBody decompresses a response body based on Content-Encoding
func decompressBody(body []byte, encoding string) ([]byte, error) {
	switch strings.ToLower(encoding) {
	case "br":
		// Brotli decompression
		reader := brotli.NewReader(bytes.NewReader(body))
		return io.ReadAll(reader)
	case "gzip":
		// Gzip decompression
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	default:
		// No compression or unknown encoding
		return body, nil
	}
}

// handleRolesRequest intercepts and handles Claude CLI roles requests
func (i *OAuthInterceptor) handleRolesRequest(w http.ResponseWriter, r *http.Request) {
	i.logFunc("🔐 Intercepting Claude CLI roles request")

	// Read request body if present
	body := []byte{}
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request", http.StatusInternalServerError)
			return
		}
	}

	// Check if we should forward to real API (capture mode)
	i.mu.RLock()
	captureMode := i.captureMode
	i.mu.RUnlock()

	if captureMode {
		i.logFunc("🌐 CAPTURE MODE: Forwarding roles request to real api.anthropic.com")
		realURL := "https://api.anthropic.com/api/oauth/claude_cli/roles"
		i.forwardToRealAPI(w, r, body, realURL)
		return
	}

	// Return mock roles response - grant all permissions for testing
	i.logFunc("✅ Returning mock Claude CLI roles")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{
		"roles": ["user"],
		"permissions": ["claude_code:use", "claude_code:full_access"]
	}`)
}

// forwardToRealAPI forwards the OAuth request to the real API and logs the response
func (i *OAuthInterceptor) forwardToRealAPI(w http.ResponseWriter, r *http.Request, body []byte, apiURL string) {
	// Create request to real API
	req, err := http.NewRequest(r.Method, apiURL, bytes.NewBuffer(body))
	if err != nil {
		i.logFunc("❌ Failed to create request: %v", err)
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	req.Header = r.Header.Clone()
	// Update Host header to match the real API
	parsedURL, _ := url.Parse(apiURL)
	req.Header.Set("Host", parsedURL.Host)

	// Create HTTP client
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Send request
	i.logFunc("🌐 Sending request to real API: %s", apiURL)
	resp, err := client.Do(req)
	if err != nil {
		i.logFunc("❌ Request failed: %v", err)
		http.Error(w, "Request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Read response (compressed)
	compressedBody, err := io.ReadAll(resp.Body)
	if err != nil {
		i.logFunc("❌ Failed to read response: %v", err)
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	// Decompress the response body for logging
	encoding := resp.Header.Get("Content-Encoding")
	decompressedBody := compressedBody
	if encoding != "" {
		var err error
		decompressedBody, err = decompressBody(compressedBody, encoding)
		if err != nil {
			i.logFunc("⚠️ Failed to decompress response (encoding: %s): %v", encoding, err)
			decompressedBody = compressedBody // fallback to showing compressed
		}
	}

	// Log the FULL response
	i.logFunc("%s", strings.Repeat("=", 80))
	i.logFunc("🎯 CAPTURED REAL API RESPONSE: %s", apiURL)
	i.logFunc("%s", strings.Repeat("=", 80))
	i.logFunc("📥 Status: %d %s", resp.StatusCode, resp.Status)
	i.logFunc("📥 Headers:")
	for key, values := range resp.Header {
		for _, value := range values {
			i.logFunc("   %s: %s", key, value)
		}
	}
	i.logFunc("📥 Body (%d bytes, encoding: %s):", len(decompressedBody), encoding)
	i.logFunc("%s", string(decompressedBody))
	i.logFunc("%s", strings.Repeat("=", 80))

	// Pretty print JSON if possible
	var jsonData map[string]interface{}
	if err := json.Unmarshal(decompressedBody, &jsonData); err == nil {
		prettyJSON, _ := json.MarshalIndent(jsonData, "", "  ")
		i.logFunc("📋 Pretty JSON:")
		i.logFunc("%s", string(prettyJSON))
		i.logFunc("%s", strings.Repeat("=", 80))
	}

	// Forward response to client (use original compressed body)
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(compressedBody)
}

// forwardRequest forwards a regular HTTP request
func (p *TestProxy) forwardRequest(w http.ResponseWriter, r *http.Request) {
	// Create client request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Forward the request
	resp, err := client.Do(r)
	if err != nil {
		p.logFunc("❌ Failed to forward request: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Write status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	_, _ = io.Copy(w, resp.Body)
}

// forwardMITMRequest forwards a request from MITM connection
func (p *TestProxy) forwardMITMRequest(conn net.Conn, req *http.Request) {
	// Create HTTP client
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Clear RequestURI (it's set when reading but must be empty for client requests)
	req.RequestURI = ""

	// Forward the request
	resp, err := client.Do(req)
	if err != nil {
		p.logFunc("❌ Failed to forward MITM request: %v", err)
		return
	}
	defer resp.Body.Close()

	// Write response to connection
	_ = resp.Write(conn)
}

// generateSelfSignedCert generates a self-signed certificate for MITM
func generateSelfSignedCert() (tls.Certificate, error) {
	// Generate private key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Create certificate template
	notBefore := time.Now()
	notAfter := notBefore.Add(24 * time.Hour)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Claude Test Proxy"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"*.anthropic.com", "console.anthropic.com", "api.anthropic.com", "*.claude.ai", "claude.ai"},
	}

	// Create self-signed certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	// Load as tls.Certificate
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, err
	}

	return cert, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
