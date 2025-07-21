# GitService Refactoring Plan

This document outlines how to refactor the unruly `container/internal/services/git.go` by extracting functionality into the `container/internal/git/` package.

## Current State Analysis

The GitService currently has ~2000+ lines with mixed responsibilities:

- Git command execution (lines 162-188)
- Branch operations (lines 364-391, 394-437)
- Fetch/Push strategies (lines 439-506, 270-340)
- Status checking (lines 508-562)
- URL management (lines 191-258)
- Repository and worktree management
- State persistence

## Refactoring Approach

### 1. Add ServiceHelper to GitService

```go
type GitService struct {
	repositories map[string]*models.Repository
	worktrees    map[string]*models.Worktree
	manager      git.Manager
	helper       *git.ServiceHelper  // NEW: Add helper
	mu           sync.RWMutex
}

func NewGitService() *GitService {
	manager := git.NewManager()

	s := &GitService{
		repositories: make(map[string]*models.Repository),
		worktrees:    make(map[string]*models.Worktree),
		manager:      manager,
		helper:       git.NewServiceHelper(), // NEW: Initialize helper
	}
	// ... rest of initialization
}
```

### 2. Replace Command Execution Methods

**Before (lines 162-188):**

```go
func (s *GitService) execGitCommand(workingDir string, args ...string) *exec.Cmd { ... }
func (s *GitService) runGitCommand(workingDir string, args ...string) ([]byte, error) { ... }
```

**After:**

```go
// Remove these methods and use:
// s.helper.ExecuteGit(workingDir, args...)
// s.helper.ExecuteCommand(command, args...)
```

### 3. Replace Branch Operations

**Before (lines 364-392):**

```go
func (s *GitService) branchExists(repoPath, branch string, isRemote bool) bool {
	return s.branchExistsWithOptions(repoPath, branch, BranchExistsOptions{
		IsRemote:   isRemote,
		RemoteName: "origin",
	})
}

func (s *GitService) branchExistsWithOptions(repoPath, branch string, opts BranchExistsOptions) bool {
	// ~20 lines of implementation
}
```

**After:**

```go
func (s *GitService) branchExists(repoPath, branch string, isRemote bool) bool {
	return s.helper.BranchExists(repoPath, branch, isRemote)
}

// Remove branchExistsWithOptions - it's now in git.BranchOperations
```

### 4. Replace Fetch Operations

**Before (lines 450-506):**

```go
func (s *GitService) fetchBranch(repoPath string, strategy FetchStrategy) error {
	// ~50 lines of complex fetch logic
}
```

**After:**

```go
func (s *GitService) fetchBranch(repoPath string, strategy git.FetchStrategy) error {
	return s.helper.FetchWithStrategy(repoPath, strategy)
}
```

### 5. Replace Status Checking

**Before (lines 508-562):**

```go
func (s *GitService) isDirty(worktreePath string) bool {
	// Implementation details
}

func (s *GitService) hasConflicts(worktreePath string) bool {
	// ~50 lines of conflict checking logic
}
```

**After:**

```go
func (s *GitService) isDirty(worktreePath string) bool {
	return s.helper.IsDirty(worktreePath)
}

func (s *GitService) hasConflicts(worktreePath string) bool {
	return s.helper.HasConflicts(worktreePath)
}
```

### 6. Replace URL Management

**Before (lines 191-258):**

```go
type RemoteURLManager struct {
	// Complex URL management with restoration
}
```

**After:**

```go
// Use s.helper.URLManager or s.helper.SetupRemoteURL() directly
// Remove the RemoteURLManager struct entirely
```

### 7. Simplify Push Operations

**Before (lines 270-340):**

```go
func (s *GitService) pushBranch(worktree *models.Worktree, repo *models.Repository, strategy PushStrategy) error {
	// ~70 lines of complex push logic with URL management
}
```

**After:**

```go
func (s *GitService) pushBranch(worktree *models.Worktree, repo *models.Repository, strategy git.PushStrategy) error {
	return s.helper.PushWithStrategy(worktree.Path, strategy)
}
```

## Benefits

1. **Reduced Complexity**: GitService drops from 2000+ lines to ~1200 lines
2. **Better Separation**: Git operations are properly abstracted
3. **Testability**: Individual components can be unit tested
4. **Reusability**: Git operations can be used by other services
5. **Maintainability**: Clear interfaces and single responsibility

## Migration Strategy

1. **Phase 1**: Add ServiceHelper to GitService (non-breaking)
2. **Phase 2**: Replace command execution methods
3. **Phase 3**: Replace branch operations
4. **Phase 4**: Replace fetch/push strategies
5. **Phase 5**: Replace status checking
6. **Phase 6**: Replace URL management
7. **Phase 7**: Remove deprecated methods

Each phase should be tested to ensure no regressions.
