package handlers

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	gorilla_websocket "github.com/gorilla/websocket"
	"github.com/vanpelt/catnip/internal/assets"
	"github.com/vanpelt/catnip/internal/recovery"
	"github.com/vanpelt/catnip/internal/services"
)

// rewriteHTMLAbsolutePaths rewrites absolute paths in HTML src/href attributes to use the proxy base path.
func rewriteHTMLAbsolutePaths(htmlContent string, basePath string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		log.Printf("❌ Failed to parse HTML: %v", err)
		return htmlContent
	}

	rewriteNodeURLs(doc, basePath)

	var buf bytes.Buffer
	err = html.Render(&buf, doc)
	if err != nil {
		log.Printf("❌ Failed to render modified HTML: %v", err)
		return htmlContent
	}

	return buf.String()
}

// rewriteNodeURLs recursively walks nodes and rewrites absolute src/href URLs
func rewriteNodeURLs(n *html.Node, basePath string) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "script", "img", "iframe":
			rewriteAttribute(n, "src", basePath)
		case "link", "a", "form":
			rewriteAttribute(n, "href", basePath)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		rewriteNodeURLs(c, basePath)
	}
}

// rewriteAttribute modifies the given attribute if it's an absolute URL starting with /
func rewriteAttribute(n *html.Node, attrName string, basePath string) {
	for i, attr := range n.Attr {
		if attr.Key == attrName && isRewritable(attr.Val, basePath) {
			newVal := basePath + strings.TrimPrefix(attr.Val, "/")
			// Preserve query strings or fragments
			if u, err := url.Parse(attr.Val); err == nil {
				newVal = basePath + strings.TrimPrefix(u.Path, "/")
				if u.RawQuery != "" {
					newVal += "?" + u.RawQuery
				}
				if u.Fragment != "" {
					newVal += "#" + u.Fragment
				}
			}
			n.Attr[i].Val = newVal
		} else if attr.Key == attrName && isWebSocketRewritable(attr.Val, basePath) {
			// Handle WebSocket URLs (ws:// and wss://)
			if u, err := url.Parse(attr.Val); err == nil {
				// Only rewrite same-host WebSocket URLs with absolute paths
				if (u.Scheme == "ws" || u.Scheme == "wss") &&
					strings.HasPrefix(u.Path, "/") &&
					!strings.HasPrefix(u.Path, basePath) {
					u.Path = basePath + strings.TrimPrefix(u.Path, "/")
					n.Attr[i].Val = u.String()
				}
			}
		} else if attr.Key == attrName && isLocalhostPortRewritable(attr.Val, basePath) {
			// Handle localhost:xxxx URLs - rewrite to localhost:8080/xxxx
			if newVal := rewriteLocalhostPort(attr.Val, basePath); newVal != attr.Val {
				n.Attr[i].Val = newVal
			}
		}
	}
}

// isRewritable determines if a URL is an absolute path that should be rewritten
func isRewritable(val string, basePath string) bool {
	// Must start with / and not already have basePath
	if !strings.HasPrefix(val, "/") || strings.HasPrefix(val, basePath) {
		return false
	}

	// Skip URLs that already start with a 4-digit port pattern (e.g., /3030/...)
	// This prevents double-prefixing URLs that were already rewritten by localhost logic
	portPattern := regexp.MustCompile(`^/\d{4}/`)
	return !portPattern.MatchString(val)
}

// isWebSocketRewritable determines if a URL is a WebSocket URL that should be rewritten
func isWebSocketRewritable(val string, basePath string) bool {
	return strings.HasPrefix(val, "ws://") || strings.HasPrefix(val, "wss://")
}

// isLocalhostPortRewritable determines if a URL is a localhost:port URL that should be rewritten
func isLocalhostPortRewritable(val string, basePath string) bool {
	// Match http://localhost:xxxx, https://localhost:xxxx, ws://localhost:xxxx, wss://localhost:xxxx
	return regexp.MustCompile(`^(https?|wss?)://localhost:\d+`).MatchString(val)
}

