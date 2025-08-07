# Branch Synchronization Architecture

## Overview

Catnip uses a dual-branch system to provide both isolation for container-based development and seamless integration with external Git tools. This document describes how worktrees remain on custom refs while maintaining synchronized nice branches for external access.

## Key Concepts

### Custom Refs (refs/catnip/\*)

- **Purpose**: Isolate worktree branches from regular Git branches
- **Format**: `refs/catnip/<workspace-name>` (e.g., `refs/catnip/muddy-cat`)
- **Behavior**: Worktrees remain permanently on these custom refs
- **Benefits**:
  - Prevents conflicts when multiple worktrees exist
  - Allows the nice branch to be checked out externally without "already checked out" errors
  - Provides clear separation between container and external work

### Nice Branches

- **Purpose**: Human-readable branches for external access and pull requests
- **Format**: Standard Git branches (e.g., `feature/awesome-feature`, `fix/bug-123`)
- **Creation**: Generated automatically when Claude provides a session title
- **Synchronization**: Kept in sync with the custom ref via commit hooks

## Architecture

```
Container Worktree              Main Repository
┌─────────────────┐            ┌──────────────────┐
│ refs/catnip/    │            │ Regular Branches │
│   muddy-cat     │───sync────▶│ feature/nice-    │
│ (HEAD)          │            │   branch-name    │
└─────────────────┘            └──────────────────┘
        │                               │
        │                               │
        └──────── Can be checked ──────┘
                  out externally
```

## Implementation Details

### 1. Worktree Creation

When a worktree is created:

1. Checkout happens on a custom ref (`refs/catnip/<name>`)
2. For local repos, a "catnip-live" remote is added pointing to the main repository
3. Worktree remains on this custom ref throughout its lifecycle

### 2. Branch Renaming (Nice Branch Creation)

When Claude generates a nice branch name:

1. The nice branch is created at the current commit WITHOUT switching the worktree
2. A mapping is stored in git config: `catnip.branch-map.refs.catnip.<name> = <nice-branch>`
3. The UI displays the nice branch name while the worktree stays on the custom ref

### 3. Commit Synchronization

When commits are made in the worktree:

1. Commits go to the custom ref first
2. The commit sync service detects the new commit
3. The nice branch is updated to point to the same commit
4. For local repos, the nice branch is pushed to the "catnip-live" remote

### 4. Pull Request Creation

When creating a pull request:

1. The system checks for a mapped nice branch
2. Ensures the nice branch is up-to-date with the custom ref
3. Pushes the nice branch (not the custom ref) to the remote
4. Creates the PR using the nice branch

## Configuration

### Git Config Mappings

Branch mappings are stored in the worktree's git config:

```
[catnip]
    branch-map.refs.catnip.muddy-cat = feature/awesome-feature
```

### Catnip-Live Remote (Local Repos Only)

For local repositories, a "catnip-live" remote is configured:

```
[remote "catnip-live"]
    url = /path/to/main/repository
```

## Benefits

1. **External Access**: Nice branches can be checked out outside the container without conflicts
2. **Clean Git History**: Pull requests use properly named branches
3. **Container Isolation**: Worktrees remain isolated on custom refs
4. **Automatic Syncing**: Changes are automatically synchronized between refs
5. **No Manual Switching**: Developers don't need to manually manage branch switching

## Sync Flow Example

```bash
# 1. Worktree is on custom ref
$ git symbolic-ref HEAD
refs/catnip/muddy-cat

# 2. Make changes and commit
$ git commit -m "Add new feature"

# 3. Commit goes to custom ref
$ git log --oneline -1
abc123 Add new feature

# 4. Nice branch automatically synced
$ git log feature/awesome-feature --oneline -1
abc123 Add new feature

# 5. External checkout works
$ cd /main/repo
$ git checkout feature/awesome-feature  # Success! No conflicts
```

## Troubleshooting

### Branch Not Syncing

Check if branch mapping exists:

```bash
git config --get catnip.branch-map.refs.catnip.<workspace-name>
```

### Nice Branch Out of Sync

The sync service should handle this automatically, but you can manually sync:

```bash
git branch -f <nice-branch> HEAD
```

### Pull Request Using Wrong Branch

Ensure the branch mapping is correctly configured and the nice branch exists.

## Future Enhancements

1. **Bidirectional Sync**: Sync changes made to the nice branch back to the custom ref
2. **Conflict Resolution**: Handle conflicts when external changes are made to the nice branch
3. **Branch Protection**: Prevent accidental deletion of nice branches that are mapped
4. **UI Indicators**: Show sync status in the UI
