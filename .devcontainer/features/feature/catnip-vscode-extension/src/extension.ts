import * as vscode from "vscode";
import { exec } from "child_process";
import { promisify } from "util";
import * as http from "http";
import * as QRCode from "qrcode";

const execAsync = promisify(exec);

// Device detection function
function isMobileDevice(): boolean {
  // In VSCode extension context, we can check the environment
  // If we're in a mobile browser or mobile VSCode environment
  const userAgent = process.env.VSCODE_USER_AGENT || "";
  const platform = process.platform;

  // Check for mobile user agents or environments
  return (
    /android|iphone|ipad|mobile/i.test(userAgent) ||
    platform === "android" ||
    process.env.CODESPACE_MOBILE === "true"
  );
}

// Catnip API types
interface WorktreeInfo {
  id: string;
  repo_id: string;
  name: string;
  branch: string;
  path: string;
  commit_hash: string;
  created_at: string;
  last_accessed: string;
  // Optional display fields
  pull_request_title?: string;
  latest_session_title?: string;
  latest_user_prompt?: string;
  // Activity state
  claude_activity_state?: "inactive" | "running" | "active";
}

// Get workspace title using same priority as web app
function getWorkspaceTitle(worktree: WorktreeInfo): string {
  // Priority 1: Use PR title if available
  if (worktree.pull_request_title) {
    return worktree.pull_request_title;
  }

  // Priority 2: Use latest session title if available
  if (worktree.latest_session_title) {
    return worktree.latest_session_title;
  }

  // Priority 3: Use latest user prompt if available
  if (worktree.latest_user_prompt) {
    return worktree.latest_user_prompt;
  }

  // Priority 4: Use workspace name
  const workspaceName = worktree.name.split("/")[1] || worktree.name;
  return workspaceName;
}

// Get status indicator icon based on activity state (matching web app)
function getStatusIcon(
  activityState?: "inactive" | "running" | "active",
): vscode.ThemeIcon {
  switch (activityState) {
    case "active":
      // Green filled circle (active/running Claude session)
      return new vscode.ThemeIcon(
        "circle-filled",
        new vscode.ThemeColor("charts.green"),
      );
    case "running":
      // Gray filled circle (recent activity)
      return new vscode.ThemeIcon(
        "circle-filled",
        new vscode.ThemeColor("charts.gray"),
      );
    case "inactive":
    default:
      // Empty circle with border (inactive)
      return new vscode.ThemeIcon(
        "circle-outline",
        new vscode.ThemeColor("charts.gray"),
      );
  }
}

// Catnip health check using HTTP
async function isCatnipRunning(): Promise<boolean> {
  return new Promise((resolve) => {
    const req = http.get(
      {
        hostname: "localhost",
        port: 6369,
        path: "/v1/info",
        timeout: 1000,
      },
      (res) => {
        resolve(res.statusCode === 200);
      },
    );

    req.on("error", () => {
      resolve(false);
    });

    req.on("timeout", () => {
      req.destroy();
      resolve(false);
    });
  });
}

// Fetch worktrees from catnip API
async function getWorktrees(): Promise<WorktreeInfo[]> {
  return new Promise((resolve) => {
    const req = http.get(
      {
        hostname: "localhost",
        port: 6369,
        path: "/v1/git/worktrees",
        timeout: 1000,
      },
      (res) => {
        let data = "";

        res.on("data", (chunk) => {
          data += chunk;
        });

        res.on("end", () => {
          try {
            if (res.statusCode === 200) {
              // API returns array directly, not wrapped
              const worktrees: WorktreeInfo[] = JSON.parse(data);
              resolve(Array.isArray(worktrees) ? worktrees : []);
            } else {
              resolve([]);
            }
          } catch (error) {
            console.error("Failed to parse worktrees response:", error);
            resolve([]);
          }
        });
      },
    );

    req.on("error", (error) => {
      console.error("Failed to fetch worktrees:", error);
      resolve([]);
    });

    req.on("timeout", () => {
      req.destroy();
      resolve([]);
    });
  });
}

class CatnipViewProvider implements vscode.TreeDataProvider<CatnipItem> {
  private _onDidChangeTreeData: vscode.EventEmitter<
    CatnipItem | undefined | null | void
  > = new vscode.EventEmitter<CatnipItem | undefined | null | void>();
  readonly onDidChangeTreeData: vscode.Event<
    CatnipItem | undefined | null | void
  > = this._onDidChangeTreeData.event;

  getTreeItem(element: CatnipItem): vscode.TreeItem {
    return element;
  }