// rewriteLocalhostPort rewrites localhost:xxxx URLs to use the proxy base path
func rewriteLocalhostPort(val string, basePath string) string {
	// Parse the URL
	u, err := url.Parse(val)
	if err != nil {
		return val
	}

	// Only rewrite localhost URLs with specific ports
	if u.Hostname() != "localhost" || u.Port() == "" {
		return val
	}

	// Extract the port
	port := u.Port()

	// Skip if it's already our proxy port (8080)
	if port == "8080" {
		return val
	}

	// Rewrite to use localhost:8080/PORT/path format
	newPath := "/" + port
	if u.Path != "" && u.Path != "/" {
		newPath += u.Path
	}

	// Preserve query and fragment
	if u.RawQuery != "" {
		newPath += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		newPath += "#" + u.Fragment
	}

	// Return the rewritten URL
	return fmt.Sprintf("%s://localhost:8080%s", u.Scheme, newPath)
}

// ProxyHandler handles reverse proxy requests to detected services
type ProxyHandler struct {
	monitor *services.PortMonitor
}

// NewProxyHandler creates a new proxy handler
func NewProxyHandler(monitor *services.PortMonitor) *ProxyHandler {
	return &ProxyHandler{
		monitor: monitor,
	}
}

// ProxyToPort handles requests to /$PORT/* and proxies them to localhost:$PORT
func (h *ProxyHandler) ProxyToPort(c *fiber.Ctx) error {
	portParam := c.Params("port")
	port, err := strconv.Atoi(portParam)
	if err != nil {
		// If this isn't a valid port number, let other handlers handle it
		return c.Next()
	}

	// Validate port range
	if port < 1 || port > 65535 {
		return c.Next()
	}

	// Check if the port is in our detected services
	services := h.monitor.GetServices()
	service, exists := services[port]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fmt.Sprintf("Port %d is not active or detected", port),
		})
	}

	// Only proxy to HTTP services (health status doesn't matter for proxying)
	if service.ServiceType != "http" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": fmt.Sprintf("Port %d is not an HTTP service", port),
		})
	}

	// Check if this is a WebSocket upgrade request
	if strings.ToLower(c.Get("Connection")) == "upgrade" &&
		strings.ToLower(c.Get("Upgrade")) == "websocket" {
		return h.handleWebSocketProxyWithFiber(c, port)
	}

	// Get the path after the port
	path := c.Params("*")
	if path == "" {
		path = "/"
	} else {
		path = "/" + path
	}

	// Build target URL
	targetURL := fmt.Sprintf("http://localhost:%d%s", port, path)

	// Add query parameters
	if c.Request().URI().QueryString() != nil {
		targetURL += "?" + string(c.Request().URI().QueryString())
	}

	// Create HTTP client
	client := &http.Client{}

	// Create request
	req, err := http.NewRequest(string(c.Request().Header.Method()), targetURL, bytes.NewReader(c.Body()))
	if err != nil {
		log.Printf("❌ Error creating proxy request: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create proxy request",
		})
	}

	// Copy headers from original request
	c.Request().Header.VisitAll(func(key, value []byte) {
		keyStr := string(key)
		valueStr := string(value)

		// Skip headers that shouldn't be forwarded
		if keyStr == "Host" || keyStr == "Connection" || keyStr == "Content-Length" {
			return
		}

		req.Header.Set(keyStr, valueStr)
	})

	// Set proper host header
	req.Host = fmt.Sprintf("localhost:%d", port)

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Error making proxy request to %s: %v", targetURL, err)
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "Failed to connect to service",
		})
	}
	defer resp.Body.Close()

	// Set response status
	c.Status(resp.StatusCode)

	// Get content type for later use
	contentType := resp.Header.Get("Content-Type")

	// Copy response headers
	for name, values := range resp.Header {
		for _, value := range values {
			if name != "Access-Control-Allow-Origin" && name != "Access-Control-Allow-Credentials" {
				c.Response().Header.Add(name, value)
			}
		}
	}

	// Add Service-Worker-Allowed header for JavaScript responses
	if strings.Contains(strings.ToLower(contentType), "javascript") ||
		strings.Contains(strings.ToLower(contentType), "application/javascript") ||
		strings.Contains(strings.ToLower(contentType), "text/javascript") {
		c.Response().Header.Set("Service-Worker-Allowed", fmt.Sprintf("/%d/", port))
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("❌ Error reading proxy response: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read service response",
		})
	}

	// Check if we should modify HTML content
	if strings.Contains(contentType, "text/html") && c.Get("X-Disable-HTML-Modification") == "" {
		modifiedBody := h.modifyHTMLContent(string(body), port)
		return c.SendString(modifiedBody)
	}

	// Check if we should modify JavaScript content
	if (strings.Contains(strings.ToLower(contentType), "javascript") ||
		strings.Contains(strings.ToLower(contentType), "application/javascript") ||
		strings.Contains(strings.ToLower(contentType), "text/javascript")) &&
		c.Get("X-Disable-JS-Modification") == "" {
		modifiedBody := h.modifyJavaScriptContent(string(body), port)
		return c.SendString(modifiedBody)
	}

	// Return response as-is
	return c.Send(body)
}

