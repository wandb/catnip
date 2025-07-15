#!/usr/bin/env node

import { execSync } from 'child_process';
import fs from 'fs';
import path from 'path';

// Parse command line arguments
const args = process.argv.slice(2);
const bump = args[0] || 'minor';
const push = args.includes('--push');
const message = args.find(arg => arg.startsWith('--message='))?.split('=')[1];

// Validate bump type
const validBumps = ['major', 'minor', 'patch', 'dev'];
if (!validBumps.includes(bump)) {
  console.error(`❌ Invalid bump type: ${bump}`);
  console.error(`   Valid options: ${validBumps.join(', ')}`);
  process.exit(1);
}

// If pushing, require a message
if (push && !message) {
  console.error('❌ --message is required when using --push');
  console.error('   Example: just release minor --push --message="Add new feature"');
  process.exit(1);
}

function run(command, options = {}) {
  try {
    return execSync(command, { encoding: 'utf8', ...options }).trim();
  } catch (error) {
    throw new Error(`Command failed: ${command}\n${error.message}`);
  }
}

function getCurrentVersion() {
  try {
    // Try to get latest tag
    const latestTag = run('git describe --tags --abbrev=0 2>/dev/null || echo ""');
    if (latestTag && latestTag.startsWith('v')) {
      return latestTag.substring(1); // Remove 'v' prefix
    }
  } catch (error) {
    // Ignore error, fall back to default
  }
  
  // Default to 0.0.0 if no tags exist
  return '0.0.0';
}

function parseVersion(version) {
  const match = version.match(/^(\d+)\.(\d+)\.(\d+)(?:-dev\.(\d+))?$/);
  if (!match) {
    throw new Error(`Invalid version format: ${version}`);
  }
  
  return {
    major: parseInt(match[1]),
    minor: parseInt(match[2]),
    patch: parseInt(match[3]),
    dev: match[4] ? parseInt(match[4]) : null
  };
}

function bumpVersion(current, bumpType) {
  const version = parseVersion(current);
  
  switch (bumpType) {
    case 'major':
      return `${version.major + 1}.0.0`;
    case 'minor':
      return `${version.major}.${version.minor + 1}.0`;
    case 'patch':
      return `${version.major}.${version.minor}.${version.patch + 1}`;
    case 'dev':
      if (version.dev !== null) {
        return `${version.major}.${version.minor}.${version.patch}-dev.${version.dev + 1}`;
      } else {
        return `${version.major}.${version.minor}.${version.patch + 1}-dev.1`;
      }
    default:
      throw new Error(`Invalid bump type: ${bumpType}`);
  }
}

function createTag(version, message) {
  const tag = `v${version}`;
  const tagMessage = message || `Release ${tag}`;
  
  console.log(`📦 Creating tag: ${tag}`);
  console.log(`📝 Message: ${tagMessage}`);
  
  if (message) {
    run(`git tag -a ${tag} -m "${tagMessage}"`);
  } else {
    run(`git tag ${tag}`);
  }
  
  return tag;
}

function main() {
  console.log('🚀 Catnip Release Manager\n');
  
  // Check if we're in a git repo
  try {
    run('git rev-parse --git-dir');
  } catch (error) {
    console.error('❌ Not in a git repository');
    process.exit(1);
  }
  
  // Check for uncommitted changes
  const status = run('git status --porcelain');
  if (status) {
    console.error('❌ Uncommitted changes detected. Please commit or stash changes first.');
    process.exit(1);
  }
  
  // Get current version
  const currentVersion = getCurrentVersion();
  console.log(`📍 Current version: v${currentVersion}`);
  
  // Calculate new version
  const newVersion = bumpVersion(currentVersion, bump);
  console.log(`⬆️  New version: v${newVersion} (${bump} bump)`);
  
  // Show what would happen
  console.log('\n📋 Plan:');
  console.log(`   1. Create tag: v${newVersion}`);
  if (push) {
    console.log(`   2. Push tag to origin (triggers GoReleaser)`);
  } else {
    console.log(`   2. Keep tag local (use --push to trigger release)`);
  }
  
  // Confirm if not pushing
  if (!push) {
    console.log('\n⚠️  This will create a LOCAL tag only. Use --push to trigger the release.');
    console.log('💡 Example: just release minor --push --message="Add awesome feature"');
  }
  
  // Create the tag
  let tag;
  try {
    tag = createTag(newVersion, message);
    console.log(`✅ Tag ${tag} created successfully`);
    
    if (push) {
      console.log(`🚀 Pushing tag to origin...`);
      run(`git push origin ${tag}`);
      console.log(`✅ Tag pushed! GoReleaser should start building release.`);
      console.log(`🔗 Check: https://github.com/wandb/catnip/actions`);
    } else {
      console.log(`📍 Local tag created. To push later:`);
      console.log(`   git push origin ${tag}`);
      console.log(`\n🧹 To clean up local tag:`);
      console.log(`   git tag -d ${tag}`);
    }
    
  } catch (error) {
    console.error(`❌ Failed to create tag: ${error.message}`);
    
    // Clean up if tag was created but push failed
    if (tag && push) {
      try {
        run(`git tag -d ${tag}`);
        console.log(`🧹 Cleaned up local tag ${tag}`);
      } catch (cleanupError) {
        console.error(`⚠️  Could not clean up tag: ${cleanupError.message}`);
      }
    }
    process.exit(1);
  }
}

main();