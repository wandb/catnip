import * as vscode from "vscode";

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

  getChildren(element?: CatnipItem): Promise<CatnipItem[]> {
    if (!element) {
      return Promise.resolve([
        new CatnipItem(
          "💻 Open Catnip Interface",
          "Click to open the catnip development environment",
          vscode.TreeItemCollapsibleState.None,
          "catnip.openInterface",
        ),
      ]);
    }
    return Promise.resolve([]);
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

  const openInterfaceCommand = vscode.commands.registerCommand(
    "catnip.openInterface",
    () => {
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

  context.subscriptions.push(openInterfaceCommand);
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

export function deactivate() {}