// modifyHTMLContent injects base tag and JavaScript to handle SPA routing
func (h *ProxyHandler) modifyHTMLContent(content string, port int) string {
	basePath := fmt.Sprintf("/%d/", port)

	// Rewrite absolute paths in HTML content
	content = rewriteHTMLAbsolutePaths(content, basePath)

	// Rewrite imports in inline script tags (e.g., Vite's @react-refresh)
	scriptRegex := regexp.MustCompile(`<script[^>]*>([\s\S]*?)</script>`)
	content = scriptRegex.ReplaceAllStringFunc(content, func(match string) string {
		// Check if it's a module script with imports
		if strings.Contains(match, "import") {
			// Rewrite paths like "/@react-refresh" to "/PORT/@react-refresh"
			importRegex := regexp.MustCompile(`from\s*["'](\/@[^"']+)["']`)
			match = importRegex.ReplaceAllStringFunc(match, func(importMatch string) string {
				if m := importRegex.FindStringSubmatch(importMatch); len(m) > 1 {
					path := m[1]
					if isRewritable(path, basePath) {
						return strings.Replace(importMatch, path, basePath+strings.TrimPrefix(path, "/"), 1)
					}
				}
				return importMatch
			})
		}
		return match
	})

	// Inject base tag and early variable declaration
	baseTag := fmt.Sprintf(`<base href="%s">`, basePath)
	earlyScript := fmt.Sprintf(`<script>window.__PROXY_BASE_PATH__ = '%s';</script>`, basePath)

	// Find head tag and inject base tag and early script
	headRegex := regexp.MustCompile(`<head[^>]*>`)
	if headRegex.MatchString(content) {
		content = headRegex.ReplaceAllStringFunc(content, func(match string) string {
			return match + "\n" + baseTag + "\n" + earlyScript
		})
	}

	// Get the proxy injection script from embedded assets
	proxyScript, err := assets.GetProxyInjectionScript()
	if err != nil {
		log.Printf("❌ Failed to load proxy injection script: %v, falling back to basic injection", err)
		// Fallback to minimal script injection
		jsCode := fmt.Sprintf(`<script>console.log("Catnip proxy active for %s");</script>`, basePath)
		content += jsCode
	} else {
		// Inject the full proxy script
		jsCode := fmt.Sprintf(`<script>%s</script>`, string(proxyScript))

		// Inject before closing body tag
		bodyRegex := regexp.MustCompile(`</body>`)
		if bodyRegex.MatchString(content) {
			content = bodyRegex.ReplaceAllStringFunc(content, func(match string) string {
				return jsCode + "\n" + match
			})
		} else {
			// If no body tag, append to end
			content += jsCode
		}
	}

	log.Printf("🔧 Modified HTML response for port %d with base path %s", port, basePath)
	return content
}