  async getChildren(element?: CatnipItem): Promise<CatnipItem[]> {
    // If element is a repository group, return its worktrees
    if (element?.worktrees) {
      return element.worktrees.map((worktree) => {
        const title = getWorkspaceTitle(worktree);
        const tooltip = `${worktree.path}\nBranch: ${worktree.branch}`;
        const item = new CatnipItem(
          title,
          tooltip,
          vscode.TreeItemCollapsibleState.None,
          "catnip.openWorktree",
          worktree.path,
        );
        item.iconPath = getStatusIcon(worktree.claude_activity_state);
        return item;
      });
    }

    if (!element) {
      const running = await isCatnipRunning();

      if (!running) {
        return [
          new CatnipItem(
            "❌ Catnip Not Running",
            "Waiting for catnip to start...",
            vscode.TreeItemCollapsibleState.None,
            undefined,
          ),
        ];
      }

      const items: CatnipItem[] = [];
      const isMobile = isMobileDevice();
      const isCodespace = !!process.env.CODESPACE_NAME;

      // Add interface controls at the top
      if (isMobile) {
        items.push(
          new CatnipItem(
            "🐱",
            "Open Catnip Interface",
            vscode.TreeItemCollapsibleState.None,
            "catnip.openInterface",
          ),
        );
      } else {
        items.push(
          new CatnipItem(
            "💻 Open Catnip Interface",
            "Click to open the catnip development environment",
            vscode.TreeItemCollapsibleState.None,
            "catnip.openInterface",
          ),
        );

        // Only show QR code button in Codespaces
        if (isCodespace) {
          items.push(
            new CatnipItem(
              "📱 Open on Mobile",
              "Scan QR code to open on mobile device",
              vscode.TreeItemCollapsibleState.None,
              "catnip.showQR",
            ),
          );
        }
      }

      // Add separator
      items.push(
        new CatnipItem(
          "────────────",
          "Workspaces",
          vscode.TreeItemCollapsibleState.None,
          undefined,
        ),
      );

      // Add "home" button only when in Codespace and inside a worktree
      const workspaceFolders = vscode.workspace.workspaceFolders;
      let currentWorkspaceRepoName: string | undefined;
      if (workspaceFolders && workspaceFolders.length > 0) {
        const currentPath = workspaceFolders[0].uri.fsPath;
        const currentName = currentPath.split("/").pop() || currentPath;
        // Extract repo name from path for grouping
        currentWorkspaceRepoName = currentName;

        // Only show "home" button when in Codespace and inside /worktrees directory
        const isInWorktree = currentPath.includes("/worktrees/");
        if (isCodespace && isInWorktree) {
          items.push(
            new CatnipItem(
              "home",
              "Return to main codespace workspace",
              vscode.TreeItemCollapsibleState.None,
              "catnip.openInterface",
            ),
          );
        }
      }

      // Fetch and add worktrees
      try {
        const worktrees = await getWorktrees();

        // Sort by last_accessed (most recent first)
        const sortedWorktrees = [...worktrees].sort((a, b) => {
          const aTime = new Date(a.last_accessed).getTime();
          const bTime = new Date(b.last_accessed).getTime();
          return bTime - aTime; // Descending order
        });

        // Group by repository
        const repoMap = new Map<string, WorktreeInfo[]>();
        for (const worktree of sortedWorktrees) {
          const repoName = worktree.name.split("/")[0];
          if (!repoMap.has(repoName)) {
            repoMap.set(repoName, []);
          }
          repoMap.get(repoName)!.push(worktree);
        }

        // If multiple repositories, create groups
        if (repoMap.size > 1) {
          for (const [repoName, repoWorktrees] of repoMap) {
            // Use current workspace name if this repo matches the current one
            const isCurrentRepo = currentWorkspaceRepoName === repoName;
            const displayName =
              isCurrentRepo && currentWorkspaceRepoName
                ? currentWorkspaceRepoName
                : repoName;
            // Expand the current workspace's repo group by default
            const collapsibleState = isCurrentRepo
              ? vscode.TreeItemCollapsibleState.Expanded
              : vscode.TreeItemCollapsibleState.Collapsed;
            const repoItem = new CatnipItem(
              displayName,
              `${repoWorktrees.length} workspace${repoWorktrees.length !== 1 ? "s" : ""}`,
              collapsibleState,
              undefined,
              undefined,
              repoWorktrees,
            );
            repoItem.iconPath = new vscode.ThemeIcon("folder");
            items.push(repoItem);
          }
        } else {
          // Single repository - show flat list
          for (const worktree of sortedWorktrees) {
            const title = getWorkspaceTitle(worktree);
            const tooltip = `${worktree.path}\nBranch: ${worktree.branch}`;
            const item = new CatnipItem(
              title,
              tooltip,
              vscode.TreeItemCollapsibleState.None,
              "catnip.openWorktree",
              worktree.path,
            );
            item.iconPath = getStatusIcon(worktree.claude_activity_state);
            items.push(item);
          }
        }
      } catch (error) {
        console.error("Failed to fetch worktrees:", error);
      }

      return items;
    }
    return [];
  }

