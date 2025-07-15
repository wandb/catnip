#!/usr/bin/env node
import { readFileSync, writeFileSync, existsSync } from 'fs';
import { join } from 'path';

// Get PEM file path from command line or use default
const pemFile = process.argv[2] || 'w-b-catnip.2025-07-12.private-key.pem';
const pemPath = join(process.cwd(), pemFile);
const envPath = join(process.cwd(), '.env.local');

if (!existsSync(pemPath)) {
  console.error(`❌ PEM file not found: ${pemPath}`);
  console.log('\nUsage: tsx worker/scripts/import-pem.ts [path-to-pem-file]');
  process.exit(1);
}

// Read PEM file
const pemContent = readFileSync(pemPath, 'utf-8').trim();

// Convert to single line format (replace newlines with \n)
const singleLinePem = pemContent.replace(/\n/g, '\\n');

// Read existing .env.local or create new content
let envContent = '';
if (existsSync(envPath)) {
  envContent = readFileSync(envPath, 'utf-8');
}

// Check if GITHUB_APP_PRIVATE_KEY already exists
if (envContent.includes('GITHUB_APP_PRIVATE_KEY=')) {
  // Update existing key
  envContent = envContent.replace(
    /GITHUB_APP_PRIVATE_KEY=.*/,
    `GITHUB_APP_PRIVATE_KEY="${singleLinePem}"`
  );
  console.log('✅ Updated GITHUB_APP_PRIVATE_KEY in .env.local');
} else {
  // Add new key
  if (envContent && !envContent.endsWith('\n')) {
    envContent += '\n';
  }
  envContent += `GITHUB_APP_PRIVATE_KEY="${singleLinePem}"\n`;
  console.log('✅ Added GITHUB_APP_PRIVATE_KEY to .env.local');
}

// Write back to file
writeFileSync(envPath, envContent);
console.log(`\n📁 Imported from: ${pemFile}`);
console.log('🔐 PEM file has been converted to single-line format and added to .env.local');
console.log('\nNote: The private key is now stored with \\n characters instead of actual newlines.');
console.log('CloudFlare and most services will automatically convert these back to newlines.');