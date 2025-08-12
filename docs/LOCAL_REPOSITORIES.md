# Local Repository Support

This document describes how Catnip handles local repositories mounted into the container.

## Overview

Catnip supports working with local Git repositories that are mounted into the container at `/live/`. This enables development workflows where the source code exists outside the container but can be worked on using Catnip's worktree-based development environment.

## How It Works

### Detection and Loading

1. **Startup Detection**: When the GitService starts, it scans `/live/` for directories containing `.git` folders
2. **Repository Registration**: Each detected repository is registered with the ID format `local/{directory_name}`
3. **Example**: A repository at `/live/catnip` becomes `local/catnip`

### Frontend Integration

- Local repositories appear in the repository selector with the description "Local repository (mounted)"
- They are listed separately from GitHub repositories in the `/v1/git/github/repos` endpoint
- The frontend handles `local/` prefixed repositories differently from remote GitHub repos

### Worktree Creation

When checking out a local repository:

- No cloning occurs (unlike GitHub repos)
- Worktrees are created directly from the mounted repository
- The `handleLocalRepoWorktree()` function manages this process
- New worktrees are created in `/workspace/{repo_name}/{session_name}` format

## Key Files

### Backend

- `container/internal/services/git.go`:
  - `detectLocalRepos()`: Scans `/live/` for Git repositories
  - `handleLocalRepoWorktree()`: Creates worktrees for local repos
  - `ListGitHubRepositories()`: Includes local repos in the response

### Frontend

- `src/routes/git.tsx`: `handleCheckout()` function routes `local/` prefixed repos correctly
- `src/components/RepoSelector.tsx`: Displays local repos with appropriate labels

## API Endpoints

- `GET /v1/git/github/repos`: Returns both GitHub and local repositories
- `POST /v1/git/checkout/local/{repo_name}`: Creates a worktree for a local repository
- `GET /v1/git/status`: Shows current repository status including local repos

## Configuration

Local repositories are automatically detected if:

1. A directory exists in `/live/`
2. The directory contains a `.git` folder
3. The container has read/write access to the directory

## Debugging

Common issues:

- **Repository not detected**: Check if `/live/{repo_name}/.git` exists and has proper permissions
- **Checkout fails**: Ensure the repository ID uses the `local/` prefix
- **Worktree creation fails**: Verify the base repository is a valid Git repository

### Logging

Look for these log messages:

- `🔍 Detected local repository at /live/{name}`
- `✅ Local repository loaded: local/{name}`
- `📦 Checkout request: local/{name}`
- `🔍 Creating preview branch preview/{branch} for worktree {name}`
- `📝 Created temporary commit {hash} with uncommitted changes`
- `✅ Preview branch {name} created with uncommitted changes`

## Preview Functionality

Local repositories support a "Preview" feature that allows you to view changes outside the container by creating a branch in the main repository.

### How Preview Works

1. **Branch Creation**: Creates a `preview/{worktree_branch}` branch in the main repository
2. **Uncommitted Changes**: Automatically includes all uncommitted changes (staged, unstaged, and untracked files)
3. **Temporary Commit**: Creates a temporary commit in the worktree with uncommitted changes
4. **Push to Main**: Pushes the worktree branch (including the temporary commit) to the preview branch
5. **Cleanup**: Removes the temporary commit from the worktree, preserving uncommitted changes

### Key Features

- **Comprehensive Change Inclusion**: Captures all types of changes (staged, unstaged, untracked)
- **Non-destructive**: Preserves the original state of the worktree
- **Overwrite Previous Previews**: Each preview operation overwrites the previous preview branch
- **Local Repository Only**: Feature is restricted to local repositories for security

### Usage

1. Make changes in your worktree (committed or uncommitted)
2. Click the "Preview" button (eye icon) in the UI
3. Check out the `preview/{branch_name}` branch in your main repository outside the container
4. View your changes, including any uncommitted work

### Implementation Details

- **API Endpoint**: `POST /v1/git/worktrees/{id}/preview`
- **Backend Function**: `CreateWorktreePreview()` in `git.go`
- **Helper Functions**: `hasUncommittedChanges()` and `createTemporaryCommit()`

## Template-Created Repositories

In addition to mounted local repositories, Catnip can create new repositories from templates using the `CreateFromTemplate` function. These template-created repositories use a persistent bare repository approach.

### How Template Repositories Work

1. **Template Creation**: When creating from a template (e.g., React, Vue, Next.js):
   - Template files are generated in a temporary directory
   - A regular git repository is initialized and committed
   - The temporary repo is cloned as a bare repository to `/volume/repos/{name}.git`

2. **Persistent Storage**: Unlike mounted repos in `/live/`, template repositories are stored in `/volume/repos/` which persists across container restarts

3. **Bare Repository Pattern**: Template repositories use the same architecture as remote GitHub repositories:
   - Bare repository at `/volume/repos/{name}.git`
   - Repository ID format: `local/{name}`
   - Worktrees created in `/workspace/{name}/{session}/`

### Key Differences from Mounted Repositories

| Aspect                | Mounted (`/live/`)                    | Template-Created (`/volume/repos/`)  |
| --------------------- | ------------------------------------- | ------------------------------------ |
| **Storage**           | External mount, disappears on restart | Persistent volume, survives restarts |
| **Repository Type**   | Regular git repository                | Bare git repository                  |
| **Creation Method**   | Pre-existing, detected on startup     | Created via `CreateFromTemplate()`   |
| **Worktree Handling** | Direct worktree from mounted repo     | Worktree from bare repository        |

### Template Repository API

- `POST /v1/git/templates/create`: Creates a new repository from a template
- **Function**: `CreateFromTemplate()` in `git.go`
- **Supported Templates**: `react-vite`, `vue-vite`, `nextjs-app`, `node-express`, `python-fastapi`

### Implementation Details

The `CreateFromTemplate` function follows this process:

1. **Validation**: Checks if project name is valid and doesn't already exist
2. **Template Setup**: Creates template files in temporary directory
3. **Git Initialization**: Initializes git repo, adds files, makes initial commit
4. **Bare Clone**: Clones temporary repo as bare repository to persistent location
5. **Repository Registration**: Creates `Repository` object pointing to bare repo
6. **Worktree Creation**: Creates initial worktree for immediate development

This approach ensures template projects work identically to GitHub repositories and persist across container restarts.

## Example Workflows

### Mounted Repository Workflow

1. Mount a Git repository to `/live/myproject`
2. Start Catnip container
3. Repository appears as `local/myproject` in the UI
4. Select and checkout to create a worktree
5. Work in the worktree at `/workspace/myproject/{session_name}`
6. Use "Preview" to create a branch with your changes in the main repository
7. Changes can be merged back to the main repository using the "Merge to Main" feature

### Template Repository Workflow

1. Create a new project from template via UI or API
2. Template repository is created at `/volume/repos/myproject.git`
3. Repository appears as `local/myproject` in the UI
4. Initial worktree is automatically created for development
5. Work in the worktree at `/workspace/myproject/{session_name}`
6. Project persists across container restarts
7. Additional worktrees can be created as needed
