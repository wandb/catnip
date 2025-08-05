# Catnip

> **The developer environment that's like catnip for agentic programming**

Catnip supercharges your development workflow by providing a **containerized environment** that can effortlessly run multiple agents in parallel.  Catnip was purpose built for Claude Code, but additional agentic toolkits will be supported in the future.

## 🚀 Why Catnip?

Git worktree's, MCP servers, live previews, unified logging and much more come for free when using Catnip.

- **🔒 Isolated Sandbox**: All code runs containerized environment using either Docker or Apple's new [Container SDK](https://github.com/apple/container).  We can use --dangerously-skip-permissions without fear!
- **🧑‍💻 Worktree Management**: Worktree's let you spawn multiple agents in parallel.  Catnip keeps everything organized.
- **💻 Full Terminal Access**: Open mutliple terminals via the web interface, CLI, or directly via SSH.
- **👀 Preview Changes**: Catnip has a built in proxy and port detection.  Start a web service and preview it live!
- **🌐 Universal Access**: Still a big fan of Cursor or VS Code?  No problem, full remote development directly in your IDE is supported.

## 🏃‍♂️ Quick Start

```bash
curl -sSfL install.catnip.sh | sh
catnip run
```

`http://localhost:8080` will open in your default browser.

## 🤓 How it works

`catnip` is a golang binary with a vite SPA embedded within it.  The `wandb/catnip` container was inspired by the [openai/codex-universal](https://github.com/openai/codex-universal) container.  It comes pre-configured with node, python, golang, gcc, and rust.  You can have the container install a different version of the language on boot by setting any of these environment variables:

- CATNIP_NODE_VERSION
- CATNIP_PYTHON_VERSION
- CATNIP_RUST_VERSION
- CATNIP_GO_VERSION

In the future we intend to support custom base images.  The `catnip run` command also configures SSH witnin the container by default.  It creates a key pair named `catnip_remote` and configures a `catnip` host allowing you to run `ssh catnip` or open a remote development environment via the [Remote-SSH extension](https://marketplace.cursorapi.com/items/?itemName=anysphere.remote-ssh).

When you start a claude session in Catnip the system automatically commits changes as claude makes them.  We intend to support restoring to a previous checkpoint in a future release.

## 🤝 Contributing

We welcome contributions! CatNip is designed to make agentic programming more powerful and accessible.

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

**Ready to supercharge your AI coding workflows?** Give CatNip a try and experience the future of collaborative development! 🚀