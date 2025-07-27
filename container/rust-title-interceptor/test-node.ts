#!/usr/bin/env tsx

// TypeScript test for title interceptor

import * as fs from "fs";
import { promisify } from "util";

const sleep = promisify(setTimeout);
const TITLE_LOG_FILE = "/tmp/catnip_syscall_titles.log";

// ANSI color codes
const colors = {
  red: "\x1b[31m",
  green: "\x1b[32m",
  yellow: "\x1b[33m",
  reset: "\x1b[0m",
} as const;

interface LogEntry {
  timestamp: string;
  pid: string;
  cwd: string;
  title: string;
}

function printResult(success: boolean, message: string): void {
  const symbol = success ? "✓" : "✗";
  const color = success ? colors.green : colors.red;
  console.log(`${color}${symbol} ${message}${colors.reset}`);
  if (!success) {
    process.exit(1);
  }
}

function parseLogFile(): LogEntry[] {
  try {
    const content = fs.readFileSync(TITLE_LOG_FILE, "utf8");
    return content
      .trim()
      .split("\n")
      .map((line) => {
        const [timestamp, pid, cwd, title] = line.split("|");
        return { timestamp, pid, cwd, title };
      });
  } catch {
    return [];
  }
}

async function testTitleCapture(
  testName: string,
  action: () => void,
  expectedTitle: string,
): Promise<void> {
  const beforeCount = parseLogFile().length;
  action();
  await sleep(100);

  const entries = parseLogFile();
  const found = entries
    .slice(beforeCount)
    .some((entry) => entry.title === expectedTitle);

  printResult(found, testName);
}

async function runTests(): Promise<void> {
  console.log("=== TypeScript Title Interceptor Test ===\n");

  // Clean up old log file
  console.log("1. Cleaning up old log file...");
  try {
    fs.unlinkSync(TITLE_LOG_FILE);
  } catch {
    // File might not exist
  }
  printResult(true, "Cleaned up old log file");
  console.log();

  // Test 1: Basic title with process.stdout.write
  console.log("2. Testing basic title with process.stdout.write...");
  await testTitleCapture(
    "Basic stdout.write test",
    () => process.stdout.write("\x1b]0;TypeScript Test Title\x07"),
    "TypeScript Test Title",
  );
  console.log();

  // Test 2: Console.log
  console.log("3. Testing with console.log...");
  await testTitleCapture(
    "Console.log test",
    () => console.log("\x1b]0;TS Console Title\x07"),
    "TS Console Title",
  );
  console.log();

  // Test 3: Process.stderr.write
  console.log("4. Testing stderr output...");
  await testTitleCapture(
    "Stderr test",
    () => process.stderr.write("\x1b]0;TS Stderr Title\x07"),
    "TS Stderr Title",
  );
  console.log();

  // Test 4: Simulate terminal UI frameworks
  console.log("5. Testing terminal UI framework patterns...");

  // Simulate what Ink, Blessed, or other TUI frameworks might do
  const terminalUITest = async () => {
    // Clear screen and set title (common pattern)
    process.stdout.write("\x1b[2J\x1b[H"); // Clear screen
    process.stdout.write("\x1b]0;📦 Package Manager UI\x07");
    await sleep(50);

    // Update title with progress
    process.stdout.write("\x1b]0;📦 Installing dependencies... 45%\x07");
    await sleep(50);

    // Final status
    process.stdout.write("\x1b]0;✅ Installation complete!\x07");
  };

  await terminalUITest();
  await sleep(100);

  const entries = parseLogFile();
  const uiTitles = [
    "📦 Package Manager UI",
    "📦 Installing dependencies... 45%",
    "✅ Installation complete!",
  ];
  const allUITitlesFound = uiTitles.every((title) =>
    entries.some((entry) => entry.title === title),
  );

  printResult(allUITitlesFound, "Terminal UI framework patterns test");
  console.log();

  // Test 6: Test with Buffer writes (lower level)
  console.log("6. Testing with Buffer writes...");
  const titleBuffer = Buffer.from("\x1b]0;Buffer Write Title\x07");
  process.stdout.write(titleBuffer);
  await sleep(100);

  const bufferFound = parseLogFile().some(
    (entry) => entry.title === "Buffer Write Title",
  );
  printResult(bufferFound, "Buffer write test");
  console.log();

  // Show final log contents
  console.log(`${colors.yellow}=== Final log contents ===${colors.reset}`);
  const finalEntries = parseLogFile();

  if (finalEntries.length > 0) {
    console.log(`Total entries captured: ${finalEntries.length}`);
    console.log("\nLast 10 entries:");
    finalEntries.slice(-10).forEach((entry) => {
      console.log(
        `  Time: ${entry.timestamp}, PID: ${entry.pid}, Title: ${entry.title}`,
      );
    });
  } else {
    console.log(`${colors.red}No log entries found!${colors.reset}`);
  }
  console.log();

  console.log(`${colors.green}=== All tests passed! ===${colors.reset}`);
}

// Check environment
if (process.env.CATNIP_TITLE_INTERCEPT !== "1") {
  console.error("Error: CATNIP_TITLE_INTERCEPT must be set to 1");
  console.error(
    "Run with: CATNIP_TITLE_INTERCEPT=1 LD_PRELOAD=./libtitle_interceptor.so tsx test-node.ts",
  );
  process.exit(1);
}

// Run tests
runTests().catch((err) => {
  console.error("Test failed:", err);
  process.exit(1);
});