  refresh(): void {
    this._onDidChangeTreeData.fire();
  }
}

class CatnipItem extends vscode.TreeItem {
  public command?: vscode.Command;

  constructor(
    public readonly label: string,
    public readonly tooltip: string,
    public readonly collapsibleState: vscode.TreeItemCollapsibleState,
    public readonly commandId?: string,
    public readonly path?: string,
    public readonly worktrees?: WorktreeInfo[],
  ) {
    super(label, collapsibleState);
    this.tooltip = tooltip;

    if (commandId) {
      this.command = {
        command: commandId,
        title: label,
        arguments: path ? [path] : [],
      };
    }
  }
}

export function activate(context: vscode.ExtensionContext) {
  const provider = new CatnipViewProvider();
  vscode.window.registerTreeDataProvider("catnip-sidebar", provider);

  // Check catnip status periodically (every 30 seconds) and refresh UI
  const healthCheckInterval = setInterval(async () => {
    provider.refresh(); // Refresh the tree view to show updated status
  }, 30000);

  // Clean up interval when extension deactivates
  context.subscriptions.push({
    dispose: () => clearInterval(healthCheckInterval),
  });

  const openInterfaceCommand = vscode.commands.registerCommand(
    "catnip.openInterface",
    async () => {
      const codespaceName = process.env.CODESPACE_NAME;

      if (codespaceName) {
        // In a Codespace environment - open in new window
        const url = `https://${codespaceName}-6369.app.github.dev`;
        vscode.env.openExternal(vscode.Uri.parse(url));
      } else {
        // Local development - try to open in webview panel
        const panel = vscode.window.createWebviewPanel(
          "catnipInterface",
          "Catnip Interface",
          vscode.ViewColumn.One,
          {
            enableScripts: true,
            retainContextWhenHidden: true,
          },
        );

        panel.webview.html = getWebviewContent();
      }
    },
  );

  const openLogsCommand = vscode.commands.registerCommand(
    "catnip.openLogs",
    async () => {
      try {
        const logPath = "/opt/catnip/catnip.log";
        const uri = vscode.Uri.file(logPath);

        // Check if log file exists
        try {
          await vscode.workspace.fs.stat(uri);
          // Open the log file
          const document = await vscode.workspace.openTextDocument(uri);
          await vscode.window.showTextDocument(document);
        } catch (_error) {
          // Log file doesn't exist
          vscode.window.showWarningMessage(
            `Catnip log file not found at ${logPath}. Catnip may not be running yet.`,
          );
        }
      } catch (error) {
        vscode.window.showErrorMessage(`Failed to open catnip logs: ${error}`);
      }
    },
  );

  const showQRCommand = vscode.commands.registerCommand(
    "catnip.showQR",
    async () => {
      const codespaceName = process.env.CODESPACE_NAME;

      if (!codespaceName) {
        vscode.window.showWarningMessage(
          "QR code is only available in GitHub Codespaces environment",
        );
        return;
      }

      const url = `https://catnip.run?cs=${codespaceName}`;

      try {
        // Generate QR code as data URL
        const qrDataUrl = await QRCode.toDataURL(url, {
          width: 300,
          margin: 2,
        });

        // Create and show webview panel with QR code
        const panel = vscode.window.createWebviewPanel(
          "catnipQR",
          "Catnip Mobile QR Code",
          vscode.ViewColumn.Beside,
          {
            enableScripts: false,
            retainContextWhenHidden: true,
          },
        );

        panel.webview.html = getQRWebviewContent(qrDataUrl, url);
      } catch (error) {
        vscode.window.showErrorMessage(`Failed to generate QR code: ${error}`);
      }
    },
  );

  const openWorktreeCommand = vscode.commands.registerCommand(
    "catnip.openWorktree",
    async (path: string) => {
      if (!path) {
        vscode.window.showErrorMessage("No path provided for workspace");
        return;
      }

      try {
        const uri = vscode.Uri.file(path);

        // Check if the directory exists locally (handles container vs host scenarios)
        try {
          await vscode.workspace.fs.stat(uri);
          // Directory exists locally, open it in a new window
          await vscode.commands.executeCommand("vscode.openFolder", uri, true);
        } catch (statError) {
          // Directory doesn't exist locally - likely in a container scenario
          // Open catnip interface in browser instead

          // Extract workspace name from path (everything after 'worktrees/')
          let workspaceName = path;
          const worktreesIndex = path.indexOf("/worktrees/");
          if (worktreesIndex !== -1) {
            workspaceName = path.substring(
              worktreesIndex + "/worktrees/".length,
            );
          } else {
            // Fallback: use the last part of the path
            workspaceName = path
              .split("/")
              .filter((p) => p)
              .slice(-2)
              .join("/");
          }

          const codespaceName = process.env.CODESPACE_NAME;
          let baseUrl: string;

          if (codespaceName) {
            // In a Codespace environment
            baseUrl = `https://${codespaceName}-6369.app.github.dev`;
          } else {
            // Local environment
            baseUrl = "http://localhost:6369";
          }

          const url = `${baseUrl}/workspace/${workspaceName}`;
          vscode.env.openExternal(vscode.Uri.parse(url));
          vscode.window.showInformationMessage(
            `Opening workspace ${workspaceName} in browser (directory not accessible from VS Code)`,
          );
        }
      } catch (error) {
        vscode.window.showErrorMessage(
          `Failed to open workspace at ${path}: ${error}`,
        );
      }
    },
  );

  context.subscriptions.push(
    openInterfaceCommand,
    openLogsCommand,
    showQRCommand,
    openWorktreeCommand,
  );
}

