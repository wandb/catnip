FROM ubuntu:24.04

# Build arguments for language versions with stable defaults
ARG NODE_VERSION=22.17.0
ARG PYTHON_VERSION=3.13.5
ARG RUST_VERSION=1.88.0
ARG GO_VERSION=1.24.4
ARG NVM_VERSION=0.40.3

# Multi-arch support
ARG TARGETPLATFORM
ARG BUILDPLATFORM

# Avoid prompts from apt during build
ENV DEBIAN_FRONTEND=noninteractive

# Install base packages, build tools, and system utilities
RUN apt-get update && apt-get install -y \
    curl \
    wget \
    git \
    vim \
    nano \
    unzip \
    zip \
    jq \
    tree \
    htop \
    ca-certificates \
    build-essential \
    pkg-config \
    libssl-dev \
    libffi-dev \
    libsqlite3-dev \
    libbz2-dev \
    libreadline-dev \
    libncurses5-dev \
    libncursesw5-dev \
    xz-utils \
    tk-dev \
    libxml2-dev \
    libxmlsec1-dev \
    libffi-dev \
    liblzma-dev \
    zlib1g-dev \
    sudo \
    && rm -rf /var/lib/apt/lists/*

# Install additional development tools
RUN apt-get update && apt-get install -y \
    make \
    cmake \
    clang \
    lldb \
    gdb \
    strace \
    ltrace \
    tcpdump \
    netcat-openbsd \
    telnet \
    openssh-client \
    rsync \
    just \
    python3-pip \
    python3-venv \
    pipx \
    gosu \
    && rm -rf /var/lib/apt/lists/*

# Install GitHub CLI
RUN mkdir -p -m 755 /etc/apt/keyrings && \
    wget -qO- https://cli.github.com/packages/githubcli-archive-keyring.gpg | tee /etc/apt/keyrings/githubcli-archive-keyring.gpg > /dev/null && \
    chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null && \
    apt-get update && \
    apt-get install -y gh && \
    rm -rf /var/lib/apt/lists/*

# Install Docker CLI (for docker-in-docker scenarios)
RUN curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu noble stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null && \
    apt-get update && \
    apt-get install -y docker-ce-cli && \
    rm -rf /var/lib/apt/lists/*

# Remove default ubuntu user if it exists and create catnip user with UID 1000
RUN if id ubuntu >/dev/null 2>&1; then userdel -r ubuntu; fi && \
    useradd -m -s /bin/bash -u 1000 catnip && \
    usermod -aG sudo catnip && \
    echo 'catnip ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers

# Create global workspace directory
RUN mkdir -p /workspace && \
    chown catnip:catnip /workspace

# Set up global environment variables and PATH
ENV CATNIP_ROOT="/opt/catnip"
ENV WORKSPACE="/workspace"
ENV PATH="${CATNIP_ROOT}/bin:${PATH}"

# Create directory structure
RUN mkdir -p ${CATNIP_ROOT}/bin ${CATNIP_ROOT}/lib ${CATNIP_ROOT}/versions && \
    chown -R catnip:catnip ${CATNIP_ROOT}

# Install language version managers globally
# Install NVM globally
RUN curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v${NVM_VERSION}/install.sh | bash && \
    mv /root/.nvm ${CATNIP_ROOT}/nvm && \
    chown -R catnip:catnip ${CATNIP_ROOT}/nvm
ENV NVM_DIR="${CATNIP_ROOT}/nvm"

# Install Rust globally
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain ${RUST_VERSION} && \
    mv /root/.cargo ${CATNIP_ROOT}/cargo && \
    mv /root/.rustup ${CATNIP_ROOT}/rustup && \
    chown -R catnip:catnip ${CATNIP_ROOT}/cargo ${CATNIP_ROOT}/rustup
ENV CARGO_HOME="${CATNIP_ROOT}/cargo"
ENV RUSTUP_HOME="${CATNIP_ROOT}/rustup"

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

# Copy and set up global shell environment for all users
COPY setup/catnip-profile.sh /etc/profile.d/catnip.sh
RUN chmod +x /etc/profile.d/catnip.sh

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

# Install Starship prompt
RUN curl -sS https://starship.rs/install.sh | sh -s -- -y

# Configure Starship with catnip config
COPY setup/starship.toml /etc/starship.toml
RUN mkdir -p /home/catnip/.config /root/.config && \
    cp /etc/starship.toml /home/catnip/.config/starship.toml && \
    cp /etc/starship.toml /root/.config/starship.toml && \
    chown -R catnip:catnip /home/catnip/.config && \
    echo 'eval "$(starship init bash)"' >> /home/catnip/.bashrc && \
    echo 'eval "$(starship init bash)"' >> /root/.bashrc

# Setup dynamic welcome banner and suppress sudo message
COPY setup/generate-banner.sh /usr/local/bin/generate-banner.sh
RUN chmod +x /usr/local/bin/generate-banner.sh && \
    rm -f /etc/motd && \
    touch /etc/motd && \
    echo '/usr/local/bin/generate-banner.sh' >> /home/catnip/.bashrc && \
    echo '/usr/local/bin/generate-banner.sh' >> /root/.bashrc && \
    touch /home/catnip/.sudo_as_admin_successful && \
    chown catnip:catnip /home/catnip/.sudo_as_admin_successful

# Install Claude Code as catnip user (not root)
USER catnip
RUN bash -c 'source /etc/profile.d/catnip.sh && \
    source "$NVM_DIR/nvm.sh" && \
    npm install -g @anthropic-ai/claude-code'

# Switch back to root for final setup
USER root

# Copy and set up entrypoint script
COPY setup/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Set working directory
WORKDIR /workspace

# Default command
ENTRYPOINT ["/entrypoint.sh"]
CMD ["/bin/bash"]