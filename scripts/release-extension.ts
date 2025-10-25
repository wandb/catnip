#!/usr/bin/env node

import { execSync } from "child_process";
import { select, confirm, input } from "@inquirer/prompts";
import { readFileSync, writeFileSync } from "fs";
import { join } from "path";

// Types
type BumpType = "major" | "minor" | "patch";

interface Version {
  major: number;
  minor: number;
  patch: number;
}

// Parse command line arguments
const args = process.argv.slice(2);

// Check for help flag
if (args.includes("--help") || args.includes("-h")) {
  console.log(`
🚀 Catnip VS Code Extension Release Manager

USAGE:
    pnpm tsx scripts/release-extension.ts [OPTIONS]

SAFETY CHECKS:
    • Ensures you're in a git repository with clean working tree
    • Ensures you're on 'main' branch
    • Checks if local main is up-to-date with remote main
    • Validates package.json version matches expected format
    • Checks if tag already exists

OPTIONS:
    --major                    Create major release (x+1.0.0)
    --minor                    Create minor release (x.y+1.0)
    --patch                    Create patch release (x.y.z+1)
    --push                     Push tag to trigger extension publishing
    --help, -h                 Show this help

EXAMPLES:
    pnpm tsx scripts/release-extension.ts                    # Interactive mode
    pnpm tsx scripts/release-extension.ts --patch           # Create patch release
    pnpm tsx scripts/release-extension.ts --minor --push    # Create and publish minor release
`);
  process.exit(0);
}

// Determine bump type from flags or prompt
let bump: BumpType | undefined;
if (args.includes("--major")) bump = "major";
else if (args.includes("--minor")) bump = "minor";
else if (args.includes("--patch")) bump = "patch";

const pushFlag = args.includes("--push");

function run(command: string, options: Record<string, any> = {}): string {
  try {
    return execSync(command, { encoding: "utf8", ...options }).trim();
  } catch (error: any) {
    throw new Error(`Command failed: ${command}\n${error.message}`);
  }
}

function getPackageVersion(): string {
  const packagePath = join(
    process.cwd(),
    ".devcontainer/features/feature/catnip-vscode-extension/package.json",
  );
  const packageJson = JSON.parse(readFileSync(packagePath, "utf8"));
  return packageJson.version;
}

function updatePackageVersion(newVersion: string): void {
  const packagePath = join(
    process.cwd(),
    ".devcontainer/features/feature/catnip-vscode-extension/package.json",
  );
  const packageJson = JSON.parse(readFileSync(packagePath, "utf8"));
  packageJson.version = newVersion;
  writeFileSync(packagePath, JSON.stringify(packageJson, null, 2) + "\n");
}

function parseVersion(version: string): Version {
  const match = version.match(/^(\d+)\.(\d+)\.(\d+)$/);
  if (!match) {
    throw new Error(`Invalid version format: ${version}`);
  }

  return {
    major: parseInt(match[1]),
    minor: parseInt(match[2]),
    patch: parseInt(match[3]),
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
    default:
      throw new Error(`Invalid bump type: ${bumpType}`);
  }
}

function createTag(version: string): string {
  const tag = `feature_vscode_${version}`;

  console.log(`📦 Creating tag: ${tag}`);
  run(`git tag ${tag}`);

  return tag;
}

function tagExists(tag: string): boolean {
  try {
    run(`git rev-parse ${tag}`);
    return true;
  } catch {
    return false;
  }
}

