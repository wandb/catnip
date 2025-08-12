#!/usr/bin/env node

import express from "express";
import cors from "cors";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const app = express();
const PORT = process.env.MOCK_PORT || 3001;

// Middleware
app.use(cors());
app.use(express.json());

// Load swagger spec
const swaggerSpec = JSON.parse(
  fs.readFileSync(path.join(__dirname, "container/docs/swagger.json"), "utf8"),
);

// Mock data generators
const mockData = {
  // Auth endpoints
  authStartResponse: {
    device_code: "mock-device-code-123",
    user_code: "ABCD-1234",
    verification_uri: "https://github.com/login/device",
    verification_uri_complete:
      "https://github.com/login/device?user_code=ABCD-1234",
    expires_in: 900,
    interval: 5,
  },

  authStatusResponse: {
    status: "pending",
    authenticated: false,
    error: "",
  },

  // Claude endpoints
  claudeSettings: {
    theme: "dark",
    isAuthenticated: true,
    hasCompletedOnboarding: true,
    numStartups: 42,
    version: "1.2.3",
  },

  claudeSessionSummary: {
    worktreePath: "/workspace/mock-project",
    currentSessionId: "session-123",
    lastSessionId: "session-122",
    isActive: true,
    header: "Mock development session",
    turnCount: 5,
    sessionStartTime: new Date().toISOString(),
    sessionEndTime: null,
    lastCost: 0.15,
    lastDuration: 1800,
    lastTotalInputTokens: 12000,
    lastTotalOutputTokens: 6000,
    allSessions: [
      {
        id: "session-123",
        name: "Current session",
        startTime: new Date().toISOString(),
      },
      {
        id: "session-122",
        name: "Previous session",
        startTime: new Date(Date.now() - 86400000).toISOString(),
      },
    ],
  },

  // Git endpoints
  gitWorktrees: [
    {
      id: "catnip-salem",
      name: "catnip/salem",
      path: "/workspace/catnip/salem",
      branch: "main",
      repo_id: "catnip-repo",
      is_main: true,
      is_bare: false,
      is_dirty: false,
      locked: false,
      prunable: false,
      head: "abc123def456",
      commit_count: 42,
      commits_behind: 0,
      has_conflicts: false,
      todos: [],
      cache_status: {
        is_cached: true,
        is_loading: false,
        last_updated: Date.now(),
      },
    },
    {
      id: "example-project",
      name: "example/project",
      path: "/workspace/example/project",
      branch: "feature/new-feature",
      repo_id: "example-repo",
      is_main: false,
      is_bare: false,
      is_dirty: true,
      locked: false,
      prunable: false,
      head: "def456ghi789",
      commit_count: 10,
      commits_behind: 2,
      has_conflicts: false,
      dirty_files: [
        { path: "src/index.js", status: "M" },
        { path: "package.json", status: "M" },
      ],
      todos: [
        { id: "1", content: "Implement new feature", status: "in_progress" },
        { id: "2", content: "Write tests", status: "pending" },
      ],
      cache_status: {
        is_cached: true,
        is_loading: false,
        last_updated: Date.now(),
      },
    },
  ],

  gitStatus: {
    repositories: {
      "catnip-repo": {
        id: "catnip-repo",
        name: "catnip",
        path: "/workspace/catnip",
        available: true,
        remote_url: "https://github.com/user/catnip.git",
        default_branch: "main",
      },
      "example-repo": {
        id: "example-repo",
        name: "example",
        path: "/workspace/example",
        available: true,
        remote_url: "https://github.com/user/example.git",
        default_branch: "main",
      },
    },
    worktrees: {},
  },

  gitBranches: [
    { name: "main", is_current: true, remote: true },
    { name: "develop", is_current: false, remote: true },
    { name: "feature/auth", is_current: false, remote: true },
    { name: "feature/mock-server", is_current: false, remote: false },
  ],

  // Ports
  ports: {
    3000: {
      port: 3000,
      service: "React Dev Server",
      protocol: "http",
      status: "open",
      url: "http://localhost:3000",
    },
    3001: {
      port: 3001,
      service: "Mock API Server",
      protocol: "http",
      status: "open",
      url: "http://localhost:3001",
    },
    5173: {
      port: 5173,
      service: "Vite Dev Server",
      protocol: "http",
      status: "open",
      url: "http://localhost:5173",
    },
  },

  // Sessions
  sessions: [
    {
      id: "pty-1",
      workspace: "/workspace/main",
      cols: 80,
      rows: 24,
      active: true,
      created_at: new Date().toISOString(),
    },
  ],

  // Notifications
  notifications: [
    {
      id: "notif-1",
      type: "info",
      message: "Mock server is running",
      timestamp: new Date().toISOString(),
    },
  ],
};

