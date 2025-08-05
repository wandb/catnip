# CLAUDE.md

## Project Overview

Catnip is an agentic coding environment with three main components:

- **Frontend**: React/Vite SPA using ShadCN UI, Tailwind CSS, and TanStack Router
- **Container**: Unified Go binary (`catnip`) providing both CLI tools and server APIs for sandboxed development
- **Worker**: Hono-based Cloudflare Worker (production only)

## Tech Stack & Conventions

### Frontend

- Package manager: `pnpm`
- UI Components: [ShadCN](https://ui.shadcn.com/docs/components) - add with `pnpm dlx shadcn@latest add button`
- Styling: Tailwind CSS with dark/light mode support
- Router: TanStack Router

### Backend

- Language: Go
- Build tool: `just` (run `just build` in container/ directory)
- Documentation: OpenAPI/Swagger (auto-generated on build - ensure API endpoints and models are well commented)
- Architecture: JSONRPC where possible
- Terminal UI: Bubbletea-based CLI tool (`catnip`)

## Directory Structure

```
catnip/
├── src/                     # Frontend React/Vite application
│   ├── components/         # React components including ShadCN UI
│   ├── routes/            # TanStack Router pages
│   └── lib/               # Utilities and shared code
├── container/             # Go application running in Docker
│   ├── cmd/               # Unified binary entry point
│   ├── internal/          # Internal Go packages
│   └── setup/             # Container setup scripts
├── worker/                # Cloudflare Worker (Hono-based)
└── public/                # Static assets
```

## Development Guidelines

### General Rules

- Dev server auto-rebuilds Go and frontend - assume it's running
- Don't restart containers unless explicitly asked
- Use ShadCN theme variables whenever possible
- For Go changes, run `just build` in container/ directory to ensure compilation
- Don't restart the container, our dev server uses air to restart the main binary automatically and restarting the container causes state to be lost

### Common Issues & Best Practices

- **TooltipPrimitive**: Always use `TooltipPrimitive.Provider`, `TooltipPrimitive.Root`, etc. - never use `TooltipPrimitive` directly as a JSX component. This is a recurring issue when working with Radix UI tooltips.

### Container Commands

- Execute in dev container: `docker exec -it catnip-dev bash --login -c '...'`
- View logs: `docker logs --tail 50 catnip-dev`

### Port System

- Services auto-detected and appear in dashboard
- Preview: `/preview/$PORT` (full-screen iframe)
- Direct access: `/$PORT/` (proxy with SPA routing)

## Essential Files

- `docs/` - Additional documentation