async function main(): Promise<void> {
  console.log("🚀 Catnip VS Code Extension Release Manager\n");

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

  // Check current branch
  const currentBranch = run("git branch --show-current");
  if (currentBranch !== "main") {
    console.error(
      `❌ You must be on the 'main' branch (currently on '${currentBranch}')`,
    );
    console.log("💡 Switch to main branch: git checkout main");
    process.exit(1);
  }

  // Check if we're up to date with remote main
  try {
    // Fetch latest to ensure we have up-to-date remote info
    run("git fetch origin main");

    const currentCommit = run("git rev-parse HEAD");
    const remoteMainCommit = run("git rev-parse origin/main");

    if (currentCommit !== remoteMainCommit) {
      const currentCommitShort = run("git rev-parse --short HEAD");
      const remoteCommitShort = run("git rev-parse --short origin/main");
      const currentCommitDate = run("git log -1 --format=%cd --date=short");
      const remoteCommitDate = run(
        "git log -1 --format=%cd --date=short origin/main",
      );
      const currentCommitMessage = run("git log -1 --format=%s");
      const remoteCommitMessage = run("git log -1 --format=%s origin/main");

      console.warn(`⚠️  Your local main is not up to date with remote main:`);
      console.warn(
        `   Local:  ${currentCommitShort} (${currentCommitDate}) ${currentCommitMessage}`,
      );
      console.warn(
        `   Remote: ${remoteCommitShort} (${remoteCommitDate}) ${remoteCommitMessage}`,
      );

      const continueFromOldCommit = await confirm({
        message: `Do you want to continue anyway?`,
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

  // Get current version from package.json
  const currentVersion = getPackageVersion();
  console.log(`📍 Current version: ${currentVersion}`);

  // Prompt for bump type if not specified
  if (!bump) {
    bump = await select({
      message: "🔢 Select version bump type:",
      choices: [
        {
          name: "Patch (x.y.z+1) - Bug fixes, backwards compatible",
          value: "patch",
        },
        {
          name: "Minor (x.y+1.0) - New features, backwards compatible",
          value: "minor",
        },
        { name: "Major (x+1.0.0) - Breaking changes", value: "major" },
      ],
      default: "patch",
    });
  }

  // Calculate new version
  const newVersion = bumpVersion(currentVersion, bump);
  const newTag = `feature_vscode_${newVersion}`;
  console.log(`⬆️  New version: ${newVersion} (${bump} bump)`);
  console.log(`📦 Tag will be: ${newTag}`);

  // Check if tag already exists
  if (tagExists(newTag)) {
    console.error(`❌ Tag ${newTag} already exists!`);
    console.log("💡 Delete it with: git tag -d " + newTag);
    process.exit(1);
  }

  // Prompt for push if not specified
  let push = pushFlag;
  if (push === undefined) {
    push = await confirm({
      message: "🚀 Push tag to trigger VS Code Marketplace publishing?",
      default: false,
    });
  }

  // Show what would happen
  console.log("\n📋 Plan:");
  console.log(
    `   1. Update package.json version: ${currentVersion} → ${newVersion}`,
  );
  console.log(`   2. Commit version bump`);
  console.log(`   3. Create tag: ${newTag}`);
  if (push) {
    console.log(
      `   4. Push tag to origin (triggers publishing to VS Code Marketplace)`,
    );
  } else {
    console.log(`   4. Keep tag local (use --push to trigger publishing)`);
  }

  // Confirm if not pushing
  if (!push) {
    console.log("\n⚠️  This will create a LOCAL tag only.");
    console.log("💡 Run with --push flag to trigger the publishing.");
  }

  const proceed = await confirm({
    message: "Continue?",
    default: true,
  });

  if (!proceed) {
    console.log("❌ Aborted");
    process.exit(0);
  }

  // Update package.json
  let tag: string | undefined;
  try {
    console.log(`\n📝 Updating package.json...`);
    updatePackageVersion(newVersion);
    console.log(`✅ package.json updated to ${newVersion}`);

    // Commit the version bump
    console.log(`📝 Committing version bump...`);
    run(
      "git add .devcontainer/features/feature/catnip-vscode-extension/package.json",
    );
    run(`git commit -m "Bump VS Code extension version to ${newVersion}"`);
    console.log(`✅ Version bump committed`);

    // Create the tag
    tag = createTag(newVersion);
    console.log(`✅ Tag ${tag} created successfully`);

    if (push) {
      console.log(`\n🚀 Pushing commit and tag to origin...`);
      run(`git push origin main`);
      run(`git push origin ${tag}`);
      console.log(`✅ Tag pushed! Extension publishing should start.`);
      console.log(`🔗 Check: https://github.com/wandb/catnip/actions`);
    } else {
      console.log(`\n📍 Local tag created. To push later:`);
      console.log(`   git push origin main`);
      console.log(`   git push origin ${tag}`);
      console.log(`\n🧹 To clean up:`);
      console.log(`   git tag -d ${tag}`);
      console.log(`   git reset --hard HEAD~1  # Undo version bump commit`);
    }
  } catch (error: any) {
    console.error(`❌ Failed: ${error.message}`);

    // Clean up if tag was created but push failed
    if (tag && push) {
      try {
        run(`git tag -d ${tag}`);
        run("git reset --hard HEAD~1");
        console.log(`🧹 Cleaned up tag and commit`);
      } catch (cleanupError: any) {
        console.error(`⚠️  Could not clean up: ${cleanupError.message}`);
      }
    }
    process.exit(1);
  }
}

main().catch((error) => {
  console.error("❌ Unexpected error:", error);
  process.exit(1);
});