// Auth endpoints
app.post("/v1/auth/github/start", (req, res) => {
  res.json(mockData.authStartResponse);
});

app.get("/v1/auth/github/status", (req, res) => {
  res.json(mockData.authStatusResponse);
});

app.post("/v1/auth/github/reset", (req, res) => {
  res.json({ message: "Authentication reset" });
});

// Claude endpoints
app.get("/v1/claude/settings", (req, res) => {
  res.json(mockData.claudeSettings);
});

app.put("/v1/claude/settings", (req, res) => {
  const updatedSettings = { ...mockData.claudeSettings, ...req.body };
  mockData.claudeSettings = updatedSettings;
  res.json(updatedSettings);
});

app.get("/v1/claude/session", (req, res) => {
  res.json(mockData.claudeSessionSummary);
});

app.get("/v1/claude/session/:uuid", (req, res) => {
  res.json({
    uuid: req.params.uuid,
    worktreePath: "/workspace/mock-project",
    messages: [
      {
        uuid: "msg-1",
        type: "user",
        message: { content: "Hello, Claude!" },
        timestamp: new Date().toISOString(),
      },
      {
        uuid: "msg-2",
        type: "assistant",
        message: { content: "Hello! How can I help you today?" },
        timestamp: new Date().toISOString(),
      },
    ],
  });
});

app.get("/v1/claude/sessions", (req, res) => {
  res.json({
    "/workspace/main": mockData.claudeSessionSummary,
    "/workspace/feature-branch": {
      ...mockData.claudeSessionSummary,
      worktreePath: "/workspace/feature-branch",
    },
  });
});

app.post("/v1/claude/messages", (req, res) => {
  const { stream = false } = req.body;

  if (stream) {
    res.setHeader("Content-Type", "text/event-stream");
    res.setHeader("Cache-Control", "no-cache");
    res.setHeader("Connection", "keep-alive");

    const response = "This is a mock streaming response from Claude.";
    const words = response.split(" ");

    words.forEach((word, index) => {
      setTimeout(() => {
        res.write(
          `data: ${JSON.stringify({
            is_chunk: true,
            text: word + " ",
            is_last: index === words.length - 1,
          })}\n\n`,
        );

        if (index === words.length - 1) {
          res.end();
        }
      }, index * 100);
    });
  } else {
    res.json({
      text: "This is a mock response from Claude.",
      is_chunk: false,
      is_last: true,
    });
  }
});

app.post("/v1/claude/hooks", (req, res) => {
  console.log("Received Claude hook:", req.body);
  res.json({ status: "ok" });
});

app.get("/v1/claude/todos", (req, res) => {
  res.json({
    todos: [
      { id: "1", content: "Implement feature X", status: "pending" },
      { id: "2", content: "Fix bug Y", status: "in_progress" },
      { id: "3", content: "Write tests", status: "completed" },
    ],
  });
});

// Git endpoints
app.get("/v1/git/worktrees", (req, res) => {
  res.json(mockData.gitWorktrees);
});

