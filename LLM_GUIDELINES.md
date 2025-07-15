# LLM Guidelines Index

**INSTRUCTION**: Always read this index first, then inject relevant sections before executing tasks.

## Index

- `STATUS_TRACKING` - Critical workflow for all tasks
- `FRONTEND_GUIDELINES` - React/ShadCN/TailwindCSS standards
- `BACKEND_GUIDELINES` - Go/Docker/API development
- `DEVELOPMENT_WORKFLOW` - Container commands and debugging
- `GIT_WORKFLOW` - Branch management and commits
- `CODE_STYLE` - Formatting and best practices
- `TESTING_GUIDELINES` - Test execution and requirements
- `DEPLOYMENT_GUIDELINES` - Build and deployment processes
- `SECURITY_GUIDELINES` - Security best practices
- `DOCUMENTATION_GUIDELINES` - Code documentation standards

---

## STATUS_TRACKING

**CRITICAL**: This MUST be done for every step in all chat sessions:

**Before starting any task:**
1. Generate a "status.txt" file in `/workspace/<branch>` with present tense action, like "writing tests" or "updating frontend"

**After completing any task:**
1. Update "status.txt" file with past tense completion, like "wrote tests" or "updated frontend"  
2. Git add all changes (not the status file) and commit: `git add . && git commit -m "<status>"`

**Always inject this section before any task that modifies files or runs commands.**

---

## FRONTEND_GUIDELINES

**Framework**: React/Vite SPA with SWC, TailwindCSS, and ShadCN

**Dependencies**: Use `pnpm` for dependency management

**Components**: 
- Prefer [ShadCN components](https://ui.shadcn.com/docs/components)
- Add new components: `pnpm dlx shadcn@latest add button`
- Use TailwindCSS with dark/light mode support
- ShadCN theme variables preferred (extend in `index.css` if needed)

**Inject before**: Frontend development, UI component creation, styling tasks

---

## BACKEND_GUIDELINES

**Language**: Go with Docker containerization

**Documentation**: 
- OpenAPI/Swagger docs (auto-generated)
- JSONRPC endpoints where possible
- Build with `just build` in container directory
- Dev server auto-rebuilds on changes

**Container**: 
- Dev server runs on `catnip-dev` container
- Execute commands: `bash --login -c '...'`
- Check logs: `docker logs --tail 50 catnip-dev`
- Don't restart container unless explicitly asked

**Inject before**: Backend development, API creation, container operations

---

## DEVELOPMENT_WORKFLOW

**Container Operations**:
- Dev server runs on `catnip-dev` container
- Execute commands: `bash --login -c '...'`
- Check logs: `docker logs --tail 50 catnip-dev`
- Don't restart container unless explicitly asked

**Build Process**:
- Run `just build` in container directory for Go changes
- Wait for recompilation before hitting http://localhost:8080
- Use `just swagger` to regenerate API docs

**Troubleshooting**:
- If getting "no such file or directory", run `pwd` and navigate to root catnip directory

**Inject before**: Development tasks, debugging, container operations

---

## GIT_WORKFLOW

**Commit Process**:
1. Always follow STATUS_TRACKING guidelines
2. Use descriptive commit messages matching project style
3. Don't commit unless explicitly asked

**Inject before**: Any git operations, commits, branch management

---

## CODE_STYLE

**General**:
- Follow existing code conventions in the file
- Mimic code style, use existing libraries and utilities
- Follow existing patterns
- Never add comments unless requested
- Never assume libraries are available - check package.json/imports first

**Security**:
- Never expose or log secrets and keys
- Never commit secrets or keys to repository
- Follow security best practices

**Inject before**: Code writing, refactoring, new feature development

---

## TESTING_GUIDELINES

**Framework Detection**:
- NEVER assume specific test framework or test script
- Check README or search codebase to determine testing approach
- Look for test commands in package.json or justfile

**Verification**:
- Run tests after implementation if available
- Run lint and typecheck commands (npm run lint, npm run typecheck, ruff, etc.)
- Ask user for commands if not found, suggest adding to CLAUDE.md

**Inject before**: Testing, verification, quality assurance tasks

---

## DEPLOYMENT_GUIDELINES

**Environments**:
- Local development via dev server
- Production via Cloudflare Workers and Containers
- Toggle with CLOUDFLARE_DEV=true or "dev:cf" script

**Port System**:
- Auto-detection via `/proc/net/tcp` monitoring
- Preview at `/preview/$PORT` with iframe auto-sizing
- Direct access at `/$PORT/` with SPA routing support

**Inject before**: Deployment tasks, environment configuration

---

## SECURITY_GUIDELINES

**Defensive Security Only**:
- Assist with defensive security tasks only
- Refuse to create, modify, or improve code for malicious use
- Allow security analysis, detection rules, vulnerability explanations
- Support defensive tools and security documentation

**Code Security**:
- Never expose or log secrets and keys
- Never commit secrets or keys to repository
- Follow security best practices in all code

**Inject before**: Security-related tasks, code review, vulnerability assessment

---

## DOCUMENTATION_GUIDELINES

**API Documentation**:
- Use OpenAPI/Swagger for API documentation
- Auto-generate docs in dev and prod environments
- Run `just swagger` to regenerate docs

**Code Documentation**:
- Don't add comments unless explicitly requested
- Follow existing documentation patterns
- Reference specific functions with `file_path:line_number` format

**Project Documentation**:
- See `docs/` for GIT.md, LOCAL_REPOSITORIES.md, SETTINGS_SYNC.md, TODOS.md
- Don't create documentation files unless explicitly requested

**Inject before**: Documentation tasks, API documentation, code commenting