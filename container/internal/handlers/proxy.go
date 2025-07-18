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
			// Don't copy CORS headers
			if name != "Access-Control-Allow-Origin" && name != "Access-Control-Allow-Credentials" {
                fmt.Println("Adding header:", name, value)
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

	// Inject JavaScript for SPA support and iframe resizing
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

    function rewriteAttribute(el, attr) {
        const val = el[attr];
        if (!val || typeof val !== 'string') return;

        const originPrefix = location.origin + '/';
        if (val.startsWith(originPrefix)) {
            const relative = val.replace(location.origin, '');
            if (!relative.startsWith(basePath)) {
                el[attr] = basePath.slice(0, -1) + relative;
            }
        }
    }

    function rewriteStaticResources() {
        // Anchor tags
        document.querySelectorAll('a[href^="/"]').forEach(link => {
            const href = link.getAttribute('href');
            if (href && !href.startsWith(basePath)) {
                link.setAttribute('href', basePath.slice(0, -1) + href);
            }
        });

        // Static <script>, <link>, <img>
        document.querySelectorAll('script[src], link[href], img[src]').forEach(el => {
            if (el.tagName === 'SCRIPT' || el.tagName === 'IMG') {
                rewriteAttribute(el, 'src');
            } else if (el.tagName === 'LINK') {
                rewriteAttribute(el, 'href');
            }
        });
    }

    function watchForDynamicInsertions() {
        const observer = new MutationObserver(mutations => {
            mutations.forEach(mutation => {
                mutation.addedNodes.forEach(node => {
                    if (!(node instanceof HTMLElement)) return;

                    if (node.tagName === 'SCRIPT' || node.tagName === 'IMG') {
                        rewriteAttribute(node, 'src');
                    } else if (node.tagName === 'LINK') {
                        rewriteAttribute(node, 'href');
                    } else if (node.tagName === 'A') {
                        const href = node.getAttribute('href');
                        if (href && href.startsWith('/') && !href.startsWith(basePath)) {
                            node.setAttribute('href', basePath.slice(0, -1) + href);
                        }
                    }
                });
            });
        });

        observer.observe(document.documentElement, {
            childList: true,
            subtree: true
        });
    }

    function patchFetchAndXHR() {
        // Patch fetch
        const originalFetch = window.fetch;
        window.fetch = function(resource, init) {
            if (typeof resource === 'string' && resource.startsWith('/') && !resource.startsWith(basePath)) {
                resource = basePath.slice(0, -1) + resource;
            } else if (resource instanceof Request && resource.url.startsWith(location.origin + '/')) {
                const relative = resource.url.replace(location.origin, '');
                if (!relative.startsWith(basePath)) {
                    resource = new Request(basePath.slice(0, -1) + relative, resource);
                }
            }
            return originalFetch(resource, init);
        };

        // Patch XMLHttpRequest
        const originalOpen = XMLHttpRequest.prototype.open;
        XMLHttpRequest.prototype.open = function(method, url, ...args) {
            if (typeof url === 'string' && url.startsWith('/') && !url.startsWith(basePath)) {
                url = basePath.slice(0, -1) + url;
            } else if (url.startsWith(location.origin + '/')) {
                const relative = url.replace(location.origin, '');
                if (!relative.startsWith(basePath)) {
                    url = basePath.slice(0, -1) + relative;
                }
            }
            return originalOpen.call(this, method, url, ...args);
        };
    }

    // Initialize on DOMContentLoaded
    document.addEventListener('DOMContentLoaded', function() {
        rewriteStaticResources();
        watchForDynamicInsertions();
        patchFetchAndXHR();
    });

    /**
     * 🚧 Things NOT Yet Handled:
     *
     * - new Image().src = "/foo.jpg" → you'd need to patch the Image constructor
     * - new EventSource("/stream") → would need to wrap EventSource
     * - import("/module.js") dynamic imports cannot be intercepted easily at runtime
     * - CSS url(/assets/foo.png) — rewriting stylesheet contents is out-of-scope unless you proxy/transform CSS
     * - WebSocket URLs like ws://example.com/...
     * - Form actions (<form action="/post">) if used
     */

    // Iframe resizer functionality
    let isInIframe = false;
    let parentOrigin = null;
    let lastHeight = 0;
    let resizeObserver = null;
    
    // Guards against infinite resize loops
    const MAX_HEIGHT = 50000; // Maximum allowed height
    const MIN_HEIGHT_CHANGE = 10; // Minimum height change to trigger update
    const RATE_LIMIT_MS = 200; // Minimum time between height updates (5 per second)
    const CYCLE_DETECTION_WINDOW = 5; // Number of recent heights to track
    
    let lastUpdateTime = 0;
    let recentHeights = []; // Track recent heights for cycle detection
    let isRateLimited = false;

    // Check if we're in an iframe
    try {
        isInIframe = window.self !== window.top;
    } catch (e) {
        isInIframe = true;
    }

    if (isInIframe) {
        // Listen for setup message from parent
        window.addEventListener('message', function(event) {
            if (event.data?.type === 'catnip-iframe-setup') {
                parentOrigin = event.data.parentOrigin;
                initializeIframeResizer();
            }
        });

        function initializeIframeResizer() {
            // Function to calculate and send height with guards
            function sendHeight() {
                if (!parentOrigin || isRateLimited) return;

                const now = Date.now();
                
                // Rate limiting - enforce minimum time between updates
                if (now - lastUpdateTime < RATE_LIMIT_MS) {
                    return;
                }

                const body = document.body;
                const html = document.documentElement;
                
                // Get the maximum height of the document
                let height = Math.max(
                    body.scrollHeight,
                    body.offsetHeight,
                    html.clientHeight,
                    html.scrollHeight,
                    html.offsetHeight
                );

                // Enforce maximum height limit
                if (height > MAX_HEIGHT) {
                    console.warn('Iframe resizer: Height exceeds maximum, capping at', MAX_HEIGHT);
                    height = MAX_HEIGHT;
                }

                // Only send if height has changed significantly
                if (Math.abs(height - lastHeight) < MIN_HEIGHT_CHANGE) {
                    return;
                }

                // Cycle detection - check if we're oscillating between heights
                if (recentHeights.length >= CYCLE_DETECTION_WINDOW) {
                    const isOscillating = recentHeights.some(h => Math.abs(h - height) < MIN_HEIGHT_CHANGE);
                    if (isOscillating && recentHeights.length > 2) {
                        console.warn('Iframe resizer: Potential oscillation detected, skipping update');
                        return;
                    }
                }

                // Update tracking variables
                recentHeights.push(height);
                if (recentHeights.length > CYCLE_DETECTION_WINDOW) {
                    recentHeights.shift();
                }
                
                lastHeight = height;
                lastUpdateTime = now;

                try {
                    window.parent.postMessage({
                        type: 'catnip-iframe-height',
                        height: height
                    }, parentOrigin);
                } catch (e) {
                    console.error('Iframe resizer: Failed to send height update', e);
                }
            }

            // Send initial height
            document.addEventListener('DOMContentLoaded', function() {
                setTimeout(sendHeight, 100); // Small delay for layout
            });

            // Send height when page is fully loaded
            window.addEventListener('load', function() {
                setTimeout(sendHeight, 100);
            });

            // Use ResizeObserver if available with timeout protection
            if (window.ResizeObserver) {
                let resizeTimeout;
                resizeObserver = new ResizeObserver(function() {
                    // Debounce resize events to prevent excessive calls
                    clearTimeout(resizeTimeout);
                    resizeTimeout = setTimeout(sendHeight, 50);
                });
                resizeObserver.observe(document.body);
                resizeObserver.observe(document.documentElement);
            } else {
                // Fallback: poll for height changes with timeout protection
                let pollInterval = setInterval(function() {
                    if (isRateLimited) {
                        clearInterval(pollInterval);
                        // Restart polling after rate limit cooldown
                        setTimeout(() => {
                            pollInterval = setInterval(sendHeight, 500);
                        }, 2000);
                    } else {
                        sendHeight();
                    }
                }, 500);
            }

            // Listen for dynamic content changes with timeout protection
            if (window.MutationObserver) {
                let mutationTimeout;
                const mutationObserver = new MutationObserver(function() {
                    // Debounce mutation events to prevent excessive calls
                    clearTimeout(mutationTimeout);
                    mutationTimeout = setTimeout(sendHeight, 50);
                });
                mutationObserver.observe(document.body, {
                    childList: true,
                    subtree: true,
                    attributes: true,
                    attributeFilter: ['style', 'class']
                });
            }

            // Send height immediately if already loaded
            if (document.readyState === 'complete') {
                setTimeout(sendHeight, 100);
            }
        }
    }
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
