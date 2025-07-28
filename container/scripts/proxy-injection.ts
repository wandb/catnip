/**
 * Proxy injection script for Catnip
 * This script is injected into HTML pages served through the proxy to handle
 * SPA routing and iframe resizing when pages are served under sub-paths like "/3000"
 */

declare global {
  interface Window {
    __PROXY_BASE_PATH__: string;
    getProxyBasePath(): string;
  }
}

(function () {
  const basePath = window.__PROXY_BASE_PATH__;

  // Helper function to get base path
  window.getProxyBasePath = function () {
    return basePath;
  };

  // Override pushState and replaceState
  const originalPushState = history.pushState;
  const originalReplaceState = history.replaceState;

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

  function rewriteAttribute(el: HTMLElement, attr: string) {
    const val = (el as any)[attr];
    if (!val || typeof val !== "string") return;

    const originPrefix = location.origin + "/";
    if (val.startsWith(originPrefix)) {
      const relative = val.replace(location.origin, "");
      if (!relative.startsWith(basePath)) {
        (el as any)[attr] = basePath.slice(0, -1) + relative;
      }
    }
  }

  function rewriteStaticResources() {
    // Anchor tags
    document.querySelectorAll('a[href^="/"]').forEach((link) => {
      const href = link.getAttribute("href");
      if (href && !href.startsWith(basePath)) {
        link.setAttribute("href", basePath.slice(0, -1) + href);
      }
    });

    // Static <script>, <link>, <img>
    document
      .querySelectorAll("script[src], link[href], img[src]")
      .forEach((el) => {
        if (el.tagName === "SCRIPT" || el.tagName === "IMG") {
          rewriteAttribute(el as HTMLElement, "src");
        } else if (el.tagName === "LINK") {
          rewriteAttribute(el as HTMLElement, "href");
        }
      });
  }

  function watchForDynamicInsertions() {
    const observer = new MutationObserver((mutations) => {
      mutations.forEach((mutation) => {
        mutation.addedNodes.forEach((node) => {
          if (!(node instanceof HTMLElement)) return;

          if (node.tagName === "SCRIPT" || node.tagName === "IMG") {
            rewriteAttribute(node, "src");
          } else if (node.tagName === "LINK") {
            rewriteAttribute(node, "href");
          } else if (node.tagName === "A") {
            const href = node.getAttribute("href");
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

  function patchFetchAndXHR() {
    // Patch fetch
    const originalFetch = window.fetch;
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
        const relative = resource.url.replace(location.origin, "");
        if (!relative.startsWith(basePath)) {
          resource = new Request(basePath.slice(0, -1) + relative, resource);
        }
      }
      return originalFetch(resource, init);
    };

    // Patch XMLHttpRequest
    const originalOpen = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function (method, url, ...args) {
      if (
        typeof url === "string" &&
        url.startsWith("/") &&
        !url.startsWith(basePath)
      ) {
        url = basePath.slice(0, -1) + url;
      } else if (url.startsWith(location.origin + "/")) {
        const relative = url.replace(location.origin, "");
        if (!relative.startsWith(basePath)) {
          url = basePath.slice(0, -1) + relative;
        }
      }
      return originalOpen.call(this, method, url, ...args);
    };
  }

  function patchWebSocket() {
    // Patch WebSocket constructor
    const originalWebSocket = window.WebSocket;
    window.WebSocket = function (url, protocols) {
      if (typeof url === "string") {
        // Handle ws:// and wss:// protocols
        if (url.startsWith("ws://") || url.startsWith("wss://")) {
          const wsUrl = new URL(url, location.href);
          // Check if it's a same-origin WebSocket with an absolute path
          if (
            wsUrl.hostname === location.hostname &&
            wsUrl.pathname.startsWith("/") &&
            !wsUrl.pathname.startsWith(basePath)
          ) {
            // Rewrite the pathname to include the base path
            wsUrl.pathname = basePath.slice(0, -1) + wsUrl.pathname;
            url = wsUrl.toString();
          }
        }
        // Handle relative WebSocket URLs (ws:///path or wss:///path)
        else if (url.startsWith("/") && !url.startsWith(basePath)) {
          // Convert to full WebSocket URL with current protocol
          const protocol = location.protocol === "https:" ? "wss:" : "ws:";
          url = `${protocol}//${location.host}${basePath.slice(0, -1)}${url}`;
        }
      }

      if (protocols !== undefined) {
        return new originalWebSocket(url, protocols);
      } else {
        return new originalWebSocket(url);
      }
    } as any;

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
  }

  // Initialize on DOMContentLoaded
  document.addEventListener("DOMContentLoaded", function () {
    rewriteStaticResources();
    watchForDynamicInsertions();
    patchFetchAndXHR();
    patchWebSocket();
  });

  /**
   * 🚧 Things NOT Yet Handled:
   *
   * - new Image().src = "/foo.jpg" → you'd need to patch the Image constructor
   * - new EventSource("/stream") → would need to wrap EventSource
   * - import("/module.js") dynamic imports cannot be intercepted easily at runtime
   * - CSS url(/assets/foo.png) — rewriting stylesheet contents is out-of-scope unless you proxy/transform CSS
   * - Form actions (<form action="/post">) if used
   */

  // Iframe resizer functionality
  let isInIframe = false;
  let parentOrigin: string | null = null;
  let lastHeight = 0;
  let resizeObserver: ResizeObserver | null = null;

  // Guards against infinite resize loops
  const MAX_HEIGHT = 50000; // Maximum allowed height
  const MIN_HEIGHT_CHANGE = 10; // Minimum height change to trigger update
  const RATE_LIMIT_MS = 200; // Minimum time between height updates (5 per second)
  const CYCLE_DETECTION_WINDOW = 5; // Number of recent heights to track

  let lastUpdateTime = 0;
  let recentHeights: number[] = []; // Track recent heights for cycle detection
  let isRateLimited = false;

  // Check if we're in an iframe
  try {
    isInIframe = window.self !== window.top;
  } catch (e) {
    isInIframe = true;
  }

  if (isInIframe) {
    // Listen for setup message from parent
    window.addEventListener("message", function (event) {
      if (event.data?.type === "catnip-iframe-setup") {
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
          const isOscillating = recentHeights.some(
            (h) => Math.abs(h - height) < MIN_HEIGHT_CHANGE,
          );
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
        let resizeTimeout: number;
        resizeObserver = new ResizeObserver(function () {
          // Debounce resize events to prevent excessive calls
          clearTimeout(resizeTimeout);
          resizeTimeout = setTimeout(sendHeight, 50) as any;
        });
        resizeObserver.observe(document.body);
        resizeObserver.observe(document.documentElement);
      } else {
        // Fallback: poll for height changes with timeout protection
        let pollInterval = setInterval(function () {
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
        let mutationTimeout: number;
        const mutationObserver = new MutationObserver(function () {
          // Debounce mutation events to prevent excessive calls
          clearTimeout(mutationTimeout);
          mutationTimeout = setTimeout(sendHeight, 50) as any;
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
