package handlers

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/services"
)

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

	// Only proxy to healthy HTTP services
	if service.ServiceType != "http" || service.Health != "healthy" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": fmt.Sprintf("Port %d is not a healthy HTTP service", port),
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

	// Copy response headers
	for name, values := range resp.Header {
		for _, value := range values {
			c.Response().Header.Add(name, value)
		}
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
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") && c.Get("X-Disable-HTML-Modification") == "" {
		modifiedBody := h.modifyHTMLContent(string(body), port)
		return c.SendString(modifiedBody)
	}

	// Return response as-is
	return c.Send(body)
}

// modifyHTMLContent injects base tag and JavaScript to handle SPA routing
func (h *ProxyHandler) modifyHTMLContent(content string, port int) string {
	basePath := fmt.Sprintf("/%d/", port)
	
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
	
	// Inject JavaScript for SPA support
	jsCode := fmt.Sprintf(`
<script>
(function() {
    const basePath = '%s';
    
    // Helper function to get base path
    window.getProxyBasePath = function() {
        return basePath;
    };
    
    // Override pushState and replaceState
    const originalPushState = history.pushState;
    const originalReplaceState = history.replaceState;
    
    history.pushState = function(state, title, url) {
        if (url && typeof url === 'string' && url.startsWith('/') && !url.startsWith(basePath)) {
            url = basePath.slice(0, -1) + url;
        }
        return originalPushState.call(history, state, title, url);
    };
    
    history.replaceState = function(state, title, url) {
        if (url && typeof url === 'string' && url.startsWith('/') && !url.startsWith(basePath)) {
            url = basePath.slice(0, -1) + url;
        }
        return originalReplaceState.call(history, state, title, url);
    };
    
    // Fix relative links on page load
    document.addEventListener('DOMContentLoaded', function() {
        const links = document.querySelectorAll('a[href^="/"]');
        links.forEach(function(link) {
            const href = link.getAttribute('href');
            if (href && !href.startsWith(basePath)) {
                link.setAttribute('href', basePath.slice(0, -1) + href);
            }
        });
    });
})();
</script>`, basePath)
	
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
	
	log.Printf("🔧 Modified HTML response for port %d with base path %s", port, basePath)
	return content
}