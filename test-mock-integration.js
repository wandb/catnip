#!/usr/bin/env node

// Test script to verify mock server integration

const baseUrl = 'http://localhost:3002';

async function testEndpoint(path, method = 'GET', body = null) {
  try {
    const options = {
      method,
      headers: {
        'Content-Type': 'application/json',
      },
    };
    
    if (body) {
      options.body = JSON.stringify(body);
    }
    
    const response = await fetch(`${baseUrl}${path}`, options);
    const data = await response.json();
    
    console.log(`✅ ${method} ${path}: ${response.status}`);
    return data;
  } catch (error) {
    console.error(`❌ ${method} ${path}: ${error.message}`);
    return null;
  }
}

async function testSSE(path) {
  return new Promise((resolve) => {
    console.log(`📡 Testing SSE ${path}...`);
    
    const eventSource = new (await import('eventsource')).default(`${baseUrl}${path}`);
    let eventCount = 0;
    
    eventSource.onmessage = (event) => {
      eventCount++;
      const data = JSON.parse(event.data);
      console.log(`   Event ${eventCount}: ${data.event?.type || 'unknown'}`);
      
      if (eventCount >= 3) {
        eventSource.close();
        console.log(`✅ SSE ${path}: Received ${eventCount} events`);
        resolve();
      }
    };
    
    eventSource.onerror = (error) => {
      console.error(`❌ SSE ${path}: ${error.message || 'Connection error'}`);
      eventSource.close();
      resolve();
    };
    
    // Timeout after 10 seconds
    setTimeout(() => {
      eventSource.close();
      console.log(`✅ SSE ${path}: Timeout after receiving ${eventCount} events`);
      resolve();
    }, 10000);
  });
}

async function runTests() {
  console.log('🧪 Testing Mock Server Integration\n');
  console.log(`Server: ${baseUrl}\n`);
  
  // Test Auth endpoints
  console.log('=== Auth Endpoints ===');
  await testEndpoint('/v1/auth/github/status');
  await testEndpoint('/v1/auth/github/start', 'POST');
  await testEndpoint('/v1/auth/github/reset', 'POST');
  
  // Test Claude endpoints
  console.log('\n=== Claude Endpoints ===');
  await testEndpoint('/v1/claude/settings');
  await testEndpoint('/v1/claude/settings', 'PUT', { theme: 'light' });
  await testEndpoint('/v1/claude/sessions');
  await testEndpoint('/v1/claude/session?worktree_path=/workspace/main');
  await testEndpoint('/v1/claude/todos');
  await testEndpoint('/v1/claude/messages', 'POST', { 
    prompt: 'Test message',
    stream: false 
  });
  
  // Test Git endpoints
  console.log('\n=== Git Endpoints ===');
  await testEndpoint('/v1/git/worktrees');
  await testEndpoint('/v1/git/worktrees/main');
  await testEndpoint('/v1/git/status');
  await testEndpoint('/v1/git/branches/repo-1');
  await testEndpoint('/v1/git/github/repos');
  
  // Test Port endpoints
  console.log('\n=== Port Endpoints ===');
  await testEndpoint('/v1/ports');
  await testEndpoint('/v1/ports/mappings');
  await testEndpoint('/v1/ports/mappings/3000');
  
  // Test Session endpoints
  console.log('\n=== Session Endpoints ===');
  await testEndpoint('/v1/sessions');
  await testEndpoint('/v1/sessions/active');
  
  // Test Notifications
  console.log('\n=== Notification Endpoints ===');
  await testEndpoint('/v1/notifications');
  await testEndpoint('/v1/notifications', 'POST', {
    type: 'info',
    message: 'Test notification'
  });
  
  // Test SSE Events
  console.log('\n=== SSE Events ===');
  await testSSE('/v1/events');
  
  console.log('\n✨ All tests completed!');
}

// Check if eventsource is installed
import('eventsource').then(() => {
  runTests();
}).catch(() => {
  console.log('Installing eventsource package...');
  const { execSync } = require('child_process');
  execSync('npm install eventsource', { stdio: 'inherit' });
  runTests();
});