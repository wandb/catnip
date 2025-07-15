#!/usr/bin/env node

import { execSync } from 'child_process';
import { readFileSync, existsSync, writeFileSync, unlinkSync } from 'fs';
import path from 'path';

// Types
interface ParsedSecret {
  key: string;
  value: string;
}

// Colors for output
const colors = {
  red: '\x1b[31m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  reset: '\x1b[0m'
};

function log(message: string, color?: keyof typeof colors): void {
  const colorCode = color ? colors[color] : '';
  const resetCode = color ? colors.reset : '';
  console.log(`${colorCode}${message}${resetCode}`);
}

function run(command: string, silent = false): string {
  try {
    const result = execSync(command, { 
      encoding: 'utf8',
      stdio: silent ? 'pipe' : 'inherit'
    });
    return result ? result.trim() : '';
  } catch (error: any) {
    if (silent) {
      return '';
    }
    throw new Error(`Command failed: ${command}\n${error.message}`);
  }
}

function parseArgs(): { env?: string; force: boolean; help: boolean } {
  const args = process.argv.slice(2);
  let env: string | undefined;
  let force = false;
  let help = false;

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    
    if (arg === '--env' && i + 1 < args.length) {
      env = args[i + 1];
      i++; // Skip next arg since we consumed it
    } else if (arg === '--force') {
      force = true;
    } else if (arg === '--help' || arg === '-h') {
      help = true;
    } else {
      log(`Unknown option: ${arg}`, 'red');
      log('Usage: npx tsx worker/scripts/upload-secrets.ts [--env <environment>] [--force] [--help]');
      process.exit(1);
    }
  }

  return { env, force, help };
}

function showHelp(): void {
  console.log(`
🔐 Catnip Secrets Upload Script

USAGE:
    npx tsx worker/scripts/upload-secrets.ts --env <environment> [OPTIONS]

OPTIONS:
    --env ENV        Environment (qa, production) [REQUIRED]
    --force          Overwrite existing secrets without error
    --help, -h       Show this help

EXAMPLES:
    npx tsx worker/scripts/upload-secrets.ts --env qa          # Upload to QA
    npx tsx worker/scripts/upload-secrets.ts --env production  # Upload to production
    npx tsx worker/scripts/upload-secrets.ts --env qa --force  # Force overwrite in QA

FEATURES:
    - Requires explicit environment selection (no default)
    - Automatically skips public variables (GITHUB_APP_ID, GITHUB_CLIENT_ID)
    - Checks for existing secrets and errors if they exist (use --force to overwrite)
    - Shows preview of secret values (first 4 characters)
    - Handles multi-line values (like private keys)
`);
}

function parseDevVars(): ParsedSecret[] {
  const devVarsPath = path.join(process.cwd(), '.dev.vars');
  
  if (!existsSync(devVarsPath)) {
    log('Error: .dev.vars file not found', 'red');
    process.exit(1);
  }

  const content = readFileSync(devVarsPath, 'utf8');
  const lines = content.split('\n');
  const secrets: ParsedSecret[] = [];
  
  let currentKey = '';
  let currentValue = '';
  let inMultiline = false;

  for (const line of lines) {
    // Skip empty lines and comments
    if (!line.trim() || line.trim().startsWith('#')) {
      continue;
    }

    // Check if this is a key=value line
    const keyValueMatch = line.match(/^([A-Z_]+)=(.*)$/);
    
    if (keyValueMatch) {
      // If we were collecting a multiline value, save it first
      if (currentKey) {
        secrets.push({ key: currentKey, value: currentValue });
      }

      currentKey = keyValueMatch[1];
      let value = keyValueMatch[2];

      // Skip non-secret values that are in wrangler.jsonc
      if (currentKey === 'GITHUB_APP_ID' || currentKey === 'GITHUB_CLIENT_ID') {
        log(`Skipping ${currentKey} (configured in wrangler.jsonc)`, 'blue');
        currentKey = '';
        currentValue = '';
        continue;
      }

      // Remove surrounding quotes if present
      value = value.replace(/^"(.*)"$/, '$1');

      // Check if this starts a multiline value (like a private key)
      if (value.includes('-----BEGIN')) {
        inMultiline = true;
        currentValue = value;
      } else {
        // Single line value
        currentValue = value;
        secrets.push({ key: currentKey, value: currentValue });
        currentKey = '';
        currentValue = '';
      }
    } else if (inMultiline && currentKey) {
      // Continue collecting multiline value
      let lineValue = line;
      
      // Check if this ends the multiline value
      if (line.endsWith('"')) {
        lineValue = line.slice(0, -1); // Remove trailing quote
        inMultiline = false;
      }

      // Append line with proper newline
      currentValue = currentValue + '\n' + lineValue;

      // If we just ended the multiline value, save it
      if (!inMultiline) {
        secrets.push({ key: currentKey, value: currentValue });
        currentKey = '';
        currentValue = '';
      }
    }
  }

  // Save any remaining secret
  if (currentKey) {
    secrets.push({ key: currentKey, value: currentValue });
  }

  return secrets;
}

