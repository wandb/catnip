import { Terminal, type IDisposable } from "@xterm/xterm";

interface UploadResponse {
  success: boolean;
  filePath: string;
  message?: string;
}

/**
 * File Drop Addon for xterm.js
 * Enables drag and drop file functionality, uploading files to container
 * and typing the file path into the terminal like native terminals
 */
export class FileDropAddon implements IDisposable {
  private _disposables: IDisposable[] = [];
  private _terminal: Terminal | undefined;
  private _sendData?: (data: string) => void;

  constructor(sendData?: (data: string) => void) {
    this._sendData = sendData;
  }

  activate(terminal: Terminal): void {
    this._terminal = terminal;

    // Setup event handlers immediately if element exists, otherwise wait
    this._setupEventHandlers();

    // If element doesn't exist yet, check periodically until it does
    if (!terminal.element) {
      const checkElement = () => {
        if (terminal.element) {
          this._setupEventHandlers();
        } else {
          // Check again in next tick
          setTimeout(checkElement, 0);
        }
      };
      checkElement();
    }
  }

  private _setupEventHandlers(): void {
    if (!this._terminal?.element) {
      return;
    }

    const element = this._terminal.element;

    // Check if already setup to avoid duplicate listeners
    if ((element as any)._fileDropSetup) {
      return;
    }

    // Prevent default drag behaviors
    const onDragOver = this._onDragOver.bind(this);
    const onDragEnter = this._onDragEnter.bind(this);
    const onDragLeave = this._onDragLeave.bind(this);
    const onDrop = this._onDrop.bind(this);

    element.addEventListener("dragover", onDragOver);
    element.addEventListener("dragenter", onDragEnter);
    element.addEventListener("dragleave", onDragLeave);
    element.addEventListener("drop", onDrop);

    // Mark as setup and store event listeners for cleanup
    (element as any)._fileDropSetup = true;
    this._disposables.push({
      dispose: () => {
        element.removeEventListener("dragover", onDragOver);
        element.removeEventListener("dragenter", onDragEnter);
        element.removeEventListener("dragleave", onDragLeave);
        element.removeEventListener("drop", onDrop);
        (element as any)._fileDropSetup = false;
      },
    });

    console.log("📎 File drop addon event handlers setup");
  }

  private _onDragEnter(e: DragEvent): void {
    e.preventDefault();
    e.stopPropagation();

    // Add visual feedback
    if (this._terminal?.element) {
      this._terminal.element.style.backgroundColor = "rgba(0, 255, 149, 0.1)";
      this._terminal.element.style.border = "2px dashed #00ff95";
    }
  }

  private _onDragOver(e: DragEvent): void {
    e.preventDefault();
    e.stopPropagation();
  }

  private _onDragLeave(e: DragEvent): void {
    e.preventDefault();
    e.stopPropagation();

    // Remove visual feedback only if leaving the terminal element
    if (
      this._terminal?.element &&
      !this._terminal.element.contains(e.relatedTarget as Node)
    ) {
      this._terminal.element.style.backgroundColor = "";
      this._terminal.element.style.border = "";
    }
  }

  private _onDrop(e: DragEvent): void {
    e.preventDefault();
    e.stopPropagation();

    // Remove visual feedback
    if (this._terminal?.element) {
      this._terminal.element.style.backgroundColor = "";
      this._terminal.element.style.border = "";
    }

    const files = e.dataTransfer?.files;
    if (files && files.length > 0) {
      void this._handleFiles(files);
    }
  }

  private async _handleFiles(files: FileList): Promise<void> {
    if (!this._terminal) {
      console.error("Terminal not available for file drop");
      return;
    }

    for (let i = 0; i < files.length; i++) {
      const file = files[i];

      try {
        // Show upload indicator
        this._terminal.write(`\r\n📤 Uploading ${file.name}...`);

        // Upload file to container
        const remotePath = await this._uploadToContainer(file);

        // Clear the upload indicator line
        this._terminal.write("\r\x1b[K");

        // Send the path as input using bracketed paste (like native terminals)
        this._sendPathAsInput(remotePath);

        console.log(`✅ File uploaded: ${file.name} -> ${remotePath}`);
      } catch (error) {
        // Clear the upload indicator line and show error
        this._terminal.write("\r\x1b[K");
        this._terminal.write(
          `\r\n❌ Failed to upload ${file.name}: ${error}\r\n`,
        );
        console.error("File upload failed:", error);
      }
    }
  }

  private _sendPathAsInput(rawPath: string): void {
    if (!this._sendData) {
      console.warn(
        "No sendData callback provided, cannot send file path as input",
      );
      return;
    }

    // Escape path like native terminals (proper bash quoting)
    const quotedPath = this._quotePath(rawPath);

    // Send bracketed paste sequences over WebSocket
    const BRK_BEGIN = "\x1b[200~";
    const BRK_END = "\x1b[201~";
    const pasteData = BRK_BEGIN + quotedPath + BRK_END;

    // Send via the provided callback (goes to WebSocket)
    this._sendData(pasteData);
  }

  private _quotePath(path: string): string {
    // Use proper bash quoting: wrap in single quotes and escape any single quotes within
    return `'${path.replace(/'/g, `'\\''`)}'`;
  }

  private async _uploadToContainer(file: File): Promise<string> {
    const formData = new FormData();
    formData.append("file", file);

    const response = await fetch("/v1/upload", {
      method: "POST",
      body: formData,
    });

    if (!response.ok) {
      throw new Error(
        `Upload failed: ${response.status} ${response.statusText}`,
      );
    }

    const result: UploadResponse = await response.json();

    if (!result.success) {
      throw new Error(result.message || "Upload failed");
    }

    return result.filePath;
  }

  dispose(): void {
    this._disposables.forEach((d) => d.dispose());
    this._disposables.length = 0;
    this._terminal = undefined;
  }
}
