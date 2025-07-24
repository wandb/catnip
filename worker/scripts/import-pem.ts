#!/usr/bin/env node
import { readFileSync, writeFileSync, existsSync, unlinkSync } from 'fs';
import { join } from 'path';
import { execSync } from 'child_process';

// Get PEM file path from command line or use default
const pemFile = process.argv[2] || 'w-b-catnip.2025-07-12.private-key.pem';
const pemPath = join(process.cwd(), pemFile);
const envPath = join(process.cwd(), '.dev.vars');

if (!existsSync(pemPath)) {
  console.error(`❌ PEM file not found: ${pemPath}`);
  console.log('\nUsage: tsx worker/scripts/import-pem.ts [path-to-pem-file]');
  process.exit(1);
}

// Read PEM file
let pemContent = readFileSync(pemPath, 'utf-8').trim();

// Check if it's PKCS#1 format and convert to PKCS#8 if needed
if (pemContent.includes('-----BEGIN RSA PRIVATE KEY-----')) {
  console.log('🔄 Detected PKCS#1 format, converting to PKCS#8...');
  
  // Create temporary output file
  const tempOutputPath = pemPath.replace('.pem', '.pkcs8.pem');
  
  try {
    // Convert using openssl
    execSync(`openssl pkcs8 -topk8 -inform PEM -outform PEM -nocrypt -in "${pemPath}" -out "${tempOutputPath}"`);
    
    // Read the converted content
    pemContent = readFileSync(tempOutputPath, 'utf-8').trim();
    
    // Clean up temporary file
    unlinkSync(tempOutputPath);
    
    console.log('✅ Successfully converted to PKCS#8 format');
  } catch (error) {
    console.error('❌ Failed to convert private key format:', error);
    process.exit(1);
  }
} else if (pemContent.includes('-----BEGIN PRIVATE KEY-----')) {
  console.log('✅ Already in PKCS#8 format');
} else {
  console.error('❌ Unrecognized private key format');
  process.exit(1);
}

// Convert to single line format (replace newlines with \n)
const singleLinePem = pemContent.replace(/\n/g, '\\n');

// Read existing .dev.vars or create new content
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
  console.log('✅ Updated GITHUB_APP_PRIVATE_KEY in .dev.vars');
} else {
  // Add new key
  if (envContent && !envContent.endsWith('\n')) {
    envContent += '\n';
  }
  envContent += `GITHUB_APP_PRIVATE_KEY="${singleLinePem}"\n`;
  console.log('✅ Added GITHUB_APP_PRIVATE_KEY to .dev.vars');
}

// Write back to file
writeFileSync(envPath, envContent);
console.log(`\n📁 Imported from: ${pemFile}`);
console.log('🔐 PEM file has been converted to single-line format and added to .dev.vars');
console.log('\nNote: The private key is now stored with \\n characters instead of actual newlines.');
console.log('CloudFlare and most services will automatically convert these back to newlines.');