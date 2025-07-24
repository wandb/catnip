#!/usr/bin/env node

import { execSync } from "child_process";
import { select, confirm, input } from "@inquirer/prompts";

// Types
type BumpType = "major" | "minor" | "patch" | "dev";

interface Version {
  major: number;
  minor: number;
  patch: number;
  dev: number | null;
}

// Parse command line arguments
const args = process.argv.slice(2);

// Check for help flag
if (args.includes("--help") || args.includes("-h")) {
  console.log(`
🚀 Catnip Release Manager

USAGE:
    pnpm tsx scripts/release.ts [OPTIONS]

SAFETY CHECKS:
    • Ensures you're in a git repository with clean working tree
    • Warns if not on 'main' branch and prompts for confirmation  
    • Checks if local main is up-to-date with remote main
    • Shows commit info (SHA, date, message) for release confirmation

OPTIONS:
    --major                    Create major release (x+1.0.0)
    --minor                    Create minor release (x.y+1.0)
    --patch                    Create patch release (x.y.z+1)
    --dev                      Create dev release (x.y.z+1-dev.1)
    --push                     Push tag to trigger GoReleaser
    --message="text"           Release message (required with --push)
    --message "text"           Release message (alternative format)
    --help, -h                 Show this help

EXAMPLES:
    pnpm tsx scripts/release.ts                                      # Interactive mode
    pnpm tsx scripts/release.ts --patch                             # Create v0.0.1 (patch, interactive)
    pnpm tsx scripts/release.ts --major --push --message="v1.0!"   # Create and release v1.0.0
    pnpm tsx scripts/release.ts --dev --push --message "Bug fixes" # Create dev release with message
`);
  process.exit(0);
}

// Determine bump type from flags or prompt
let bump: BumpType | undefined;
if (args.includes("--major")) bump = "major";
else if (args.includes("--minor")) bump = "minor";
else if (args.includes("--patch")) bump = "patch";
else if (args.includes("--dev")) bump = "dev";

const pushFlag = args.includes("--push");

// Support both --message="text" and --message "text" formats
let messageFlag: string | undefined;
const messageEqualIndex = args.findIndex((arg) => arg.startsWith("--message="));
const messageSpaceIndex = args.findIndex((arg) => arg === "--message");

if (messageEqualIndex !== -1) {
  messageFlag = args[messageEqualIndex].split("=")[1];
} else if (messageSpaceIndex !== -1 && messageSpaceIndex + 1 < args.length) {
  messageFlag = args[messageSpaceIndex + 1];
}

function run(command: string, options: Record<string, any> = {}): string {
  try {
    return execSync(command, { encoding: "utf8", ...options }).trim();
  } catch (error: any) {
    throw new Error(`Command failed: ${command}\n${error.message}`);
  }
}

function getCurrentVersion(): string {
  try {
    // Try to get latest tag
    const latestTag = run(
      'git describe --tags --abbrev=0 2>/dev/null || echo ""',
    );
    if (latestTag && latestTag.startsWith("v")) {
      return latestTag.substring(1); // Remove 'v' prefix
    }
  } catch (error) {
    // Ignore error, fall back to default
  }

  // Default to 0.0.0 if no tags exist
  return "0.0.0";
}

function parseVersion(version: string): Version {
  const match = version.match(/^(\d+)\.(\d+)\.(\d+)(?:-dev\.(\d+))?$/);
  if (!match) {
    throw new Error(`Invalid version format: ${version}`);
  }

  return {
    major: parseInt(match[1]),
    minor: parseInt(match[2]),
    patch: parseInt(match[3]),
    dev: match[4] ? parseInt(match[4]) : null,
  };
}

function bumpVersion(current: string, bumpType: BumpType): string {
  const version = parseVersion(current);

  switch (bumpType) {
    case "major":
      return `${version.major + 1}.0.0`;
    case "minor":
      return `${version.major}.${version.minor + 1}.0`;
    case "patch":
      return `${version.major}.${version.minor}.${version.patch + 1}`;
    case "dev":
      if (version.dev !== null) {
        return `${version.major}.${version.minor}.${version.patch}-dev.${version.dev + 1}`;
      } else {
        return `${version.major}.${version.minor}.${version.patch + 1}-dev.1`;
      }
    default:
      throw new Error(`Invalid bump type: ${bumpType}`);
  }
}

