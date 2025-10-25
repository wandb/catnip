import * as vscode from "vscode";
import { exec } from "child_process";
import { promisify } from "util";
import * as fs from "fs";
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

// Catnip management functions
async function isCatnipRunning(): Promise<boolean> {
  try {
    const pidFile = "/opt/catnip/catnip.pid";
    if (!fs.existsSync(pidFile)) {
      return false;
    }

    const pidStr = fs.readFileSync(pidFile, "utf8").trim();
    const pid = parseInt(pidStr);

    if (isNaN(pid)) {
      return false;
    }

    // Check if process is actually running
    const { stdout } = await execAsync(
      `kill -0 ${pid} 2>/dev/null && echo "running" || echo "not running"`,
    );
    return stdout.trim() === "running";
  } catch (_error) {
    return false;
  }
}

async function startCatnip(): Promise<void> {
  try {
    console.log("🐾 Starting catnip...");
    await execAsync("bash /opt/catnip/bin/catnip-start.sh");
    console.log("✅ Catnip started successfully");
  } catch (error) {
    console.error("❌ Failed to start catnip:", error);
    vscode.window.showErrorMessage(`Failed to start catnip: ${error}`);
  }
}

async function ensureCatnipRunning(): Promise<void> {
  const running = await isCatnipRunning();
  if (!running) {
    console.log("🐾 Catnip not running, starting...");
    await startCatnip();
  } else {
    console.log("✅ Catnip already running");
  }
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
    if (!element) {
      const running = await isCatnipRunning();

      if (running) {
        const isMobile = isMobileDevice();

        if (isMobile) {
          // Mobile view: show cat logo
          return [
            new CatnipItem(
              "🐱",
              "Open Catnip Interface",
              vscode.TreeItemCollapsibleState.None,
              "catnip.openInterface",
            ),
          ];
        } else {
          // Desktop view: show button + QR code
          return [
            new CatnipItem(
              "💻 Open Catnip Interface",
              "Click to open the catnip development environment",
              vscode.TreeItemCollapsibleState.None,
              "catnip.openInterface",
            ),
            new CatnipItem(
              "📱 Open on Mobile",
              "Scan QR code to open on mobile device",
              vscode.TreeItemCollapsibleState.None,
              "catnip.showQR",
            ),
          ];
        }
      } else {
        return [
          new CatnipItem(
            "❌ Catnip Not Running",
            "Click to view catnip logs and troubleshoot",
            vscode.TreeItemCollapsibleState.None,
            "catnip.openLogs",
          ),
        ];
      }
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
  ) {
    super(label, collapsibleState);
    this.tooltip = tooltip;

    if (commandId) {
      this.command = {
        command: commandId,
        title: label,
        arguments: [],
      };
    }
  }
}

export function activate(context: vscode.ExtensionContext) {
  const provider = new CatnipViewProvider();
  vscode.window.registerTreeDataProvider("catnip-sidebar", provider);

  // Ensure catnip is running when extension activates
  ensureCatnipRunning().catch((error) => {
    console.error("Failed to ensure catnip is running:", error);
  });

  // Check catnip status periodically (every 30 seconds) and refresh UI
  const healthCheckInterval = setInterval(async () => {
    try {
      await ensureCatnipRunning();
      provider.refresh(); // Refresh the tree view to show updated status
    } catch (error) {
      console.error("Health check failed:", error);
      provider.refresh(); // Still refresh to show the error state
    }
  }, 30000);

  // Clean up interval when extension deactivates
  context.subscriptions.push({
    dispose: () => clearInterval(healthCheckInterval),
  });

  const openInterfaceCommand = vscode.commands.registerCommand(
    "catnip.openInterface",
    async () => {
      // Ensure catnip is running before opening interface
      await ensureCatnipRunning();

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
          // Log file doesn't exist, show error and try to start catnip
          vscode.window.showWarningMessage(
            `Catnip log file not found at ${logPath}. Attempting to start catnip...`,
          );
          await ensureCatnipRunning();
          provider.refresh();
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

  context.subscriptions.push(
    openInterfaceCommand,
    openLogsCommand,
    showQRCommand,
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
