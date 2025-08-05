#!/usr/bin/env node
/**
 * Node.js Terminal Title Interceptor for Claude Code Detection
 *
 * This version intercepts process.stdout.write to detect title sequences
 * since Claude Code doesn't use node-pty, but writes titles directly to stdout.
 */

const fs = require("fs");
const path = require("path");

// Early exit if interceptor is disabled
if (
  process.env.CATNIP_DISABLE_PTY_INTERCEPTOR === "1" ||
  process.env.CATNIP_DISABLE_PTY_INTERCEPTOR === "true"
) {
  return;
}

const TITLE_LOG_PATH =
  process.env.CATNIP_TITLE_LOG ||
  require("os").homedir() + "/.catnip/title_events.log";
const TITLE_START_SEQ = "\x1b]0;";
const TITLE_END_CHAR = "\x07";

let logFileReady = false;

function ensureLogFile() {
  if (logFileReady) return true;

  try {
    const tmpDir = path.dirname(TITLE_LOG_PATH);
    if (fs.existsSync(tmpDir) && fs.statSync(tmpDir).isDirectory()) {
      if (!fs.existsSync(TITLE_LOG_PATH)) {
        fs.writeFileSync(TITLE_LOG_PATH, "");
      }
      logFileReady = true;
      return true;
    }
  } catch (err) {
    // Silent failure
  }
  return false;
}

function logTitleChange(cwd, title) {
  if (!ensureLogFile()) return;

  try {
    const timestamp = new Date().toISOString();
    const pid = process.pid;
    const logEntry = `${timestamp}|${pid}|${cwd}|${title}\n`;
    fs.appendFileSync(TITLE_LOG_PATH, logEntry);
  } catch (err) {
    // Silent failure
  }
}

function extractTitleFromData(data) {
  if (!Buffer.isBuffer(data)) {
    try {
      data = Buffer.from(data);
    } catch (err) {
      return null;
    }
  }

  const dataStr = data.toString();
  const startIndex = dataStr.indexOf(TITLE_START_SEQ);

  if (startIndex === -1) return null;

  const titleStart = startIndex + TITLE_START_SEQ.length;
  const endIndex = dataStr.indexOf(TITLE_END_CHAR, titleStart);

  if (endIndex === -1) return null;

  const title = dataStr.substring(titleStart, endIndex).trim();
  return title.length > 0 ? title : null;
}

// Only intercept stdout/stderr writes for processes that look like Claude
const isClaudeProcess = process.argv.some(
  (arg) => arg.includes("claude") || arg.includes("@anthropic-ai/claude-code"),
);

if (isClaudeProcess) {
  // Intercept stdout.write
  if (process.stdout && typeof process.stdout.write === "function") {
    const originalStdoutWrite = process.stdout.write.bind(process.stdout);

    process.stdout.write = function (chunk, encoding, callback) {
      try {
        const title = extractTitleFromData(chunk);
        if (title) {
          logTitleChange(process.cwd(), title);
        }
      } catch (err) {
        // Silent failure
      }

      return originalStdoutWrite(chunk, encoding, callback);
    };
  }

  // Intercept stderr.write
  if (process.stderr && typeof process.stderr.write === "function") {
    const originalStderrWrite = process.stderr.write.bind(process.stderr);

    process.stderr.write = function (chunk, encoding, callback) {
      try {
        const title = extractTitleFromData(chunk);
        if (title) {
          logTitleChange(process.cwd(), title);
        }
      } catch (err) {
        // Silent failure
      }

      return originalStderrWrite(chunk, encoding, callback);
    };
  }
}

// Silent cleanup on exit
process.on("exit", () => {
  // Silent cleanup
});

process.on("SIGINT", () => {
  process.exit(0);
});

process.on("SIGTERM", () => {
  process.exit(0);
});