function createTag(version: string, message?: string): string {
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

async function main(): Promise<void> {
  console.log("🚀 Catnip Release Manager\n");

  // Check if we're in a git repo
  try {
    run("git rev-parse --git-dir");
  } catch (error) {
    console.error("❌ Not in a git repository");
    process.exit(1);
  }

  // Check for uncommitted changes
  const status = run("git status --porcelain");
  if (status) {
    console.error(
      "❌ Uncommitted changes detected. Please commit or stash changes first.",
    );
    process.exit(1);
  }

  // Check current branch and HEAD status
  const currentBranch = run("git branch --show-current");
  const currentCommit = run("git rev-parse HEAD");
  const currentCommitShort = run("git rev-parse --short HEAD");
  const currentCommitDate = run("git log -1 --format=%cd --date=short");
  const currentCommitMessage = run("git log -1 --format=%s");

  // Check if we're on main branch
  if (currentBranch !== "main") {
    console.warn(`⚠️  You are on branch '${currentBranch}', not 'main'`);

    const continueFromBranch = await confirm({
      message: `Do you want to release from branch '${currentBranch}'?`,
      default: false,
    });

    if (!continueFromBranch) {
      console.log("💡 Switch to main branch: git checkout main");
      process.exit(0);
    }
  }

  // Check if we're up to date with remote main (only if on main)
  if (currentBranch === "main") {
    try {
      // Fetch latest to ensure we have up-to-date remote info
      run("git fetch origin main");

      const remoteMainCommit = run("git rev-parse origin/main");

      if (currentCommit !== remoteMainCommit) {
        const remoteCommitShort = run("git rev-parse --short origin/main");
        const remoteCommitDate = run(
          "git log -1 --format=%cd --date=short origin/main",
        );
        const remoteCommitMessage = run("git log -1 --format=%s origin/main");

        console.warn(`⚠️  Your local main is not up to date with remote main:`);
        console.warn(
          `   Local:  ${currentCommitShort} (${currentCommitDate}) ${currentCommitMessage}`,
        );
        console.warn(
          `   Remote: ${remoteCommitShort} (${remoteCommitDate}) ${remoteCommitMessage}`,
        );

        const continueFromOldCommit = await confirm({
          message: `Do you want to release from commit ${currentCommitShort} instead of the latest remote main?`,
          default: false,
        });

        if (!continueFromOldCommit) {
          console.log("💡 Update your main branch: git pull origin main");
          process.exit(0);
        }
      }
    } catch (error) {
      console.warn(`⚠️  Could not check remote main (${error}). Continuing...`);
    }
  } else {
    // For non-main branches, show current commit info
    console.log(
      `📍 Releasing from: ${currentCommitShort} (${currentCommitDate}) ${currentCommitMessage}`,
    );
  }

  // Get current version
  const currentVersion = getCurrentVersion();
  console.log(`📍 Current version: v${currentVersion}`);

  // Prompt for bump type if not specified
  if (!bump) {
    bump = await select({
      message: "🔢 Select version bump type:",
      choices: [
        {
          name: "Minor (x.y+1.0) - New features, backwards compatible",
          value: "minor",
        },
        {
          name: "Patch (x.y.z+1) - Bug fixes, backwards compatible",
          value: "patch",
        },
        { name: "Major (x+1.0.0) - Breaking changes", value: "major" },
        { name: "Dev (x.y.z+1-dev.1) - Development release", value: "dev" },
      ],
      default: "minor",
    });
  }

  // Calculate new version
  const newVersion = bumpVersion(currentVersion, bump);
  console.log(`⬆️  New version: v${newVersion} (${bump} bump)`);

  // Prompt for push if not specified
  let push = pushFlag;
  if (push === undefined) {
    push = await confirm({
      message: "🚀 Push tag to trigger GoReleaser release?",
      default: false,
    });
  }

  // Prompt for message if pushing and not specified
  let message = messageFlag;
  if (push && !message) {
    message = await input({
      message: "📝 Enter release message:",
      default: `Release v${newVersion}`,
      validate: (value) => {
        if (!value.trim()) {
          return "Release message is required when pushing";
        }
        return true;
      },
    });
  }

  // Show what would happen
  console.log("\n📋 Plan:");
  console.log(`   1. Create tag: v${newVersion}`);
  if (push) {
    console.log(`   2. Push tag to origin (triggers GoReleaser)`);
    console.log(`   3. Message: "${message}"`);
  } else {
    console.log(`   2. Keep tag local (use --push to trigger release)`);
  }

  // Confirm if not pushing
  if (!push) {
    console.log("\n⚠️  This will create a LOCAL tag only.");
    console.log("💡 Run with --push flag to trigger the release.");
  }

  // Create the tag
  let tag: string | undefined;
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
  } catch (error: any) {
    console.error(`❌ Failed to create tag: ${error.message}`);

    // Clean up if tag was created but push failed
    if (tag && push) {
      try {
        run(`git tag -d ${tag}`);
        console.log(`🧹 Cleaned up local tag ${tag}`);
      } catch (cleanupError: any) {
        console.error(`⚠️  Could not clean up tag: ${cleanupError.message}`);
      }
    }
    process.exit(1);
  }
}

main().catch((error) => {
  console.error("❌ Unexpected error:", error);
  process.exit(1);
});
