# Go Version Management

## Overview

The Go version for this project is managed centrally via:

- **`.goversion`** - Single source of truth containing the full Go version (e.g., `1.25.4`)
- **`scripts/update-go.ts`** - Automated script that updates Go versions across all project files

This ensures consistency across Dockerfiles, go.mod, and CI workflows.

## Quick Start

```bash
# Update to a new Go version (updates .goversion and all files)
pnpm exec tsx scripts/update-go.ts 1.26.0

# Update all files to match current .goversion
pnpm exec tsx scripts/update-go.ts
```

## What Gets Updated

The script automatically updates Go versions in:

### 1. Dockerfiles

- **`FROM golang:X.Y`** - Base image tags (major.minor only)
  - `Dockerfile`
  - `container/Dockerfile`
  - `container/Dockerfile.base`
  - `container/Dockerfile.dev`
  - `container/test/Dockerfile.test`

- **`ARG GO_VERSION=X.Y.Z`** - Build arguments (full version)

### 2. Go Module

- **`container/go.mod`** - `go X.Y` directive (major.minor only)

### 3. GitHub Actions Workflows

- **`.github/workflows/test-go.yml`**
  - Matrix `go-version: ["X.Y"]`
  - `setup-go` version
  - Conditional checks `matrix.go-version == 'X.Y'`

- **`.github/workflows/release.yml`**
  - `setup-go` version

## Version Format Strategy

The script uses different version formats for different purposes:

| Location         | Format  | Example  | Reason                            |
| ---------------- | ------- | -------- | --------------------------------- |
| `.goversion`     | `X.Y.Z` | `1.25.4` | Full version for tracking         |
| `FROM golang:`   | `X.Y`   | `1.25`   | Docker uses major.minor tags      |
| `ARG GO_VERSION` | `X.Y.Z` | `1.25.4` | Explicit version for builds       |
| `go.mod`         | `X.Y`   | `1.25`   | Go modules use major.minor        |
| GitHub Actions   | `X.Y`   | `1.25`   | Actions typically use major.minor |

This approach ensures:

- **Stability**: Docker and actions use stable base versions
- **Reproducibility**: Full version tracked in `.goversion` and build args
- **Compatibility**: Follows Go and Docker conventions

## Examples

### Update to Go 1.26.0

```bash
# Run the update script
pnpm exec tsx scripts/update-go.ts 1.26.0

# Review all changes
git diff

# Verify the updates look correct
git diff .goversion
git diff container/go.mod
git diff Dockerfile
git diff .github/workflows/

# Commit the changes
git add .goversion scripts/ Dockerfile container/ .github/workflows/
git commit -m "Update Go to 1.26.0"
```

### Sync files after manual .goversion edit

```bash
# Edit .goversion manually
echo "1.25.5" > .goversion

# Sync all files to match
pnpm exec tsx scripts/update-go.ts

# Review and commit
git diff
git add -A
git commit -m "Update Go to 1.25.5"
```

## Verification

After updating, verify the changes:

```bash
# Check .goversion
cat .goversion

# Check go.mod
grep "^go " container/go.mod

# Check Dockerfiles
grep "golang:" Dockerfile container/Dockerfile*
grep "GO_VERSION" Dockerfile container/Dockerfile*

# Check workflows
grep "go-version" .github/workflows/*.yml
```

## CI/CD Integration

The update script should be run locally before committing. However, you could add a CI check:

```yaml
# Example: Verify Go version consistency
- name: Check Go version consistency
  run: |
    pnpm exec tsx scripts/update-go.ts
    if ! git diff --quiet; then
      echo "::error::Go versions are inconsistent. Run: pnpm exec tsx scripts/update-go.ts"
      exit 1
    fi
```

## Troubleshooting

### Script fails with "MODULE_NOT_FOUND"

Make sure you're running from the project root:

```bash
cd /path/to/catnip
pnpm exec tsx scripts/update-go.ts
```

### Version not updating in a file

Check if the file uses a different format. The script looks for:

- `FROM golang:X.Y` (with optional `-alpine` suffix)
- `ARG GO_VERSION=X.Y.Z`
- `go X.Y` (in go.mod)
- `go-version: "X.Y"` or `go-version: ["X.Y"]` (in workflows)

### Need to update additional files

Edit `scripts/update-go.ts` and add patterns for the new file format.

## Related Scripts

- `scripts/update-versions.sh` - Updates other version numbers (Node, Python, etc.)
- See `scripts/` directory for more automation tools
