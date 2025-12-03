FROM oven/bun:1-alpine

# Install gh CLI and openssh-client
RUN apk add --no-cache github-cli openssh-client

WORKDIR /app

# Copy package files
COPY package.json bun.lock* ./

# Install dependencies
RUN bun install --frozen-lockfile --production

# Copy server code
COPY server.ts ./

# Expose port
EXPOSE 8080

# Run server
CMD ["bun", "run", "server.ts"]