// modifyJavaScriptContent rewrites import paths and other absolute paths in JavaScript content
func (h *ProxyHandler) modifyJavaScriptContent(content string, port int) string {
	basePath := fmt.Sprintf("/%d", port)

	// Regex patterns to match various import and path patterns in JavaScript
	patterns := []struct {
		regex   *regexp.Regexp
		replace func(match string, path string) string
	}{
		// Dynamic imports: import("/path") -> import("/PORT/path")
		{
			regex: regexp.MustCompile("import\\s*\\(\\s*['\"`]([^'\"`]+)['\"`]\\s*\\)"),
			replace: func(match, path string) string {
				if isRewritable(path, basePath) {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// Bare module specifiers (no leading ./ or /) that might resolve to absolute paths
		// This handles cases like: import "chunk-XXX.js"
		{
			regex: regexp.MustCompile("(?:import|export)\\s*[^'\"]*['\"`]([^/][^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				// Check if this looks like a Vite chunk or node_modules reference
				if strings.Contains(path, "chunk-") || strings.Contains(path, "node_modules") {
					// Don't modify - let browser resolve it
					return match
				}
				return match
			},
		},
		// Static imports: import ... from "/path" -> import ... from "/PORT/path"
		{
			regex: regexp.MustCompile("import\\s+[^'\"]*from\\s*['\"`]([^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				if isRewritable(path, basePath) {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// Vite imports without space: import"./chunk-XXX.js" or import"/node_modules/..."
		{
			regex: regexp.MustCompile("import['\"`]([^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				if isRewritable(path, basePath) {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// export without space: export"..." or export*from"..."
		{
			regex: regexp.MustCompile("export(?:\\*?from)?['\"`]([^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				if isRewritable(path, basePath) {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// Export from: export ... from "/path" -> export ... from "/PORT/path"
		{
			regex: regexp.MustCompile("export\\s+[^'\"]*from\\s*['\"`]([^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				if isRewritable(path, basePath) {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// Fetch calls: fetch("/path") -> fetch("/PORT/path")
		{
			regex: regexp.MustCompile("fetch\\s*\\(\\s*['\"`]([^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				if isRewritable(path, basePath) {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// XMLHttpRequest.open: xhr.open("GET", "/path") -> xhr.open("GET", "/PORT/path")
		{
			regex: regexp.MustCompile("\\.open\\s*\\(\\s*['\"`][^'\"`]*['\"`]\\s*,\\s*['\"`]([^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				if isRewritable(path, basePath) {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// new URL("/path", ...) -> new URL("/PORT/path", ...)
		{
			regex: regexp.MustCompile("new\\s+URL\\s*\\(\\s*['\"`]([^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				if isRewritable(path, basePath) {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// new WebSocket("ws://host/path") or new WebSocket("wss://host/path") -> rewrite path
		{
			regex: regexp.MustCompile("new\\s+WebSocket\\s*\\(\\s*['\"`](wss?://[^'\"`]+)['\"`]"),
			replace: func(match, wsUrl string) string {
				if u, err := url.Parse(wsUrl); err == nil {
					if isRewritable(u.Path, basePath) {
						u.Path = basePath + u.Path
						return strings.Replace(match, wsUrl, u.String(), 1)
					}
				}
				return match
			},
		},
		// new WebSocket("/path") -> new WebSocket("ws://host/PORT/path")
		{
			regex: regexp.MustCompile("new\\s+WebSocket\\s*\\(\\s*['\"`]([^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				if isRewritable(path, basePath) {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// String literals that look like absolute paths (be more conservative here)
		// This catches cases like: const path = "/api/data"
		{
			regex: regexp.MustCompile("['\"`](/(?:api|assets|static|public|src|dist|build|node_modules)[^'\"`]*)['\"`]"),
			replace: func(match, path string) string {
				if isRewritable(path, basePath) {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// Vite-specific imports like /@react-refresh, /@vite/client
		{
			regex: regexp.MustCompile("['\"`](/@[^'\"`]*)['\"`]"),
			replace: func(match, path string) string {
				if isRewritable(path, basePath) {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// Special handling for Vite's node_modules paths without quotes in some contexts
		// This catches: importnode_modules or similar concatenated paths
		{
			regex: regexp.MustCompile("(import|export|from)(['\"`]?)(/node_modules[^'\"`\\s]*)"),
			replace: func(match, path string) string {
				// Extract all parts from the match
				submatches := regexp.MustCompile("(import|export|from)(['\"`]?)(/node_modules[^'\"`\\s]*)").FindStringSubmatch(match)
				if len(submatches) == 4 {
					keyword := submatches[1]
					quote := submatches[2]
					nodePath := submatches[3]
					if !strings.HasPrefix(nodePath, basePath+"/") {
						return keyword + quote + basePath + nodePath
					}
				}
				return match
			},
		},
		// Localhost URLs: "http://localhost:5173" -> "http://localhost:8080/5173"
		{
			regex: regexp.MustCompile("(['\"`])(https?://localhost:\\d+[^'\"`]*)(['\"`])"),
			replace: func(match, _ string) string {
				// Extract all submatches to get quote and URL
				submatches := regexp.MustCompile("(['\"`])(https?://localhost:\\d+[^'\"`]*)(['\"`])").FindStringSubmatch(match)
				if len(submatches) >= 4 {
					quote := submatches[1]
					originalURL := submatches[2]
					rewrittenURL := rewriteLocalhostPort(originalURL, basePath)
					log.Printf("🔄 Rewriting localhost URL: %s -> %s", originalURL, rewrittenURL)
					return quote + rewrittenURL + quote
				}
				return match
			},
		},
		// WebSocket localhost URLs: "ws://localhost:5173" -> "ws://localhost:8080/5173"
		{
			regex: regexp.MustCompile("(['\"`])(wss?://localhost:\\d+[^'\"`]*)(['\"`])"),
			replace: func(match, _ string) string {
				// Extract all submatches to get quote and URL
				submatches := regexp.MustCompile("(['\"`])(wss?://localhost:\\d+[^'\"`]*)(['\"`])").FindStringSubmatch(match)
				if len(submatches) >= 4 {
					quote := submatches[1]
					originalURL := submatches[2]
					rewrittenURL := rewriteLocalhostPort(originalURL, basePath)
					log.Printf("🔄 Rewriting WebSocket localhost URL: %s -> %s", originalURL, rewrittenURL)
					return quote + rewrittenURL + quote
				}
				return match
			},
		},
		// WebSocket localhost URLs in template literals: `ws://localhost:5173` -> `ws://localhost:8080/5173`
		{
			regex: regexp.MustCompile("(`)(wss?://localhost:\\d+[^`]*)(`)"),
			replace: func(match, _ string) string {
				// Extract all submatches
				submatches := regexp.MustCompile("(`)(wss?://localhost:\\d+[^`]*)(`)").FindStringSubmatch(match)
				if len(submatches) >= 4 {
					backtick := submatches[1]
					originalURL := submatches[2]
					closingBacktick := submatches[3]
					rewrittenURL := rewriteLocalhostPort(originalURL, basePath)
					log.Printf("🔄 Rewriting template literal WebSocket URL: %s -> %s", originalURL, rewrittenURL)
					return backtick + rewrittenURL + closingBacktick
				}
				return match
			},
		},
		// WebSocket URLs in more general contexts (without quotes): ws://localhost:5173
		{
			regex: regexp.MustCompile(`(wss?://localhost:\d+[^\s)},;]+)`),
			replace: func(match, _ string) string {
				// The entire match is the URL
				originalURL := match
				if isLocalhostPortRewritable(originalURL, basePath) {
					rewrittenURL := rewriteLocalhostPort(originalURL, basePath)
					log.Printf("🔄 Rewriting unquoted WebSocket URL: %s -> %s", originalURL, rewrittenURL)
					return rewrittenURL
				}
				return match
			},
		},
		// HTTP localhost URLs in more general contexts (without quotes): http://localhost:5173
		{
			regex: regexp.MustCompile(`(https?://localhost:\d+[^\s)},;]+)`),
			replace: func(match, _ string) string {
				// The entire match is the URL
				originalURL := match
				if isLocalhostPortRewritable(originalURL, basePath) {
					rewrittenURL := rewriteLocalhostPort(originalURL, basePath)
					log.Printf("🔄 Rewriting unquoted HTTP URL: %s -> %s", originalURL, rewrittenURL)
					return rewrittenURL
				}
				return match
			},
		},
	}

	originalContent := content

	// Apply all patterns
	for _, pattern := range patterns {
		content = pattern.regex.ReplaceAllStringFunc(content, func(match string) string {
			// Extract the path from the match using the first capture group
			submatches := pattern.regex.FindStringSubmatch(match)
			if len(submatches) > 1 {
				path := submatches[1]
				return pattern.replace(match, path)
			}
			return match
		})
	}

	// Only log if we actually made changes and debug is enabled
	if content != originalContent {
		if os.Getenv("CATNIP_DEBUG") != "" {
			log.Printf("🔧 Modified JavaScript response for port %d with base path %s", port, basePath)
			// Log first few import/export statements for debugging
			importMatches := regexp.MustCompile("(?:import|export)[^;{]+[;{]").FindAllString(content, 5)
			if len(importMatches) > 0 {
				log.Printf("   Sample imports after rewriting: %v", importMatches)
			}
		}
	}

	return content
}

// handleWebSocketProxyWithFiber uses Fiber's built-in WebSocket support for cleaner proxy handling
func (h *ProxyHandler) handleWebSocketProxyWithFiber(c *fiber.Ctx, port int) error {
	// Get the path after the port
	path := c.Params("*")
	if path == "" {
		path = "/" // For /:port route, connect to root path
	} else {
		path = "/" + path // For /:port/* route, preserve the path
	}

	// Build target WebSocket URL
	targetURL := fmt.Sprintf("ws://localhost:%d%s", port, path)

	// Add query parameters
	if c.Request().URI().QueryString() != nil {
		targetURL += "?" + string(c.Request().URI().QueryString())
	}

	log.Printf("🔌 WebSocket proxy request from %s to target: %s", c.Path(), targetURL)

	// Extract headers from the original request BEFORE entering the WebSocket handler
	// because the Fiber context becomes invalid inside the handler
	requestHeader := make(http.Header)
	c.Request().Header.VisitAll(func(key, value []byte) {
		keyStr := string(key)
		valueStr := string(value)
		// Only forward the protocol header - dialer handles all other WebSocket headers automatically
		if strings.ToLower(keyStr) == "sec-websocket-protocol" {
			requestHeader.Set(keyStr, valueStr)
		}
	})

	// Use Fiber's WebSocket handler - this handles the upgrade automatically
	return websocket.New(func(clientConn *websocket.Conn) {
		// Add panic recovery to prevent container crashes
		defer func() {
			if r := recover(); r != nil {
				log.Printf("🚨 PANIC recovered in WebSocket proxy: %v", r)
				if clientConn != nil {
					clientConn.Close()
				}
			}
		}()

		defer clientConn.Close()
		log.Printf("✅ Fiber WebSocket connection established")

		// Create WebSocket dialer to connect to the target
		dialer := gorilla_websocket.Dialer{
			HandshakeTimeout: 5 * time.Second,
		}

		// Connect to target WebSocket server
		log.Printf("🔌 Attempting to dial target WebSocket: %s", targetURL)
		targetConn, _, err := dialer.Dial(targetURL, requestHeader)
		if err != nil {
			log.Printf("🔌❌ WebSocket dial failed: %v", err)
			if closeErr := clientConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "Failed to connect to target")); closeErr != nil {
				log.Printf("❌ Failed to send close message: %v", closeErr)
			}
			return
		}
		defer targetConn.Close()
		log.Printf("✅ Successfully connected to target WebSocket")

		log.Printf("✅ WebSocket proxy established successfully - starting message relay")

		// Start proxying messages between client and target
		h.proxyWebSocketConnectionsSimple(clientConn, targetConn)
	})(c)
}

// proxyWebSocketConnectionsSimple performs bidirectional message proxying between Fiber and Gorilla WebSocket connections
func (h *ProxyHandler) proxyWebSocketConnectionsSimple(fiberConn *websocket.Conn, gorillaConn *gorilla_websocket.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Copy messages from Fiber client to Gorilla target
	recovery.SafeGoWithCleanup("websocket-client-to-target", func() {
		for {
			messageType, data, err := fiberConn.ReadMessage()
			if err != nil {
				if os.Getenv("CATNIP_DEBUG") != "" {
					log.Printf("❌ WebSocket read error from Fiber client: %v", err)
				}
				break
			}

			err = gorillaConn.WriteMessage(messageType, data)
			if err != nil {
				if os.Getenv("CATNIP_DEBUG") != "" {
					log.Printf("❌ WebSocket write error to Gorilla target: %v", err)
				}
				break
			}
		}
	}, func() {
		wg.Done()
		gorillaConn.Close()
	})

	// Copy messages from Gorilla target to Fiber client
	recovery.SafeGoWithCleanup("websocket-target-to-client", func() {
		for {
			messageType, data, err := gorillaConn.ReadMessage()
			if err != nil {
				log.Printf("❌ WebSocket read error from target: %v", err)
				break
			}

			err = fiberConn.WriteMessage(messageType, data)
			if err != nil {
				if os.Getenv("CATNIP_DEBUG") != "" {
					log.Printf("❌ WebSocket write error to Fiber client: %v", err)
				}
				break
			}
		}
	}, func() {
		wg.Done()
		fiberConn.Close()
	})

	wg.Wait()
	log.Printf("🔌 WebSocket proxy connection closed")
}
