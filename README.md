<div align="center">
  <img src="public/logo@2x.webp" alt="Catnip Logo" width="200"/>

# 🐾 Catnip

**The developer environment that's like catnip for agentic programming.**

**🎯 Perfect for**: Building LLM applications, agentic systems, or any project where you want multiple AI coding assistants working in parallel without stepping on each other's toes.

[![GitHub Stars](https://img.shields.io/github/stars/wandb/catnip?style=social)](https://github.com/wandb/catnip)
[![Docker Pulls](https://img.shields.io/docker/pulls/wandb/catnip)](https://hub.docker.com/r/wandb/catnip)
[![Version](https://img.shields.io/github/v/release/wandb/catnip)](https://github.com/wandb/catnip/releases)
<br/>

**🔥 Parallel vibe coding in containers. Stay organized, get notified, create anything! 😎**

<img src="public/screenshot.png" alt="Catnip UI Screenshot"/>

</div>

## 💡 When to Use Catnip

**You should use Catnip if you:**

- Build LLM applications (chatbots, RAG systems, AI agents) and want AI assistants to help code different parts simultaneously
- Use Claude Code (or plan to) but wish you could run multiple sessions on different features without Git conflicts
- Want a safe, isolated environment where AI assistants can run terminal commands without risking your main system
- Build full-stack applications and need multiple services (API, frontend, database) running with automatic port management
- Work on complex projects and want AI assistants to collaborate on different components in parallel


## 🚀 Why Catnip?

Think of Catnip as a **multi-agent coding workspace** that solves the chaos of having AI assistants work together on complex projects.

**The Problem:** You want Claude Code (or other AI assistants) to help build your LLM app, but:

- You can't run multiple Claude sessions on the same project (Git checkout conflicts)
- AI assistants might break your main development environment
- Managing different services (API, frontend, database) manually is tedious
- You lose track of what each AI assistant is working on

**The Solution:** Catnip gives each AI assistant its own isolated workspace while keeping everything synchronized:

- **🔒 Isolated Sandbox**: All code runs containerized environment using either Docker or Apple's new [Container SDK]
(https://github.com/apple/container). We can use --dangerously-skip-permissions without fear!
- **🧑‍💻 Worktree Management**: Worktree's let you spawn multiple agents in parallel. Catnip keeps everything organized.
- **💻 Full Terminal Access**: Open multiple terminals via the web interface, CLI, or directly via SSH.
- **👀 Preview Changes**: Catnip has a built in proxy and port detection. Start a web service and preview it live!
- **🌐 Universal Access**: Still a big fan of Cursor or VS Code? No problem, full remote development directly in your IDE is 
supported.

## ⚡ Quick Start

```bash
curl -sSfL install.catnip.sh | sh
# Optionally start catnip from an existing git repo
cd ~/Development/my_awesome_project
catnip run
# Open http://localhost:8080 🎉
```

## 🎯 AI Engineering Workflows

### Multi-Agent Development Pattern

```mermaid
graph TB
    subgraph project ["🚀 AI Engineering Project"]
        subgraph agents ["🤖 Parallel AI Agents"]
            agent1["<b>Claude Agent 1</b><br/>Feature Development"]
            agent2["<b>Claude Agent 2</b><br/>Model Training"]
            agent3["<b>Claude Agent 3</b><br/>API Integration"]
        end
        
        subgraph worktrees ["📁 Isolated Worktrees"]
            wt1["<b>refs/catnip/feature-dev</b><br/>Working Directory 1"]
            wt2["<b>refs/catnip/ml-experiment</b><br/>Working Directory 2"] 
            wt3["<b>refs/catnip/api-work</b><br/>Working Directory 3"]
        end
        
        subgraph services ["🔗 Live Services"]
            jupyter["<b>Jupyter Lab</b><br/>:8888"]
            api["<b>FastAPI</b><br/>:8000"]
            frontend["<b>React Dev</b><br/>:3000"]
            streamlit["<b>Streamlit</b><br/>:8501"]
        end
        
        subgraph branches ["🌿 Synced Branches"]
            br1["<b>feature/auth-system</b><br/>Ready for PR"]
            br2["<b>feature/model-optimization</b><br/>Ready for PR"]
            br3["<b>feature/api-endpoints</b><br/>Ready for PR"]
        end
    end
    
    agent1 -.->|"Works in"| wt1
    agent2 -.->|"Works in"| wt2  
    agent3 -.->|"Works in"| wt3
    
    wt1 -.->|"Auto-syncs"| br1
    wt2 -.->|"Auto-syncs"| br2
    wt3 -.->|"Auto-syncs"| br3
    
    agents -.->|"Access"| services
    
    classDef agentStyle fill:#10b981,stroke:#047857,stroke-width:2px,color:#ffffff
    classDef worktreeStyle fill:#2563eb,stroke:#1e40af,stroke-width:2px,color:#ffffff  
    classDef serviceStyle fill:#7c3aed,stroke:#6d28d9,stroke-width:2px,color:#ffffff
    classDef branchStyle fill:#f59e0b,stroke:#d97706,stroke-width:2px,color:#ffffff
    
    class agent1,agent2,agent3 agentStyle
    class wt1,wt2,wt3 worktreeStyle
    class jupyter,api,frontend,streamlit serviceStyle
    class br1,br2,br3 branchStyle
```

### Example: LLM Application Development

```bash
# Start with LLM development environment
catnip run -e ANTHROPIC_API_KEY -e OPENAI_API_KEY

# Agent 1: RAG Backend Development  
# - FastAPI service with vector database integration
# - Embedding pipelines and retrieval logic
# - Auto-detected on port 8000, proxied at localhost:8080/8000

# Agent 2: Chat Interface Development
# - React/Next.js frontend with streaming chat UI
# - Real-time message handling via WebSocket
# - Live preview at localhost:3000

# Agent 3: Data Pipeline & Tools
# - Document ingestion scripts
# - Streamlit dashboard for data exploration
# - Vector database management tools

# All services accessible through Catnip's unified interface
```

### Example: Multi-Agent System Development

```bash
# Environment for building agentic applications
catnip run -e ANTHROPIC_API_KEY \
           -e CATNIP_PYTHON_VERSION=3.11 \
           -e DATABASE_URL

# Parallel development workflow:
# Workspace 1: Agent orchestration system
# Workspace 2: Tool integration and function calling
# Workspace 3: Memory and state management
# Workspace 4: Web interface and monitoring dashboard

# Services auto-detected and forwarded:
# - FastAPI orchestrator: localhost:8000
# - Streamlit monitoring: localhost:8501
# - React admin panel: localhost:3000
# - Jupyter for experimentation: localhost:8888
```

## 🤓 How it works

```mermaid
graph TB
    subgraph host ["🖥️ Host System"]
        catnip["<b>catnip</b><br/>Go Binary"]
    end

    subgraph container ["🐳 wandb/catnip Container"]
        server["<b>Catnip Server</b><br/>Port 8080"]

        subgraph worktrees ["📁 Git Worktrees"]
            wt1["<b>main</b><br/>worktree"]
            wt2["<b>feature-a</b><br/>worktree"]
            wt3["<b>feature-b</b><br/>worktree"]
        end

        subgraph services ["🚀 Services"]
            svc1["<b>Port 3000</b><br/>Service"]
            svc2["<b>Port 5000</b><br/>Service"]
            svc3["<b>Port 8000</b><br/>Service"]
        end
    end

    catnip -.->|"🚀 Launches"| server
    server -.->|"📋 Manages"| wt1
    server -.->|"📋 Manages"| wt2
    server -.->|"📋 Manages"| wt3
    server -.->|"🔀 Proxies"| svc1
    server -.->|"🔀 Proxies"| svc2
    server -.->|"🔀 Proxies"| svc3

    classDef hostStyle fill:#2563eb,stroke:#1e40af,stroke-width:2px,color:#ffffff
    classDef serverStyle fill:#7c3aed,stroke:#6d28d9,stroke-width:2px,color:#ffffff
    classDef worktreeStyle fill:#2563eb,stroke:#1e40af,stroke-width:2px,color:#ffffff
    classDef serviceStyle fill:#7c3aed,stroke:#6d28d9,stroke-width:2px,color:#ffffff

    class catnip hostStyle
    class server serverStyle
    class wt1,wt2,wt3 worktreeStyle
    class svc1,svc2,svc3 serviceStyle
```

`catnip` is a golang binary with a vite SPA embedded in it. The [wandb/catnip](./container/Dockerfile) container was inspired by the [openai/codex-universal](https://github.com/openai/codex-universal) container.

It comes pre-configured with node, python, golang, gcc, and rust. You can have the container install a different version of the language on boot by setting any of these environment variables:

```bash
# Set specific language versions for AI development
CATNIP_NODE_VERSION=20.11.0
CATNIP_PYTHON_VERSION=3.12
CATNIP_RUST_VERSION=1.75.0
CATNIP_GO_VERSION=1.22
```

> [!NOTE]
> In the future we intend to support custom base images.

### Environment Setup

Catnip currently looks for a file named `setup.sh` in the root of your repo and runs it when a workspace is created. This is a great place to run `pnpm install`, `pip install -r requirements.txt`, or `uv sync` - perfect for AI projects with complex dependencies.

```bash
#!/bin/bash
# Example setup.sh for LLM application development
pip install -r requirements.txt
# Pre-load common dependencies for LLM apps
pip install openai anthropic chromadb
npm install  # For full-stack AI applications
# Set up vector database or other services
docker-compose up -d --build
```

### Environment variables

`catnip run` accepts `-e` arguments. For instance if you want to pass `ANTHROPIC_API_KEY` from your host into the container you can simply add `-e ANTHROPIC_API_KEY` and then all terminals and AI agent sessions within the container will see that variable. You can also explicitly set variables, `-e ANTHROPIC_BASE_URL=https://some.otherprovider.com/v1`

```bash
# Essential for LLM application development
catnip run -e ANTHROPIC_API_KEY -e OPENAI_API_KEY -e PINECONE_API_KEY
```

### SSH

The `catnip run` command configures SSH within the container by default. It creates a key pair named `catnip_remote` and configures a `catnip` host allowing you to run `ssh catnip` or open a remote development environment via the [Remote-SSH extension](https://marketplace.cursorapi.com/items/?itemName=anysphere.remote-ssh). This works perfectly with Cursor, VS Code, and other editors that AI engineers commonly use. You can disable ssh by adding `--disable-ssh` to the run command.

### Docker in Docker

If you want the catnip container to be able to run `docker` commands, pass the `--dind` flag to the `catnip run` command. This mounts the docker socket from the host into the container allowing your terminals and AI agents to build or run containers - useful for containerized ML services or complex multi-service applications.

### Git

If you run `catnip` from within a git repo, we mount the repo into the container and create a default workspace. When you start a Claude session in Catnip the system automatically commits changes as Claude makes them.

> [!TIP]
> The workspace within the container is committing to a custom ref `refs/catnip/$NAME`. For convenience we also create a nicely named branch like `feature/make-something-great`. This branch is kept in sync with the workspace ref which means you can run `git checkout feature/make-something-great` outside of the container to see changes locally - perfect for AI-assisted development workflows where you want to review agent changes!

We also run a git server in the container. You will see a Git option in the "Open in..." menu that will provide you with a clone command like:

```bash
git clone -o catnip http://localhost:8080/my-sick-repo.git
```

As you create new workspaces in the container, you can run `git fetch catnip` back on your host to see your changes outside of the container!

### Ports

Catnip forwards ports directly to the host system. When a service starts within the container, Catnip automatically detects and forwards the port, making it accessible at `http://localhost:$PORT`. Each workspace also has the `PORT` environment variable set to a known free port. For convenience, services can also be accessed through the Catnip UI proxy at `http://localhost:8080/$PORT`.

This is especially powerful for LLM and agentic application development where you might have:

- FastAPI backends with LLM integration on port 8000
- React/Next.js chat interfaces on port 3000
- Streamlit data exploration dashboards on port 8501
- Jupyter notebooks for experimentation on port 8888
- Vector databases and other services on various ports

> [!NOTE]
> If a port isn't bindable on the host (e.g., already in use), Catnip will automatically find and use the first available port instead. The UI will notify you of the actual port being used.

## 🗺️ Roadmap

### Coming Soon

- [ ] 🎯 Custom base images
- [ ] 🔄 Restore to previous checkpoints
- [ ] 🤖 Support for more AI coding agents
- [ ] 🌐 Cloud based deployments
- [ ] 🔧 Plugin ecosystem

## ❓ FAQ

<details>
<summary><b>How is Catnip different from Jules, Open SWE, or Conductor</b></summary>
Catnip is Open Source, built to be extensible, and prioritizes local development first with support for cloud based deployments on the roadmap. It's specifically designed for AI engineers who need sophisticated multi-agent orchestration with powerful Git worktree management and real-time service discovery.
</details>
<details>
<summary><b>What AI assistants does Catnip support?</b></summary>

Currently optimized for Claude Code, with support for additional AI coding assistants likely coming soon. The architecture is designed to be extensible for the growing ecosystem of AI development tools.

</details>
<details>
<summary><b>Can I use this for LLM and AI application projects?</b></summary>
Absolutely! Catnip is perfect for LLM app development. The containerized environment handles complex dependencies (vector databases, embedding models, etc.), automatic port detection works great with Jupyter/Streamlit/FastAPI, and the multi-agent system lets you parallelize RAG backend development, chat interface building, and data pipeline work.
</details>
<details>
<summary><b>How does the Git worktree system work with multiple AI agents?</b></summary>
Each agent works in an isolated worktree using custom `refs/catnip/*` references, preventing Git checkout conflicts. Catnip automatically creates and syncs "nice" feature branches for PRs, so you get the isolation you need for parallel agents while maintaining clean Git workflows.
</details>
<details>
<summary><b>Did you develop Catnip with Catnip?</b></summary>
Big time... Inception 🤯 We've been using Catnip to build Catnip, which has been invaluable for dogfooding the multi-agent workflow experience.
</details>

## 🤝 Contributing

We welcome contributions! Catnip is designed to make agentic programming more powerful and accessible for AI engineers.

1. 🍴 Fork the repository
2. 🌿 Run catnip in dev mode `catnip run --dev` (you must run this from within the catnip repo)
3. 💻 Make your changes
4. ✅ Add tests if applicable
5. 📤 Submit a pull request

The codebase includes both a Go backend for container orchestration and Git operations, plus a React/TypeScript frontend for the web interface. Contributing to AI agent integration, multi-workspace management, or real-time features are all great ways to help improve the platform for AI development workflows.

## 📄 License

This project is licensed under the Apache 2.0 - see the [LICENSE](LICENSE) file for details.

---

<div align="center">

**Made with ❤️ by the [Weights & Biases](https://wandb.ai) team**
<br/> <a href="https://github.com/wandb/catnip">
<img src="https://img.shields.io/badge/⭐_Star_Catnip-000000?style=for-the-badge&logo=github&logoColor=white" alt="Star on GitHub"/>
</a>

</div>
