#!/usr/bin/env node
import { generateEncryptionKey } from '../lib/crypto';
import { readFileSync, writeFileSync, existsSync } from 'fs';
import { join } from 'path';

const envPath = join(process.cwd(), '.env.local');
const key = generateEncryptionKey();

// Read existing .env.local or create new content
let envContent = '';
if (existsSync(envPath)) {
  envContent = readFileSync(envPath, 'utf-8');
}

// Check if CATNIP_ENCRYPTION_KEY already exists
if (envContent.includes('CATNIP_ENCRYPTION_KEY=')) {
  // Update existing key
  envContent = envContent.replace(
    /CATNIP_ENCRYPTION_KEY=.*/,
    `CATNIP_ENCRYPTION_KEY=${key}`
  );
  console.log('✅ Updated CATNIP_ENCRYPTION_KEY in .env.local');
} else {
  // Add new key
  if (envContent && !envContent.endsWith('\n')) {
    envContent += '\n';
  }
  envContent += `CATNIP_ENCRYPTION_KEY=${key}\n`;
  console.log('✅ Added CATNIP_ENCRYPTION_KEY to .env.local');
}

// Write back to file
writeFileSync(envPath, envContent);
console.log(`\nGenerated key: ${key}`);