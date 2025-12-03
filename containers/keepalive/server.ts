/**
 * Ephemeral Keep-Alive Container for GitHub Codespaces
 *
 * This container is designed to be short-lived:
 * - Starts on demand when called by the coordinator
 * - Executes a single SSH ping to keep the codespace alive
 * - Shuts down immediately after responding (no sleepAfter)
 *
 * State management and rate limiting happen in the KeepAliveCoordinator DO.
 */

// Configuration constants
const PORT = parseInt(process.env.PORT || "8080");
const COMMAND_TIMEOUT_MS = 30000; // 30 seconds

interface PingRequest {
  githubToken: string;
}

interface PingResponse {
  success: boolean;
  codespaceName: string;
  output?: string;
  error?: string;
  stderr?: string;
  timestamp: number;
}

// Health check endpoint
const healthHandler = () => {
  return Response.json({ status: "ok", timestamp: Date.now() });
};

// Keep-alive ping endpoint
const pingHandler = async (
  req: Request,
  codespaceName: string,
): Promise<Response> => {
  if (!codespaceName) {
    return Response.json(
      { error: "Missing codespaceName parameter" },
      { status: 400 },
    );
  }

  let body: PingRequest;
  try {
    body = await req.json();
  } catch {
    return Response.json({ error: "Invalid JSON body" }, { status: 400 });
  }

  const { githubToken } = body;

  if (!githubToken) {
    return Response.json(
      { error: "Missing githubToken in request body" },
      { status: 400 },
    );
  }

  console.log(`🫀 Pinging codespace: ${codespaceName}`);

  try {
    // Execute: gh codespace ssh -c {name} -- uptime
    const proc = Bun.spawn(
      ["gh", "codespace", "ssh", "-c", codespaceName, "--", "uptime"],
      {
        env: {
          ...process.env,
          GH_TOKEN: githubToken,
        },
        stdout: "pipe",
        stderr: "pipe",
      },
    );

    // Wait for process to complete with timeout
    const timeoutPromise = new Promise<never>((_, reject) => {
      setTimeout(() => {
        proc.kill(); // Kill the process on timeout
        reject(
          new Error(
            `Command timeout after ${COMMAND_TIMEOUT_MS / 1000} seconds`,
          ),
        );
      }, COMMAND_TIMEOUT_MS);
    });

    await Promise.race([proc.exited, timeoutPromise]);

    const stdout = await new Response(proc.stdout).text();
    const stderr = await new Response(proc.stderr).text();

    if (proc.exitCode !== 0) {
      throw new Error(
        `gh command failed with exit code ${proc.exitCode}: ${stderr}`,
      );
    }

    console.log(`✅ Keep-alive successful for ${codespaceName}`);
    console.log(`   Output: ${stdout.trim()}`);

    const response: PingResponse = {
      success: true,
      codespaceName,
      output: stdout.trim(),
      timestamp: Date.now(),
    };

    return Response.json(response);
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : String(error);
    console.error(`❌ Keep-alive failed for ${codespaceName}:`, errorMessage);

    const response: PingResponse = {
      success: false,
      error: errorMessage,
      codespaceName,
      timestamp: Date.now(),
    };

    return Response.json(response, { status: 500 });
  }
};

// Simple router
const server = Bun.serve({
  port: PORT,
  async fetch(req) {
    const url = new URL(req.url);

    // Health check
    if (url.pathname === "/health" && req.method === "GET") {
      return healthHandler();
    }

    // Ping endpoint: POST /ping/:codespaceName
    const pingMatch = url.pathname.match(/^\/ping\/(.+)$/);
    if (pingMatch && req.method === "POST") {
      const codespaceName = pingMatch[1];
      return pingHandler(req, codespaceName);
    }

    return Response.json({ error: "Not found" }, { status: 404 });
  },
});

console.log(`🫀 Keep-alive service listening on port ${PORT}`);
