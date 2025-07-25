package handlers

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/assets"
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
		}
	}
}

// isRewritable determines if a URL is an absolute path that should be rewritten
func isRewritable(val string, basePath string) bool {
	return strings.HasPrefix(val, "/") && !strings.HasPrefix(val, basePath)
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
				if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, basePath+"/") {
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
				if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, basePath+"/") {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// Vite imports without space: import"./chunk-XXX.js" or import"/node_modules/..."
		{
			regex: regexp.MustCompile("import['\"`]([^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, basePath+"/") {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// export without space: export"..." or export*from"..."
		{
			regex: regexp.MustCompile("export(?:\\*?from)?['\"`]([^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, basePath+"/") {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// Export from: export ... from "/path" -> export ... from "/PORT/path"
		{
			regex: regexp.MustCompile("export\\s+[^'\"]*from\\s*['\"`]([^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, basePath+"/") {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// Fetch calls: fetch("/path") -> fetch("/PORT/path")
		{
			regex: regexp.MustCompile("fetch\\s*\\(\\s*['\"`]([^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, basePath+"/") {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// XMLHttpRequest.open: xhr.open("GET", "/path") -> xhr.open("GET", "/PORT/path")
		{
			regex: regexp.MustCompile("\\.open\\s*\\(\\s*['\"`][^'\"`]*['\"`]\\s*,\\s*['\"`]([^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, basePath+"/") {
					return strings.Replace(match, path, basePath+path, 1)
				}
				return match
			},
		},
		// new URL("/path", ...) -> new URL("/PORT/path", ...)
		{
			regex: regexp.MustCompile("new\\s+URL\\s*\\(\\s*['\"`]([^'\"`]+)['\"`]"),
			replace: func(match, path string) string {
				if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, basePath+"/") {
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
				if !strings.HasPrefix(path, basePath+"/") {
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

	// Only log if we actually made changes
	if content != originalContent {
		log.Printf("🔧 Modified JavaScript response for port %d with base path %s", port, basePath)
		// Log first few import/export statements for debugging
		importMatches := regexp.MustCompile("(?:import|export)[^;{]+[;{]").FindAllString(content, 5)
		if len(importMatches) > 0 {
			log.Printf("   Sample imports after rewriting: %v", importMatches)
		}
	}

	return content
}
