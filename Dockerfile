# Stage 1: Build the Go binary
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Install just and swag
RUN apk add just

# Copy go mod files
COPY container/go.mod container/go.sum ./
RUN go mod download && \
    go install github.com/swaggo/swag/cmd/swag@latest

# Copy source code
COPY container/ .

# Create a simple index.html for embedding
RUN mkdir -p ./internal/assets/dist && \
    cat > ./internal/assets/dist/index.html << 'EOF'
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Catnip Development Environment</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            background: #1a1a1a;
            color: #ffffff;
        }
        .container {
            text-align: center;
            padding: 2rem;
            background: #2a2a2a;
            border-radius: 12px;
            box-shadow: 0 4px 16px rgba(0,0,0,0.3);
            max-width: 600px;
        }
        h1 {
            color: #ffffff;
            margin-bottom: 1rem;
            font-size: 2.5rem;
        }
        p {
            color: #cccccc;
            line-height: 1.6;
            margin-bottom: 1rem;
        }
        code {
            background: #3a3a3a;
            padding: 4px 8px;
            border-radius: 4px;
            font-family: 'Monaco', 'Consolas', monospace;
            color: #00ff88;
        }
        .links {
            margin-top: 2rem;
            padding-top: 2rem;
            border-top: 1px solid #3a3a3a;
        }
        a {
            color: #00ff88;
            text-decoration: none;
            margin: 0 1rem;
        }
        a:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🐱 Catnip</h1>
        <p>Your cloud development environment is running!</p>
        <p>Use GitHub CLI: <code>gh auth login</code></p>
        <p>Or use Claude Code: <code>claude-code --help</code></p>
        <div class="links">
            <a href="/health">Health Check</a>
            <a href="/swagger/">API Documentation</a>
        </div>
    </div>
</body>
</html>
EOF

# Generate swagger documentation
RUN just swagger

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o catnip cmd/cli/main.go

# Stage 2: Final runtime image
FROM ubuntu:24.04

# Build arguments for language versions
ARG NODE_VERSION=22.17.0
ARG PYTHON_VERSION=3.13.5
ARG GO_VERSION=1.24.4
ARG NVM_VERSION=0.40.3

# Multi-arch support
ARG TARGETPLATFORM

# Avoid prompts from apt during build
ENV DEBIAN_FRONTEND=noninteractive

