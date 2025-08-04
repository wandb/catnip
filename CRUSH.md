## Catnip Agentic Coding Environment

This document provides guidelines for AI agents working in the Catnip repository.

### Commands

**Go (in `/container`):**

- **Build:** `just build`
- **Test:** `just test` (unit) or `go test -v ./path/to/package_test.go`
- **Lint:** `just lint`
- **Format:** `gofmt -w -s .`

**TypeScript/Frontend (in root):**

- **Install:** `pnpm install`
- **Build:** `pnpm build`
- **Test:** (No dedicated test command found, use `pnpm typecheck`)
- **Lint:** `pnpm lint`
- **Format:** `pnpm format:changed`
- **Type Check:** `pnpm typecheck`

### Code Style

**Go:**

- **Formatting:** Use `gofmt`.
- **Linting:** Adhere to `.golangci.yml` rules.
- **Error Handling:** Check errors, except for `Close()` and `os.Remove()` in cleanup.
- **Dependencies:** Manage with Go modules.

**TypeScript/Frontend:**

- **Package Manager:** Use `pnpm`.
- **Formatting:** Use `prettier` with project's `.prettierrc.json` settings.
- **UI Components:** Use ShadCN. Add new components with `pnpm dlx shadcn-ui@latest add <component>`.
- **Styling:** Use Tailwind CSS with dark/light mode support.
- **Routing:** Use TanStack Router.

**General:**

- This is a monorepo with a Go container, a React/Vite frontend, and a Cloudflare worker.
- Make changes in small, logical commits.
