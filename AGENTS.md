# Repository Guidelines

## Project Structure & Module Organization

- `src/`: React + TypeScript SPA.
  - `components/` (PascalCase `.tsx`), `routes/` (TanStack Router), `lib/` utilities, `hooks/` (files start with `use-`), `stores/`, `types/`.
- `worker/`: Cloudflare Worker (entry `worker/index.ts`) and scripts in `worker/scripts/`.
- `public/`: static assets; `dist/`: build output.
- `container/`: container build files; `docs/`: documentation; `scripts/`: repo maintenance.
- Path alias: import from `@/*` (configured in `tsconfig`). Example: `import { cn } from "@/lib/utils"`.

## Build, Test, and Development Commands

- `pnpm dev`: run Vite dev server at `http://localhost:5173`.
- `pnpm dev:cf:vite`: SPA with Cloudflare dev flag for local Worker integration.
- `pnpm dev:cf`: build SPA then run `wrangler dev` (Worker + assets); logs are tee’d to `/tmp/wrangler.log`.
- `pnpm build`: type-check then Vite build to `dist/`.
- `pnpm preview`: serve built assets locally.
- `pnpm typecheck` / `pnpm typecheck:worker`: strict TS checks for app/worker.
- `pnpm lint`: ESLint over repo.
- `pnpm format:changed`: Prettier on staged/changed files.
- Containers: `just build-dev` then `just run-dev` for full-stack containerized dev.

## Coding Style & Naming Conventions

- TypeScript strict enabled; prefer explicit types where helpful.
- ESLint rules in `eslint.config.js`; unused vars allowed with leading `_`.
- Components: PascalCase filenames and exports. Hooks: `use-*.ts` in `src/hooks`.
- Imports: prefer `@/*` alias; avoid deep relative chains.
- Formatting: Prettier is available; use `pnpm format:changed` before commits.

## Testing Guidelines

- No unit test runner is configured yet. Use `pnpm typecheck` and `pnpm lint` as gates.
- For new tests, prefer colocated files named `*.test.ts(x)` under `src/` and keep fast, isolated modules.
- Validate end-to-end flows via `pnpm dev:cf` (Worker) or `pnpm dev` (SPA).

## Commit & Pull Request Guidelines

- Commits: short imperative subject with scope prefix when helpful (e.g., `CLI: reannounce host port mappings`, `UI: fix sidebar hostPort`).
- PRs: include summary, linked issues, screenshots/recordings for UI, and notes on dev/testing steps. Update `docs/` when user-facing behavior changes.

## Security & Configuration Tips

- Secrets: never commit secrets. Use Wrangler secrets (e.g., `wrangler secret put GITHUB_APP_PRIVATE_KEY`).
- Env: local overrides in `.env.local`; deployment vars are set per-environment in `wrangler.jsonc`.
- On workspace creation, the container runs `./setup.sh`—install any dependencies there.
