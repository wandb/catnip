# 🐱 CatNip

> **The developer environment that's like catnip for agentic programming**

CatNip transforms the way you work with AI coding assistants by providing a **sandboxed, containerized environment** that's designed from the ground up for seamless human-AI collaboration. Think of it as your AI pair programmer's dream workspace.

## 🚀 Why CatNip?

Traditional development environments force AI assistants to work blind, making assumptions about your setup and struggling with context. CatNip flips this paradigm by providing:

- **🔒 Isolated Sandbox**: Every coding session runs in a pristine, containerized environment
- **💻 Full Terminal Access**: Real PTY sessions with bash, not simulated command execution
- **🌐 Universal Access**: Works locally via Docker or in the cloud with Cloudflare Containers
- **🔄 Live Collaboration**: Real-time WebSocket connections between your AI and the environment
- **📊 Rich Observability**: Built-in logging, metrics, and debugging tools for AI workflows

## ✨ Features

### 🎯 Current Features
- ✅ **Full PTY Access**: Complete terminal sessions via xterm.js
- ✅ **Git Integration**: Advanced git operations with worktree support
- ✅ **Multi-Project Support**: Work on multiple repositories simultaneously
- ✅ **API Documentation**: Comprehensive OpenAPI specs with interactive UI
- ✅ **Dark/Light Theme**: Beautiful UI with ShadCN components

### 🚧 Coming Soon
- 🔐 **Credential Persistence**: Seamless authentication for Claude Code and GitHub CLI
- 🌍 **HTTP Git Server**: Fetch changes across branches and worktrees
- 🔗 **Auto Port Forwarding**: Automatic proxy setup for development servers
- 🌐 **Browser MCP Server**: Puppeteer-like automation directly in the browser
- 📈 **Log Aggregation**: Centralized logging for easier debugging
- 🔒 **SSH Access**: Full remote VSCode integration
- 🛠️ **Custom Startup Scripts**: Personalized environment configuration

## 🏃‍♂️ Quick Start

### Local Development

```bash
# Clone the repository
git clone https://github.com/your-org/catnip.git
cd catnip

# Install dependencies
pnpm install

# Start the development server
pnpm dev
```

Visit `http://localhost:3000` to access the CatNip interface.

### Cloud Deployment

Deploy to Cloudflare Containers for global access:

```bash
# Build and deploy
pnpm deploy
```

## 🏗️ Architecture

CatNip is built with a modern, scalable architecture:

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   React/Vite    │    │ Cloudflare      │    │   Go Container  │
│   Frontend      │◄──►│ Worker (Hono)   │◄──►│   Environment   │
│                 │    │                 │    │                 │
│ • ShadCN UI     │    │ • WebSocket     │    │ • PTY Sessions  │
│ • TanStack      │    │ • Container     │    │ • Git Server    │
│ • Tailwind      │    │   Management    │    │ • API Endpoints │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## 🛠️ Development

### Prerequisites

- Node.js 18+ and pnpm
- Docker and Docker Compose
- Go 1.21+ (for container development)

### Development Commands

```bash
# Frontend development
pnpm dev              # Start Vite dev server
pnpm dev:cf          # Start with Cloudflare Workers
pnpm build           # Build for production
pnpm typecheck       # Type checking
pnpm lint            # ESLint

# Container development
cd container
go run cmd/server/main.go  # Start Go server
```

### Working with the Container

```bash
# Execute commands in the dev container
docker exec -it catnip-dev bash --login -c 'your-command'

# Check logs
docker logs --tail 200 catnip-dev
```

## 📚 Documentation

- **[Directory Structure](./CLAUDE.md#directory-structure)**: Project organization
- **[Git Operations](./docs/GIT.md)**: Advanced git workflows
- **[Settings Sync](./docs/SETTINGS_SYNC.md)**: Configuration management
- **[Local Repositories](./docs/LOCAL_REPOSITORIES.md)**: Repository handling

## 🤝 Contributing

We welcome contributions! CatNip is designed to make agentic programming more powerful and accessible.

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🌟 Why "CatNip"?

Just like how catnip makes cats go crazy with excitement, CatNip makes AI coding assistants incredibly productive and effective. It's the perfect environment for unleashing the full potential of agentic programming!

---

**Ready to supercharge your AI coding workflows?** Give CatNip a try and experience the future of collaborative development! 🚀