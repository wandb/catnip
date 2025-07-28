#!/usr/bin/env node
/**
 * Test script for the Node.js terminal title interceptor
 * This simulates Claude Code writing terminal titles directly to stdout
 */

console.log("Testing Node.js terminal title interceptor...");
console.log("Process args:", process.argv);

// Simulate being a Claude process by modifying argv
process.argv.push("claude");

// Require the interceptor after setting up argv
require("/opt/catnip/lib/pty-title-interceptor.js");

console.log("Interceptor loaded, testing title sequences...");

// Test title sequences by writing directly to stdout (like Claude does)
setTimeout(() => {
  console.log("\n--- Testing title sequence 1 ---");
  process.stdout.write("\x1b]0;Test Title 1\x07");
}, 1000);

setTimeout(() => {
  console.log("\n--- Testing title sequence 2 ---");
  process.stdout.write("\x1b]0;Claude Code: Implementing feature\x07");
}, 2000);

setTimeout(() => {
  console.log("\n--- Testing title sequence 3 ---");
  process.stdout.write("\x1b]0;Working on bug fix for login system\x07");
}, 3000);

// Check results after 4 seconds
setTimeout(() => {
  console.log("\n--- Test complete, checking results ---");

  // Check if log file was created
  const fs = require("fs");
  const logPath =
    process.env.CATNIP_TITLE_LOG || "/home/catnip/.catnip/title_events.log";

  if (fs.existsSync(logPath)) {
    console.log("\n✅ Log file created successfully!");
    console.log("Contents:");
    const content = fs.readFileSync(logPath, "utf8");
    console.log(content);
  } else {
    console.log("\n❌ Log file was not created");
  }

  process.exit(0);
}, 4000);