function getWebviewContent(): string {
  return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Catnip Interface</title>
    <style>
        body, html {
            margin: 0;
            padding: 0;
            width: 100%;
            height: 100%;
            overflow: hidden;
        }
        iframe {
            width: 100%;
            height: 100vh;
            border: none;
        }
        .error {
            padding: 20px;
            text-align: center;
            color: #666;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
        }
    </style>
</head>
<body>
    <div id="content">
        <div class="error">
            <h2>🐾 Catnip Development Interface</h2>
            <p>Loading interface from localhost:6369...</p>
            <p><small>If this doesn't load, make sure catnip is running on port 6369</small></p>
        </div>
    </div>
    
    <script>
        // Try to load the catnip interface
        const iframe = document.createElement('iframe');
        iframe.src = 'http://localhost:6369';
        iframe.onload = function() {
            document.getElementById('content').innerHTML = '';
            document.getElementById('content').appendChild(iframe);
        };
        iframe.onerror = function() {
            document.querySelector('.error p').textContent = 'Failed to connect to localhost:6369. Make sure catnip is running!';
        };
        
        // Add iframe after a short delay to show loading message
        setTimeout(() => {
            document.getElementById('content').appendChild(iframe);
        }, 1000);
    </script>
</body>
</html>`;
}

function getQRWebviewContent(qrDataUrl: string, url: string): string {
  return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Catnip Mobile QR Code</title>
    <style>
        body {
            margin: 0;
            padding: 20px;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
            background-color: var(--vscode-editor-background);
            color: var(--vscode-editor-foreground);
        }
        .container {
            text-align: center;
            max-width: 400px;
        }
        .qr-code {
            margin: 20px 0;
            padding: 20px;
            background-color: white;
            border-radius: 8px;
            display: inline-block;
        }
        .qr-code img {
            display: block;
            margin: 0 auto;
        }
        .url {
            margin-top: 20px;
            padding: 10px;
            background-color: var(--vscode-textBlockQuote-background);
            border-radius: 4px;
            word-break: break-all;
            font-family: monospace;
            font-size: 12px;
        }
        .instructions {
            margin-top: 20px;
            color: var(--vscode-descriptionForeground);
            line-height: 1.5;
        }
        h2 {
            color: var(--vscode-titleBar-activeForeground);
            margin-bottom: 10px;
        }
    </style>
</head>
<body>
    <div class="container">
        <h2>📱 Open Catnip on Mobile</h2>
        <p class="instructions">
            Scan this QR code with your mobile device to open Catnip:
        </p>
        <div class="qr-code">
            <img src="${qrDataUrl}" alt="QR Code for Catnip Mobile" />
        </div>
        <div class="url">${url}</div>
        <p class="instructions">
            <small>This will open Catnip in your mobile browser with your current Codespace session.</small>
        </p>
    </div>
</body>
</html>`;
}

export function deactivate() {}