# Install essential packages only
RUN apt-get update && apt-get install -y \
    curl \
    wget \
    git \
    unzip \
    ca-certificates \
    build-essential \
    pkg-config \
    libssl-dev \
    python3-pip \
    python3-venv \
    pipx \
    sudo \
    && rm -rf /var/lib/apt/lists/*

# Install GitHub CLI
RUN mkdir -p -m 755 /etc/apt/keyrings && \
    wget -qO- https://cli.github.com/packages/githubcli-archive-keyring.gpg | tee /etc/apt/keyrings/githubcli-archive-keyring.gpg > /dev/null && \
    chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null && \
    apt-get update && \
    apt-get install -y gh && \
    rm -rf /var/lib/apt/lists/*

# Remove default ubuntu user if it exists and create catnip user with UID 1000
RUN if id ubuntu >/dev/null 2>&1; then userdel -r ubuntu; fi && \
    useradd -m -s /bin/bash -u 1000 catnip && \
    usermod -aG sudo catnip && \
    echo '#1000 ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers

# Create workspace directory
RUN mkdir -p /workspace && \
    chown catnip:catnip /workspace

# Set up global environment variables and PATH
ENV CATNIP_ROOT="/opt/catnip"
ENV WORKSPACE="/workspace"
ENV PATH="${CATNIP_ROOT}/bin:${PATH}"

# Create directory structure
RUN mkdir -p ${CATNIP_ROOT}/bin ${CATNIP_ROOT}/versions && \
    chown -R catnip:catnip ${CATNIP_ROOT} && \
    chmod -R 755 ${CATNIP_ROOT}

# Install NVM globally
RUN curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v${NVM_VERSION}/install.sh | bash && \
    mv /root/.nvm ${CATNIP_ROOT}/nvm && \
    chown -R catnip:catnip ${CATNIP_ROOT}/nvm
ENV NVM_DIR="${CATNIP_ROOT}/nvm"

# Install Go globally with multi-arch support
RUN ARCH=$(case "${TARGETPLATFORM}" in \
        "linux/amd64") echo "amd64" ;; \
        "linux/arm64") echo "arm64" ;; \
        *) echo "amd64" ;; \
    esac) && \
    wget https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz && \
    tar -C ${CATNIP_ROOT} -xzf go${GO_VERSION}.linux-${ARCH}.tar.gz && \
    rm go${GO_VERSION}.linux-${ARCH}.tar.gz && \
    chown -R catnip:catnip ${CATNIP_ROOT}/go
ENV GOROOT="${CATNIP_ROOT}/go"
ENV GOPATH="${CATNIP_ROOT}/go-workspace"

# Install uv and set up Python
ENV PIPX_BIN_DIR="${CATNIP_ROOT}/bin"
ENV PIPX_HOME="${CATNIP_ROOT}/pipx"
RUN mkdir -p ${CATNIP_ROOT}/pipx && \
    chown -R catnip:catnip ${CATNIP_ROOT}/pipx && \
    pipx install uv && \
    echo "System python: $(python3 --version)" && \
    if [ "${PYTHON_VERSION}" != "system" ]; then \
        echo "Installing Python ${PYTHON_VERSION} via uv..." && \
        ${CATNIP_ROOT}/bin/uv python install ${PYTHON_VERSION} && \
        ${CATNIP_ROOT}/bin/uv python pin ${PYTHON_VERSION}; \
    else \
        echo "Using system Python"; \
    fi && \
    ln -sf /usr/bin/python3 /usr/bin/python

# Create simple profile script for environment setup
RUN echo '#!/bin/bash' > /etc/profile.d/catnip.sh && \
    echo 'export NVM_DIR="${CATNIP_ROOT}/nvm"' >> /etc/profile.d/catnip.sh && \
    echo 'export GOROOT="${CATNIP_ROOT}/go"' >> /etc/profile.d/catnip.sh && \
    echo 'export GOPATH="${CATNIP_ROOT}/go-workspace"' >> /etc/profile.d/catnip.sh && \
    echo 'export PATH="${CATNIP_ROOT}/go/bin:${GOPATH}/bin:${CATNIP_ROOT}/bin:${PATH}"' >> /etc/profile.d/catnip.sh && \
    echo '[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"' >> /etc/profile.d/catnip.sh && \
    echo '[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"' >> /etc/profile.d/catnip.sh && \
    chmod +x /etc/profile.d/catnip.sh

# Install default Node.js version and enable corepack for pnpm
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
ENV COREPACK_DEFAULT_TO_LATEST=0
ENV COREPACK_ENABLE_AUTO_PIN=0
ENV COREPACK_ENABLE_STRICT=0
RUN bash -c 'source /etc/profile.d/catnip.sh && \
    source "$NVM_DIR/nvm.sh" && \
    nvm install ${NODE_VERSION} && \
    nvm use ${NODE_VERSION} && \
    nvm alias default ${NODE_VERSION} && \
    corepack enable && \
    corepack install -g yarn pnpm npm'

ENV GOSU_VERSION 1.17
RUN set -eux; \
# save list of currently installed packages for later so we can clean up
    savedAptMark="$(apt-mark showmanual)"; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates gnupg wget; \
    rm -rf /var/lib/apt/lists/*; \
    \
    dpkgArch="$(dpkg --print-architecture | awk -F- '{ print $NF }')"; \
    wget -O /usr/local/bin/gosu "https://github.com/tianon/gosu/releases/download/$GOSU_VERSION/gosu-$dpkgArch"; \
    wget -O /usr/local/bin/gosu.asc "https://github.com/tianon/gosu/releases/download/$GOSU_VERSION/gosu-$dpkgArch.asc"; \
    \
# verify the signature
    export GNUPGHOME="$(mktemp -d)"; \
    gpg --batch --keyserver hkps://keys.openpgp.org --recv-keys B42F6819007F00F88E364FD4036A9C25BF357DD4; \
    gpg --batch --verify /usr/local/bin/gosu.asc /usr/local/bin/gosu; \
    gpgconf --kill all; \
    rm -rf "$GNUPGHOME" /usr/local/bin/gosu.asc; \
    \
# clean up fetch dependencies
    apt-mark auto '.*' > /dev/null; \
    [ -z "$savedAptMark" ] || apt-mark manual $savedAptMark; \
    apt-get purge -y --auto-remove -o APT::AutoRemove::RecommendsImportant=false; \
    \
    chmod +x /usr/local/bin/gosu; \
# verify that the binary works
    gosu --version; \
    gosu nobody true

# Set up basic shell environment
RUN echo 'source /etc/profile.d/catnip.sh' >> /home/catnip/.bashrc && \
    echo 'source /etc/profile.d/catnip.sh' >> /root/.bashrc

# Set working directory
WORKDIR /workspace

# Copy the catnip binary from builder stage
COPY --from=builder /build/catnip ${CATNIP_ROOT}/bin/catnip
RUN chmod +x ${CATNIP_ROOT}/bin/catnip

# Copy and set up entrypoint script
COPY container/setup/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Switch to catnip user for installing Claude Code
USER catnip

RUN bash -c 'source /etc/profile.d/catnip.sh && \
    source "$NVM_DIR/nvm.sh" && \
    npm install -g @anthropic-ai/claude-code && \
    rm -rf $(npm root -g)/@anthropic-ai/claude-code/vendor'

# Switch back to root for entrypoint
USER root

# Expose port
EXPOSE 8080

# Default entrypoint and command
ENTRYPOINT ["/entrypoint.sh"]
CMD ["catnip", "serve"]