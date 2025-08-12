#!/usr/bin/env node

import { EventSource } from "eventsource";

const eventSource = new EventSource("http://localhost:3003/v1/events");
let eventCount = 0;
const maxEvents = 10;

console.log("Connecting to SSE endpoint...\n");

eventSource.onmessage = (event) => {
  eventCount++;
  try {
    const data = JSON.parse(event.data);
    console.log(`Event ${eventCount}: ${data.event?.type || "unknown"}`);

    if (data.event?.type === "worktree:created") {
      console.log(
        `  - Worktree: ${data.event.payload.worktree.name} (${data.event.payload.worktree.id})`,
      );
    }

    if (eventCount >= maxEvents) {
      console.log(`\nReceived ${maxEvents} events, closing connection.`);
      eventSource.close();
      process.exit(0);
    }
  } catch (error) {
    console.error("Failed to parse event:", error);
  }
};

eventSource.onerror = (error) => {
  console.error("SSE Error:", error);
  eventSource.close();
  process.exit(1);
};

// Timeout after 5 seconds
setTimeout(() => {
  console.log(`\nTimeout: Received ${eventCount} events total`);
  eventSource.close();
  process.exit(0);
}, 5000);