app.get("/v1/git/worktrees/:id", (req, res) => {
  const worktree = mockData.gitWorktrees.find((w) => w.id === req.params.id);
  if (worktree) {
    res.json(worktree);
  } else {
    res.status(404).json({ error: "Worktree not found" });
  }
});

app.delete("/v1/git/worktrees/:id", (req, res) => {
  res.json({ message: `Worktree ${req.params.id} deleted` });
});

app.get("/v1/git/status", (req, res) => {
  res.json(mockData.gitStatus);
});

app.get("/v1/git/branches/:repo_id", (req, res) => {
  res.json(mockData.gitBranches);
});

app.post("/v1/git/checkout/:org/:repo", (req, res) => {
  res.json({
    success: true,
    worktree: {
      id: `${req.params.org}-${req.params.repo}`,
      path: `/workspace/${req.params.org}/${req.params.repo}`,
      branch: req.body.branch || "main",
    },
  });
});

app.get("/v1/git/github/repos", (req, res) => {
  res.json([
    {
      id: 1,
      name: "mock-repo",
      full_name: "user/mock-repo",
      private: false,
      description: "A mock repository",
      clone_url: "https://github.com/user/mock-repo.git",
    },
  ]);
});

// Port management
app.get("/v1/ports", (req, res) => {
  res.json(Object.values(mockData.ports));
});

app.get("/v1/ports/mappings", (req, res) => {
  res.json(mockData.ports);
});

app.get("/v1/ports/mappings/:port", (req, res) => {
  const port = mockData.ports[req.params.port];
  if (port) {
    res.json(port);
  } else {
    res.status(404).json({ error: "Port not found" });
  }
});

app.post("/v1/ports/:port", (req, res) => {
  const port = parseInt(req.params.port);
  mockData.ports[port] = {
    port,
    service: req.body.service || "Unknown Service",
    protocol: req.body.protocol || "http",
    status: "open",
    url: `http://localhost:${port}`,
  };
  res.json(mockData.ports[port]);
});

// Sessions
app.get("/v1/sessions", (req, res) => {
  res.json(mockData.sessions);
});

app.get("/v1/sessions/active", (req, res) => {
  res.json(mockData.sessions.filter((s) => s.active));
});

app.get("/v1/sessions/workspace/:workspace", (req, res) => {
  res.json(
    mockData.sessions.filter(
      (s) => s.workspace === decodeURIComponent(req.params.workspace),
    ),
  );
});

// Notifications
app.get("/v1/notifications", (req, res) => {
  res.json(mockData.notifications);
});

app.post("/v1/notifications", (req, res) => {
  const notification = {
    id: `notif-${Date.now()}`,
    ...req.body,
    timestamp: new Date().toISOString(),
  };
  mockData.notifications.push(notification);
  res.json(notification);
});

// File upload
app.post("/v1/upload", (req, res) => {
  res.json({
    success: true,
    file_path: "/workspace/uploads/mock-file.txt",
    size: 1024,
  });
});

// PTY endpoint (mock WebSocket upgrade)
app.get("/v1/pty", (req, res) => {
  res.json({
    message: "WebSocket endpoint - use ws:// protocol",
    url: `ws://localhost:${PORT}/v1/pty`,
  });
});

