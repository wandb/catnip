#!/usr/bin/env tsx
/**
 * Update Go version across all project files
 *
 * Usage: tsx scripts/update-go.ts [version]
 *
 * If no version is provided, reads from .goversion file.
 *
 * Updates:
 * - .goversion file
 * - ARG GO_VERSION in Dockerfiles
 * - FROM golang: image tags in Dockerfiles
 * - go.mod go directive
 * - GitHub Actions workflow go-version matrix and setup-go
 */

import { readFileSync, writeFileSync, existsSync, readdirSync } from "fs";
import { join } from "path";

const GOVERSION_FILE = ".goversion";

function getVersion(providedVersion?: string): {
  full: string;
  major: string;
  minor: string;
} {
  const versionStr =
    providedVersion || readFileSync(GOVERSION_FILE, "utf-8").trim();

  if (!versionStr) {
    console.error("❌ No version provided and .goversion file is empty");
    process.exit(1);
  }

  const parts = versionStr.split(".");
  if (parts.length < 3) {
    console.error(
      `❌ Invalid version format: ${versionStr}. Expected format: X.Y.Z`,
    );
    process.exit(1);
  }

  return {
    full: versionStr,
    major: parts[0],
    minor: parts[1],
  };
}

function updateDockerfile(
  filePath: string,
  version: { full: string; major: string; minor: string },
) {
  if (!existsSync(filePath)) {
    console.warn(`⚠️  File not found: ${filePath}`);
    return;
  }

  let content = readFileSync(filePath, "utf-8");
  let modified = false;

  // Update ARG GO_VERSION=X.Y.Z
  const argPattern = /^ARG GO_VERSION=[\d.]+$/gm;
  if (argPattern.test(content)) {
    content = content.replace(argPattern, `ARG GO_VERSION=${version.full}`);
    modified = true;
    console.log(`  ✓ Updated ARG GO_VERSION to ${version.full}`);
  }

  // Update FROM golang:X.Y
  const fromPattern = /^FROM golang:[\d.]+/gm;
  if (fromPattern.test(content)) {
    const shortVersion = `${version.major}.${version.minor}`;
    content = content.replace(fromPattern, (match) => {
      // Preserve any suffix like -alpine
      const suffix = match.match(/golang:[\d.]+(-.+)?/)?.[1] || "";
      return `FROM golang:${shortVersion}${suffix}`;
    });
    modified = true;
    console.log(`  ✓ Updated FROM golang to ${version.major}.${version.minor}`);
  }

  if (modified) {
    writeFileSync(filePath, content, "utf-8");
  }
}

function updateGoMod(
  filePath: string,
  version: { full: string; major: string; minor: string },
) {
  if (!existsSync(filePath)) {
    console.warn(`⚠️  File not found: ${filePath}`);
    return;
  }

  let content = readFileSync(filePath, "utf-8");
  const shortVersion = `${version.major}.${version.minor}`;

  // Update go X.Y directive
  const goPattern = /^go \d+\.\d+$/gm;
  if (goPattern.test(content)) {
    content = content.replace(goPattern, `go ${shortVersion}`);
    writeFileSync(filePath, content, "utf-8");
    console.log(`  ✓ Updated go directive to ${shortVersion}`);
  }
}

function updateGitHubWorkflow(
  filePath: string,
  version: { full: string; major: string; minor: string },
) {
  if (!existsSync(filePath)) {
    console.warn(`⚠️  File not found: ${filePath}`);
    return;
  }

  let content = readFileSync(filePath, "utf-8");
  let modified = false;
  const shortVersion = `${version.major}.${version.minor}`;

  // Update go-version in matrix (e.g., go-version: ["1.25"])
  const matrixPattern = /go-version:\s*\["[\d.]+"\]/g;
  if (matrixPattern.test(content)) {
    content = content.replace(matrixPattern, `go-version: ["${shortVersion}"]`);
    modified = true;
    console.log(`  ✓ Updated matrix go-version to ${shortVersion}`);
  }

  // Update go-version in setup-go (e.g., go-version: "1.25")
  const setupPattern = /go-version:\s*"[\d.]+"/g;
  if (setupPattern.test(content)) {
    content = content.replace(setupPattern, `go-version: "${shortVersion}"`);
    modified = true;
    console.log(`  ✓ Updated setup-go version to ${shortVersion}`);
  }

  // Update conditions checking go-version (e.g., matrix.go-version == '1.25')
  const conditionPattern = /matrix\.go-version\s*==\s*'[\d.]+'/g;
  if (conditionPattern.test(content)) {
    content = content.replace(
      conditionPattern,
      `matrix.go-version == '${shortVersion}'`,
    );
    modified = true;
    console.log(`  ✓ Updated go-version condition to ${shortVersion}`);
  }

  if (modified) {
    writeFileSync(filePath, content, "utf-8");
  }
}

function main() {
  const providedVersion = process.argv[2];
  const version = getVersion(providedVersion);

  console.log(
    `\n🔄 Updating Go version to ${version.full} (${version.major}.${version.minor})\n`,
  );

  // Update .goversion if version was provided as argument
  if (providedVersion) {
    writeFileSync(GOVERSION_FILE, version.full + "\n", "utf-8");
    console.log(`✓ Updated ${GOVERSION_FILE}\n`);
  }

  // Update Dockerfiles
  console.log("📝 Updating Dockerfiles:");
  const dockerfiles = [
    "Dockerfile",
    "container/Dockerfile",
    "container/Dockerfile.base",
    "container/Dockerfile.dev",
  ].filter(existsSync);

  // Also search for Dockerfiles in container/test directory if it exists
  if (existsSync("container/test")) {
    const testFiles = readdirSync("container/test");
    for (const file of testFiles) {
      if (file.startsWith("Dockerfile")) {
        dockerfiles.push(join("container/test", file));
      }
    }
  }

  for (const dockerfile of dockerfiles) {
    console.log(`  ${dockerfile}:`);
    updateDockerfile(dockerfile, version);
  }
  console.log();

  // Update go.mod
  console.log("📝 Updating go.mod:");
  console.log("  container/go.mod:");
  updateGoMod("container/go.mod", version);
  console.log();

  // Update GitHub Actions workflows
  console.log("📝 Updating GitHub Actions workflows:");
  const workflows = [
    ".github/workflows/test-go.yml",
    ".github/workflows/release.yml",
  ].filter(existsSync);

  for (const workflow of workflows) {
    console.log(`  ${workflow}:`);
    updateGitHubWorkflow(workflow, version);
  }
  console.log();

  console.log(`✅ Go version updated successfully!`);
  console.log(`\nNext steps:`);
  console.log(`  1. Review changes: git diff`);
  console.log(`  2. Test build: docker build -f Dockerfile .`);
  console.log(
    `  3. Commit: git add -A && git commit -m "Update Go to ${version.full}"\n`,
  );
}

main();
