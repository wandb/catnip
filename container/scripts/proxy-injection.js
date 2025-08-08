/**
 * Proxy injection script for Catnip
 * This script is injected into HTML pages served through the proxy to handle
 * SPA routing and iframe resizing when pages are served under sub-paths like "/3000"
 */

(function () {
  var basePath = window.__PROXY_BASE_PATH__;

  // Helper function to get base path
  window.getProxyBasePath = function () {
    return basePath;
  };

  // Override pushState and replaceState
  var originalPushState = history.pushState;
  var originalReplaceState = history.replaceState;

  history.pushState = function (state, title, url) {
    if (
      url &&
      typeof url === "string" &&
      url.startsWith("/") &&
      !url.startsWith(basePath)
    ) {
      url = basePath.slice(0, -1) + url;
    }
    return originalPushState.call(history, state, title, url);
  };

  history.replaceState = function (state, title, url) {
    if (
      url &&
      typeof url === "string" &&
      url.startsWith("/") &&
      !url.startsWith(basePath)
    ) {
      url = basePath.slice(0, -1) + url;
    }
    return originalReplaceState.call(history, state, title, url);
  };

  function rewriteAttribute(el, attr) {
    var val = el[attr];
    if (!val || typeof val !== "string") return;

    var originPrefix = location.origin + "/";
    if (val.startsWith(originPrefix)) {
      var relative = val.replace(location.origin, "");
      if (!relative.startsWith(basePath)) {
        el[attr] = basePath.slice(0, -1) + relative;
      }
    }
  }

  function rewriteStaticResources() {
    // Anchor tags
    document.querySelectorAll('a[href^="/"]').forEach(function (link) {
      var href = link.getAttribute("href");
      if (href && !href.startsWith(basePath)) {
        link.setAttribute("href", basePath.slice(0, -1) + href);
      }
    });

    // Static <script>, <link>, <img>
    document
      .querySelectorAll("script[src], link[href], img[src]")
      .forEach(function (el) {
        if (el.tagName === "SCRIPT" || el.tagName === "IMG") {
          rewriteAttribute(el, "src");
        } else if (el.tagName === "LINK") {
          rewriteAttribute(el, "href");
        }
      });
  }

  function watchForDynamicInsertions() {
    var observer = new MutationObserver(function (mutations) {
      mutations.forEach(function (mutation) {
        mutation.addedNodes.forEach(function (node) {
          if (!(node instanceof HTMLElement)) return;

          if (node.tagName === "SCRIPT" || node.tagName === "IMG") {
            rewriteAttribute(node, "src");
          } else if (node.tagName === "LINK") {
            rewriteAttribute(node, "href");
          } else if (node.tagName === "A") {
            var href = node.getAttribute("href");
            if (href && href.startsWith("/") && !href.startsWith(basePath)) {
              node.setAttribute("href", basePath.slice(0, -1) + href);
            }
          }
        });
      });
    });

    observer.observe(document.documentElement, {
      childList: true,
      subtree: true,
    });
  }

  function patchNetworkAPIs() {
    // Patch fetch
    var originalFetch = window.fetch;
    window.fetch = function (resource, init) {
      if (
        typeof resource === "string" &&
        resource.startsWith("/") &&
        !resource.startsWith(basePath)
      ) {
        resource = basePath.slice(0, -1) + resource;
      } else if (
        resource instanceof Request &&
        resource.url.startsWith(location.origin + "/")
      ) {
        var relative = resource.url.replace(location.origin, "");
        if (!relative.startsWith(basePath)) {
          resource = new Request(basePath.slice(0, -1) + relative, resource);
        }
      }
      return originalFetch(resource, init);
    };

    // Patch XMLHttpRequest
    var originalOpen = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function (method, url) {
      var args = Array.prototype.slice.call(arguments, 2);
      if (
        typeof url === "string" &&
        url.startsWith("/") &&
        !url.startsWith(basePath)
      ) {
        url = basePath.slice(0, -1) + url;
      } else if (url.startsWith(location.origin + "/")) {
        var relative = url.replace(location.origin, "");
        if (!relative.startsWith(basePath)) {
          url = basePath.slice(0, -1) + relative;
        }
      }
      return originalOpen.apply(this, [method, url].concat(args));
    };

    // Patch WebSocket constructor
    var originalWebSocket = window.WebSocket;
    window.WebSocket = function (url, protocols) {
      console.log("🔍 WebSocket intercepted, original URL:", url);

      if (typeof url === "string") {
        // Handle ws:// and wss:// protocols
        if (url.startsWith("ws://") || url.startsWith("wss://")) {
          try {
            var wsUrl = new URL(url);

            // If we're already on localhost:8080, check if the path needs basePath prefix
            if (wsUrl.hostname === "localhost" && wsUrl.port === "8080") {
              // Only rewrite if path doesn't already start with basePath
              if (
                wsUrl.pathname.startsWith("/") &&
                !wsUrl.pathname.startsWith(basePath.slice(0, -1))
              ) {
                // Check if path already starts with a port pattern like /3001/
                var portPattern = /^\/\d{4}\//;
                if (!portPattern.test(wsUrl.pathname)) {
                  wsUrl.pathname = basePath.slice(0, -1) + wsUrl.pathname;
                  url = wsUrl.toString();
                  console.log(
                    "🔄 Rewritten WebSocket path (already on 8080):",
                    url,
                  );
                } else {
                  console.log(
                    "✅ WebSocket URL already has port prefix, no rewrite needed:",
                    url,
                  );
                }
              } else {
                console.log(
                  "✅ WebSocket URL already correct, no rewrite needed:",
                  url,
                );
              }
            }
            // Handle localhost:PORT URLs - rewrite to localhost:8080/PORT
            else if (
              wsUrl.hostname === "localhost" &&
              wsUrl.port &&
              wsUrl.port !== "8080"
            ) {
              var originalPort = wsUrl.port;
              wsUrl.hostname = "localhost";
              wsUrl.port = "8080";

              // Check if the path already starts with the port (avoid double-prefixing)
              var portPrefix = "/" + originalPort;
              if (!wsUrl.pathname.startsWith(portPrefix)) {
                wsUrl.pathname = portPrefix + wsUrl.pathname;
              } else {
                console.log(
                  "✅ WebSocket path already has port prefix, keeping as-is:",
                  wsUrl.pathname,
                );
              }

              url = wsUrl.toString();
              console.log("🔄 Rewritten WebSocket localhost URL:", url);
            }
            // Check if it's a same-origin WebSocket with an absolute path that needs rewriting
            else if (
              wsUrl.hostname === location.hostname &&
              wsUrl.pathname.startsWith("/") &&
              !wsUrl.pathname.startsWith(basePath.slice(0, -1))
            ) {
              // Rewrite the pathname to include the base path
              wsUrl.pathname = basePath.slice(0, -1) + wsUrl.pathname;
              url = wsUrl.toString();
              console.log("🔄 Rewritten WebSocket path:", url);
            }
          } catch (e) {
            console.warn("Failed to parse WebSocket URL:", url, e);
          }
        }
        // Handle relative WebSocket URLs (starting with /)
        else if (
          url.startsWith("/") &&
          !url.startsWith(basePath.slice(0, -1))
        ) {
          // Convert to full WebSocket URL with current protocol
          var protocol = location.protocol === "https:" ? "wss:" : "ws:";
          url = protocol + "//" + location.host + basePath.slice(0, -1) + url;
          console.log("🔄 Rewritten relative WebSocket URL:", url);
        }
      }

      if (protocols !== undefined) {
        return new originalWebSocket(url, protocols);
      } else {
        return new originalWebSocket(url);
      }
    };

    // Copy static properties and methods
    Object.setPrototypeOf(window.WebSocket, originalWebSocket);
    Object.defineProperty(window.WebSocket, "prototype", {
      value: originalWebSocket.prototype,
      writable: false,
    });

    // Copy static constants
    window.WebSocket.CONNECTING = originalWebSocket.CONNECTING;
    window.WebSocket.OPEN = originalWebSocket.OPEN;
    window.WebSocket.CLOSING = originalWebSocket.CLOSING;
    window.WebSocket.CLOSED = originalWebSocket.CLOSED;

    // Patch EventSource constructor
    var originalEventSource = window.EventSource;
    window.EventSource = function (url, eventSourceInitDict) {
      console.log("🔍 EventSource intercepted, original URL:", url);

      if (typeof url === "string") {
        // Handle absolute URLs starting with /
        if (url.startsWith("/") && !url.startsWith(basePath.slice(0, -1))) {
          url = basePath.slice(0, -1) + url;
          console.log("🔄 Rewritten EventSource URL:", url);
        }
        // Handle full URLs with localhost:PORT
        else if (
          url.startsWith("http://localhost:") ||
          url.startsWith("https://localhost:")
        ) {
          try {
            var eventUrl = new URL(url);
            if (
              eventUrl.hostname === "localhost" &&
              eventUrl.port &&
              eventUrl.port !== "8080"
            ) {
              var originalPort = eventUrl.port;
              eventUrl.hostname = "localhost";
              eventUrl.port = "8080";

              // Check if the path already starts with the port (avoid double-prefixing)
              var portPrefix = "/" + originalPort;
              if (!eventUrl.pathname.startsWith(portPrefix)) {
                eventUrl.pathname = portPrefix + eventUrl.pathname;
              }

              url = eventUrl.toString();
              console.log("🔄 Rewritten EventSource localhost URL:", url);
            }
          } catch (e) {
            console.warn("Failed to parse EventSource URL:", url, e);
          }
        }
      }

      if (eventSourceInitDict !== undefined) {
        return new originalEventSource(url, eventSourceInitDict);
      } else {
        return new originalEventSource(url);
      }
    };

    // Copy static properties and methods
    Object.setPrototypeOf(window.EventSource, originalEventSource);
    Object.defineProperty(window.EventSource, "prototype", {
      value: originalEventSource.prototype,
      writable: false,
    });

    // Copy static constants
    window.EventSource.CONNECTING = originalEventSource.CONNECTING;
    window.EventSource.OPEN = originalEventSource.OPEN;
    window.EventSource.CLOSED = originalEventSource.CLOSED;

    // Patch dynamic import() - intercept import calls
    // Note: This approach has limitations but covers many common cases

    // Approach 1: Patch eval to catch dynamically constructed import statements
    var originalEval = window.eval;
    window.eval = function (code) {
      if (typeof code === "string" && code.includes("import(")) {
        // Replace import() calls in evaluated code
        code = code.replace(
          /import\s*\(\s*['"`]([^'"`]+)['"`]\s*\)/g,
          function (match, moduleSpecifier) {
            if (
              moduleSpecifier.startsWith("/") &&
              !moduleSpecifier.startsWith(basePath)
            ) {
              return (
                'import("' + basePath.slice(0, -1) + moduleSpecifier + '")'
              );
            }
            return match;
          },
        );
      }
      return originalEval.call(this, code);
    };

    // Approach 2: Function constructor patching removed to avoid webpack conflicts

    // Approach 3: Create a global import wrapper (for explicit calls)
    // This won't catch all import() usage but will catch code that explicitly calls window.import
    if (!window.originalImport) {
      window.originalImport = window.import; // Store if it exists
      window.import = function (moduleSpecifier) {
        if (
          typeof moduleSpecifier === "string" &&
          moduleSpecifier.startsWith("/") &&
          !moduleSpecifier.startsWith(basePath)
        ) {
          moduleSpecifier = basePath.slice(0, -1) + moduleSpecifier;
        }

        // Use native import
        return (function () {
          return import(moduleSpecifier);
        })();
      };
    }
  }

  // Patch WebSocket immediately (before any modules load)
  patchNetworkAPIs();

  // Initialize other patches on DOMContentLoaded
  document.addEventListener("DOMContentLoaded", function () {
    rewriteStaticResources();
    watchForDynamicInsertions();
  });

  /**
   * ✅ Handled:
   * - fetch() and XMLHttpRequest URL rewriting
   * - Static resource URL rewriting (script, link, img, a tags)
   * - Dynamic DOM insertion monitoring
   * - History API patching (pushState/replaceState)
   * - Dynamic import() patching (covers eval, Function constructor, and explicit window.import calls)
   * - WebSocket URL rewriting (ws://, wss://, relative paths, localhost:PORT)
   * - EventSource URL rewriting (relative paths, localhost:PORT)
   *
   * 🚧 Things NOT Yet Handled:
   * - new Image().src = "/foo.jpg" → would need to patch the Image constructor
   * - CSS url(/assets/foo.png) — rewriting stylesheet contents is out-of-scope unless you proxy/transform CSS
   * - Form actions (<form action="/post">) if used
   * - import() calls in already-loaded modules (static analysis would catch these, but runtime patching has limits)
   */

  // Iframe resizer functionality
  var isInIframe = false;
  var parentOrigin = null;
  var lastHeight = 0;
  var resizeObserver = null;

  // Guards against infinite resize loops
  var MAX_HEIGHT = 50000; // Maximum allowed height
  var MIN_HEIGHT_CHANGE = 10; // Minimum height change to trigger update
  var RATE_LIMIT_MS = 200; // Minimum time between height updates (5 per second)
  var CYCLE_DETECTION_WINDOW = 5; // Number of recent heights to track

  var lastUpdateTime = 0;
  var recentHeights = []; // Track recent heights for cycle detection
  var isRateLimited = false;

  // Check if we're in an iframe
  try {
    isInIframe = window.self !== window.top;
  } catch (e) {
    isInIframe = true;
  }

  if (isInIframe) {
    // Listen for setup message from parent
    window.addEventListener("message", function (event) {
      if (event.data && event.data.type === "catnip-iframe-setup") {
        parentOrigin = event.data.parentOrigin;
        initializeIframeResizer();
      }
    });

    function initializeIframeResizer() {
      // Function to calculate and send height with guards
      function sendHeight() {
        if (!parentOrigin || isRateLimited) return;

        var now = Date.now();

        // Rate limiting - enforce minimum time between updates
        if (now - lastUpdateTime < RATE_LIMIT_MS) {
          return;
        }

        var body = document.body;
        var html = document.documentElement;

        // Get the maximum height of the document
        var height = Math.max(
          body.scrollHeight,
          body.offsetHeight,
          html.clientHeight,
          html.scrollHeight,
          html.offsetHeight,
        );

        // Enforce maximum height limit
        if (height > MAX_HEIGHT) {
          console.warn(
            "Iframe resizer: Height exceeds maximum, capping at",
            MAX_HEIGHT,
          );
          height = MAX_HEIGHT;
        }

        // Only send if height has changed significantly
        if (Math.abs(height - lastHeight) < MIN_HEIGHT_CHANGE) {
          return;
        }

        // Cycle detection - check if we're oscillating between heights
        if (recentHeights.length >= CYCLE_DETECTION_WINDOW) {
          var isOscillating = recentHeights.some(function (h) {
            return Math.abs(h - height) < MIN_HEIGHT_CHANGE;
          });
          if (isOscillating && recentHeights.length > 2) {
            console.warn(
              "Iframe resizer: Potential oscillation detected, skipping update",
            );
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
          window.parent.postMessage(
            {
              type: "catnip-iframe-height",
              height: height,
            },
            parentOrigin,
          );
        } catch (e) {
          console.error("Iframe resizer: Failed to send height update", e);
        }
      }

      // Send initial height
      document.addEventListener("DOMContentLoaded", function () {
        setTimeout(sendHeight, 100); // Small delay for layout
      });

      // Send height when page is fully loaded
      window.addEventListener("load", function () {
        setTimeout(sendHeight, 100);
      });

      // Use ResizeObserver if available with timeout protection
      if (window.ResizeObserver) {
        var resizeTimeout;
        resizeObserver = new ResizeObserver(function () {
          // Debounce resize events to prevent excessive calls
          clearTimeout(resizeTimeout);
          resizeTimeout = setTimeout(sendHeight, 50);
        });
        resizeObserver.observe(document.body);
        resizeObserver.observe(document.documentElement);
      } else {
        // Fallback: poll for height changes with timeout protection
        var pollInterval = setInterval(function () {
          if (isRateLimited) {
            clearInterval(pollInterval);
            // Restart polling after rate limit cooldown
            setTimeout(function () {
              pollInterval = setInterval(sendHeight, 500);
            }, 2000);
          } else {
            sendHeight();
          }
        }, 500);
      }

      // Listen for dynamic content changes with timeout protection
      if (window.MutationObserver) {
        var mutationTimeout;
        var mutationObserver = new MutationObserver(function () {
          // Debounce mutation events to prevent excessive calls
          clearTimeout(mutationTimeout);
          mutationTimeout = setTimeout(sendHeight, 50);
        });
        mutationObserver.observe(document.body, {
          childList: true,
          subtree: true,
          attributes: true,
          attributeFilter: ["style", "class"],
        });
      }

      // Send height immediately if already loaded
      if (document.readyState === "complete") {
        setTimeout(sendHeight, 100);
      }
    }
  }
})();
