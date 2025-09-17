# AGENTS.md

Canonical playbook for all coding agents working in the Catnip repo. Model-specific quirks live alongside this file (for example `CLAUDE.md`). Always start here, then consult your agent supplement if one exists.

## Project Summary

Catnip is an agent-friendly development environment composed of:

- **Frontend**: React + TypeScript SPA built with Vite, ShadCN UI, Tailwind CSS, and TanStack Router
- **Container**: Unified Go binary (`catnip`) providing CLI tools and JSONRPC-powered APIs
- **Worker**: Cloudflare Worker using Hono for production edge logic

## Repository Layout

```
catnip/
├── src/                 React app (components, routes, hooks, lib, stores, types)
├── worker/              Cloudflare Worker entry `worker/index.ts` and scripts
├── container/           Go application, `just` tasks, and setup files
├── docs/                Additional documentation
├── public/              Static assets served by Vite
├── dist/                Build output (generated)
└── scripts/             Repository maintenance scripts
```

Path aliases are configured so the frontend imports from `@/*` (e.g. `import { cn } from "@/lib/utils"`).

## Core Commands

- `pnpm dev`: Vite dev server at `http://localhost:5173`
- `pnpm dev:cf:vite`: SPA dev server with Cloudflare integration flag
- `pnpm dev:cf`: Build SPA then run `wrangler dev` (worker + assets); logs stream to `/tmp/wrangler.log`
- `pnpm build`: Type-check then Vite build to `dist/`
- `pnpm preview`: Serve the built bundle locally
- `pnpm typecheck` / `pnpm typecheck:worker`: Strict TypeScript validation for app/worker
- `pnpm lint`: Run ESLint over the repo
- `pnpm format:changed`: Prettier on staged or changed files
- `just build-dev` then `just run-dev`: Containerized Go + frontend development flow

## Coding & Styling Guidelines

- TypeScript is strict; add explicit types when it improves clarity
- Component files are PascalCase `.tsx`; hooks live in `src/hooks` with `use-*.ts` names
- Prefer `@/*` imports over deep relative chains
- ESLint allows unused vars only when prefixed with `_`
- Use Prettier formatting before commits (`pnpm format:changed`)
- Reuse ShadCN theme tokens; avoid custom styling unless required

## Testing Expectations

- No unit test runner is configured yet; rely on `pnpm typecheck` and `pnpm lint`
- Add colocated tests as `*.test.ts(x)` if needed, kept fast and isolated
- Validate end-to-end flows via `pnpm dev` or `pnpm dev:cf`

## Development Environment Notes

- Dev servers auto-rebuild frontend and Go code; assume they are running
- Avoid restarting containers unless explicitly asked—the dev server manages restarts
- For Go changes, run `just build` inside `container/` to ensure compilation
- Tooltip components must use `TooltipPrimitive.Provider`, `.Root`, etc. Never mount `TooltipPrimitive` directly

## Security & Configuration

- Never commit secrets; use Wrangler secrets (e.g. `wrangler secret put GITHUB_APP_PRIVATE_KEY`)
- Local overrides belong in `.env.local`; deployment env vars are defined in `wrangler.jsonc`
- Workspace initialization runs `./setup.sh`; add dependency installs there when needed

## Working With Agents

- Treat `AGENTS.md` as the source of truth for shared workflows
- Keep per-agent documents lean—focus on deviations or ergonomics specific to that model and link back here for the baseline instructions
