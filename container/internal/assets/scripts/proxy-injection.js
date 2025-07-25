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

    // Approach 2: Patch Function constructor for dynamically created functions
    var originalFunction = window.Function;
    window.Function = function () {
      var args = Array.prototype.slice.call(arguments);
      var code = args[args.length - 1];

      if (typeof code === "string" && code.includes("import(")) {
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
        args[args.length - 1] = code;
      }

      return originalFunction.apply(this, args);
    };

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

  // Initialize on DOMContentLoaded
  document.addEventListener("DOMContentLoaded", function () {
    rewriteStaticResources();
    watchForDynamicInsertions();
    patchNetworkAPIs();
  });

  /**
   * ✅ Handled:
   * - fetch() and XMLHttpRequest URL rewriting
   * - Static resource URL rewriting (script, link, img, a tags)
   * - Dynamic DOM insertion monitoring
   * - History API patching (pushState/replaceState)
   * - Dynamic import() patching (covers eval, Function constructor, and explicit window.import calls)
   *
   * 🚧 Things NOT Yet Handled:
   * - new Image().src = "/foo.jpg" → would need to patch the Image constructor
   * - new EventSource("/stream") → would need to wrap EventSource
   * - CSS url(/assets/foo.png) — rewriting stylesheet contents is out-of-scope unless you proxy/transform CSS
   * - WebSocket URLs like ws://example.com/...
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