// SSE Events endpoint
app.get("/v1/events", (req, res) => {
  res.setHeader("Content-Type", "text/event-stream");
  res.setHeader("Cache-Control", "no-cache");
  res.setHeader("Connection", "keep-alive");
  res.setHeader("Access-Control-Allow-Origin", "*");

  // Send initial state
  res.write(
    `data: ${JSON.stringify({
      event: {
        type: "container:status",
        payload: {
          status: "running",
          message: "Mock server connected",
          sshEnabled: false,
        },
      },
      timestamp: Date.now(),
      id: "init-1",
    })}\n\n`,
  );

  // Send worktree information - critical for app to redirect properly
  mockData.gitWorktrees.forEach((worktree, index) => {
    // Send worktree created event
    res.write(
      `data: ${JSON.stringify({
        event: {
          type: "worktree:created",
          payload: {
            worktree: worktree,
          },
        },
        timestamp: Date.now(),
        id: `worktree-created-${index}`,
      })}\n\n`,
    );

    // Send worktree status if dirty
    if (worktree.is_dirty) {
      res.write(
        `data: ${JSON.stringify({
          event: {
            type: "worktree:dirty",
            payload: {
              worktree_id: worktree.id,
              files: worktree.dirty_files || [],
            },
          },
          timestamp: Date.now(),
          id: `worktree-dirty-${index}`,
        })}\n\n`,
      );
    }

    // Send todos if any
    if (worktree.todos && worktree.todos.length > 0) {
      res.write(
        `data: ${JSON.stringify({
          event: {
            type: "worktree:todos_updated",
            payload: {
              worktree_id: worktree.id,
              todos: worktree.todos,
            },
          },
          timestamp: Date.now(),
          id: `worktree-todos-${index}`,
        })}\n\n`,
      );
    }
  });

  // Send current ports
  Object.values(mockData.ports).forEach((port) => {
    res.write(
      `data: ${JSON.stringify({
        event: {
          type: "port:opened",
          payload: port,
        },
        timestamp: Date.now(),
        id: `port-${port.port}`,
      })}\n\n`,
    );
  });

  // Heartbeat interval
  const heartbeatInterval = setInterval(() => {
    res.write(
      `data: ${JSON.stringify({
        event: {
          type: "heartbeat",
          payload: {
            timestamp: Date.now(),
            uptime: process.uptime() * 1000,
          },
        },
        timestamp: Date.now(),
        id: `heartbeat-${Date.now()}`,
      })}\n\n`,
    );
  }, 5000);

  // Simulate random events
  const eventInterval = setInterval(() => {
    const eventTypes = [
      {
        type: "process:started",
        payload: {
          pid: Math.floor(Math.random() * 10000),
          command: "npm run dev",
          workspace: "/workspace/main",
        },
      },
      {
        type: "git:dirty",
        payload: {
          workspace: "/workspace/main",
          files: ["src/App.tsx", "package.json"],
        },
      },
      {
        type: "port:opened",
        payload: {
          port: 3000 + Math.floor(Math.random() * 100),
          service: "Development Server",
          protocol: "http",
        },
      },
    ];

    const randomEvent =
      eventTypes[Math.floor(Math.random() * eventTypes.length)];

    res.write(
      `data: ${JSON.stringify({
        event: randomEvent,
        timestamp: Date.now(),
        id: `event-${Date.now()}`,
      })}\n\n`,
    );
  }, 15000); // Random event every 15 seconds

  // Clean up on client disconnect
  req.on("close", () => {
    clearInterval(heartbeatInterval);
    clearInterval(eventInterval);
    res.end();
  });
});

// Catch-all for undefined routes
app.use("/v1/*", (req, res) => {
  console.warn(`Unhandled route: ${req.method} ${req.path}`);
  res.status(404).json({
    error: "Not found",
    message: `Mock endpoint not implemented: ${req.path}`,
    method: req.method,
  });
});

// Error handling - Express 5 requires 4 parameters
app.use((err, req, res, next) => {
  console.error("Error:", err);
  res.status(500).json({ error: err.message || "Internal server error" });
});

// Start server
app.listen(PORT, () => {
  console.log(`🚀 Mock server running on http://localhost:${PORT}`);
  console.log(`📡 SSE events available at http://localhost:${PORT}/v1/events`);
  console.log("\nAvailable endpoints:");
  console.log("  - Auth: /v1/auth/github/*");
  console.log("  - Claude: /v1/claude/*");
  console.log("  - Git: /v1/git/*");
  console.log("  - Ports: /v1/ports/*");
  console.log("  - Sessions: /v1/sessions/*");
  console.log("  - Events (SSE): /v1/events");
  console.log("\nUse MOCK_PORT env var to change port (default: 3001)");
});