function secretExists(key: string, env?: string): boolean {
  try {
    const envFlag = env ? `--env ${env}` : '';
    const output = run(`wrangler secret list ${envFlag}`, true);
    return output.includes(`"${key}"`);
  } catch {
    return false;
  }
}

function uploadSecret(secret: ParsedSecret, env?: string, force: boolean): void {
  const { key, value } = secret;

  log(`Processing secret: ${key}`, 'green');

  // Check if secret already exists
  if (secretExists(key, env)) {
    if (!force) {
      throw new Error(`Secret '${key}' already exists! Use --force flag to overwrite existing secrets`);
    } else {
      log(`Warning: Overwriting existing secret '${key}'`, 'yellow');
    }
  }

  // Show preview
  if (key.includes('KEY')) {
    log('Value: [PRIVATE KEY - content hidden]', 'yellow');
  } else {
    const preview = value.substring(0, 4) + '...';
    log(`Value preview: ${preview}`, 'yellow');
  }

  // Upload the secret using stdin
  const envFlag = env ? `--env ${env}` : '';
  
  log(`Uploading ${key}...`, 'blue');
  
  // Use execSync with input option to pass the secret value
  execSync(`wrangler secret put "${key}" ${envFlag}`, {
    input: value,
    encoding: 'utf8',
    stdio: ['pipe', 'inherit', 'inherit']
  });
  
  log(`✓ Uploaded ${key}`, 'green');
}

function main(): void {
  const { env, force, help } = parseArgs();

  if (help) {
    showHelp();
    return;
  }

  // Require environment to be specified
  if (!env) {
    log('Error: Environment must be specified', 'red');
    log('Use --env qa or --env production', 'yellow');
    log('Run --help for more information', 'blue');
    process.exit(1);
  }

  // Validate environment
  if (!['qa', 'production'].includes(env)) {
    log(`Error: Invalid environment '${env}'`, 'red');
    log('Valid environments: qa, production', 'yellow');
    process.exit(1);
  }

  log('🔐 Catnip Secrets Upload Script', 'blue');
  log('================================', 'blue');

  log(`Uploading secrets for environment: ${env}`, 'blue');

  if (force) {
    log('Force mode enabled - will overwrite existing secrets', 'yellow');
  }

  console.log('');

  // Parse secrets from .dev.vars
  const secrets = parseDevVars();

  if (secrets.length === 0) {
    log('No secrets found to upload', 'yellow');
    return;
  }

  log(`Found ${secrets.length} secrets to upload:`, 'blue');
  secrets.forEach((secret, index) => {
    log(`  ${index + 1}. ${secret.key}`, 'blue');
  });
  console.log('');

  // Upload each secret
  for (let i = 0; i < secrets.length; i++) {
    const secret = secrets[i];
    log(`[${i + 1}/${secrets.length}] Processing: ${secret.key}`, 'blue');
    
    try {
      uploadSecret(secret, env, force);
      log(`[${i + 1}/${secrets.length}] ✓ Success: ${secret.key}`, 'green');
    } catch (error: any) {
      log(`[${i + 1}/${secrets.length}] ✗ Failed: ${secret.key}`, 'red');
      log(`Error: ${error.message}`, 'red');
      
      // Don't exit on error, continue with next secret
      log('Continuing with next secret...', 'yellow');
    }
    
    console.log('');
  }

  log('Secret upload complete!', 'green');
  log('You can verify your secrets with: wrangler secret list' + (env ? ` --env ${env}` : ''), 'blue');
}

try {
  main();
} catch (error: any) {
  log(`Error: ${error.message}`, 'red');
  process.exit(1);
}