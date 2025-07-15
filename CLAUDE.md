# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Just-In-Time Guidelines System

**CRITICAL**: Before executing any task, read `LLM_GUIDELINES.md` index and inject relevant sections.

**Example workflow:**
1. User asks to implement a React component
2. Read `LLM_GUIDELINES.md` index
3. Inject `STATUS_TRACKING` + `FRONTEND_GUIDELINES` sections
4. Execute task
5. Follow STATUS_TRACKING requirements

## Project Overview

**Catnip** is an agentic coding environment that combines:
- **Frontend**: React/Vite SPA with SWC, TailwindCSS, and ShadCN
- **Worker**: Hono-based Cloudflare Worker (production only)
- **Container**: Go server in Docker providing PTY/shell access

## Directory Structure

```
catnip/
├── src/                     # Frontend (React/Vite)
├── container/              # Go server & Docker
├── worker/                 # Cloudflare Worker
├── docs/                   # Documentation
├── reference/              # Prototype examples
└── LLM_GUIDELINES.md       # Just-in-time guideline sections
```

## Features

**Implemented:**
- ✅ PTY shell access via xterm
- Port detection with iframe previews
- Reverse proxy (`/$PORT/*` → `localhost:$PORT`)
- Claude completion API

**Planned:**
- Credential persistence, Git worktrees, SSH server, MCP browser server

## Claude Completion API

**Endpoint:** `POST /v1/claude/completion`

**Setup:**
```bash
export ANTHROPIC_API_KEY="your-api-key-here"
```

**Request:**
```json
{
  "message": "Your message",
  "max_tokens": 1024,
  "model": "claude-3-5-sonnet-20241022",
  "system": "Optional system prompt",
  "context": [{"role": "user", "content": "Previous message"}]
}
```

# Required Status Tracking

**IMPORTANT**: SUPER IMPORTANT, FOR ALL CHAT SESSIONS, ALWAYS DO THIS, AFTER EVERY STEP:

1. Generate a "status.txt" file in `/workspace/<branch>` with only a few words summarizing the current status, like "writing tests" or "updating frontend"
2. Git add all changes (not the status file)and commit with the status as the commit message: `git add . && git commit -m "<status>"`

## Quick Reference

- **Swagger docs**: `just swagger` in container directory
- **Troubleshooting**: Run `pwd` if getting "no such file or directory"
- **Documentation**: See `docs/` for GIT.md, LOCAL_REPOSITORIES.md, SETTINGS_SYNC.md, TODOS.md
