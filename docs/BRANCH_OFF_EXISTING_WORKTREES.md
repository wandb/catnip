# Branch Off Existing Worktrees Implementation Plan

## Overview

This document outlines the plan to implement a feature that allows users to create new worktrees by branching off existing worktrees, rather than always branching from the default base branch.

## Current State Analysis

### How Worktrees Are Currently Created

The current implementation in `container/internal/services/git.go` creates worktrees only from the default branch or a specified remote branch:

1. **CheckoutRepository** (`git.go:117-131`): Entry point for creating worktrees
   - Always uses the repository's default branch or a specified remote branch
   - Calls `createWorktreeForExistingRepo` with branch parameter
   - Never uses existing worktrees as source

2. **createWorktreeForExistingRepo** (`git.go:~420-450`): Creates worktree from remote branch
   - Fetches branch from remote if it doesn't exist locally
   - Creates worktree using `git worktree add -b <name> <path> <source>`
   - Source is always a remote branch reference

3. **createWorktreeInternalForRepo** (`git.go:~460-520`): Internal worktree creation
   - Uses `git worktree add -b <name> <path> <source>`
   - Source parameter is always a branch name or commit hash from remote
   - Sets `SourceBranch` field in worktree model

### Data Model

The `Worktree` model (`container/internal/models/git.go:46-77`) contains:

- `SourceBranch`: Currently tracks the remote branch this worktree branched from
- `Branch`: The actual branch name for this worktree
- `CommitHash`: Current commit hash
- `CommitCount`: Commits ahead of source branch

### Frontend Implementation

The frontend (`src/components/WorktreeRow.tsx`) displays:

- Source branch information (line 687-689)
- Worktree actions dropdown
- Currently no option to create new worktrees from existing ones

## Implementation Plan

### Phase 1: Backend API Changes

#### 1.1 Extend Worktree Creation API

**New Endpoint**: `POST /v1/git/worktrees`

```go
// WorktreeCreateRequest represents a request to create a new worktree
type WorktreeCreateRequest struct {
    Source     string `json:"source"`      // Branch name, commit hash, or worktree ID
    Name       string `json:"name"`        // User-friendly name (optional, generated if empty)
    SourceType string `json:"source_type"` // "branch", "commit", or "worktree"
}
```

#### 1.2 Update GitService Methods

**New Method**: `CreateWorktreeFromSource`

```go
func (s *GitService) CreateWorktreeFromSource(repoID string, req WorktreeCreateRequest) (*models.Worktree, error)
```

This method will:

1. Validate the source type and resolve the actual commit/branch
2. If source is a worktree ID, resolve to its current commit hash
3. Create the new worktree using the resolved source
4. Update the source branch tracking appropriately

#### 1.3 Source Resolution Logic

```go
func (s *GitService) resolveWorktreeSource(repoID string, source string, sourceType string) (resolvedSource string, sourceBranch string, err error) {
    switch sourceType {
    case "worktree":
        // Find the worktree by ID
        worktree, exists := s.worktrees[source]
        if !exists {
            return "", "", fmt.Errorf("worktree %s not found", source)
        }

        // Use the current commit hash as source
        return worktree.CommitHash, worktree.Branch, nil

    case "branch":
        // Use branch name directly
        return source, source, nil

    case "commit":
        // Use commit hash, try to resolve parent branch
        parentBranch := s.findParentBranch(repoID, source)
        return source, parentBranch, nil

    default:
        return "", "", fmt.Errorf("invalid source type: %s", sourceType)
    }
}
```

### Phase 2: Frontend UI Changes

#### 2.1 Add "Branch From This" Action

Modify `WorktreeActionDropdown` in `src/components/WorktreeRow.tsx`:

```tsx
<DropdownMenuItem
  onClick={() => onBranchFromWorktree(worktree.id, worktree.name)}
>
  <GitBranch size={16} />
  Branch from this worktree
</DropdownMenuItem>
```

#### 2.2 Create Branch Dialog

New component: `src/components/BranchFromWorktreeDialog.tsx`

```tsx
interface BranchFromWorktreeDialogProps {
  open: boolean;
  onClose: () => void;
  sourceWorktree: Worktree;
  onSubmit: (name: string) => void;
}
```

Features:

- Input field for new worktree name
- Display source worktree information
- Auto-generate name based on source + timestamp
- Show preview of what will be created

#### 2.3 Update Git State Management

Extend `src/hooks/useGitState.ts` to support:

- Creating worktrees from existing worktrees
- Tracking parent-child relationships
- Handling the new API endpoint

### Phase 3: Enhanced Features

#### 3.1 Worktree Lineage Tracking

Enhance the data model to track worktree relationships:

```go
type Worktree struct {
    // ... existing fields ...

    // Parent worktree ID if created from another worktree
    ParentWorktreeID *string `json:"parent_worktree_id,omitempty"`

    // Child worktree IDs created from this worktree
    ChildWorktreeIDs []string `json:"child_worktree_ids,omitempty"`
}
```

#### 3.2 Visual Lineage Display

Frontend enhancements:

- Tree view showing worktree relationships
- Visual indicators for parent/child relationships
- Breadcrumb navigation showing lineage path

#### 3.3 Cascade Operations

Smart operations that consider relationships:

- When deleting a parent worktree, offer to update children
- Sync operations that can propagate to children
- Merge operations that consider the full lineage

### Phase 4: Advanced Scenarios

#### 4.1 Cross-Repository Branching

Support branching from worktrees in different repositories:

- Validate compatibility
- Handle remote tracking differences
- Update source branch tracking appropriately

#### 4.2 Conflict Resolution

Enhanced conflict detection:

- Check for conflicts when branching from worktrees
- Provide warnings about potential issues
- Suggest alternative source points

#### 4.3 Performance Optimizations

- Lazy loading of worktree relationships
- Efficient lineage queries
- Caching of resolved sources

## Implementation Steps

### Step 1: Backend Foundation

1. Add new API endpoint for worktree creation
2. Implement source resolution logic
3. Update worktree creation to support existing worktrees as sources
4. Add proper error handling and validation

### Step 2: Basic Frontend Integration

1. Add "Branch from this" action to worktree dropdown
2. Create basic dialog for collecting new worktree name
3. Wire up API calls
4. Test basic functionality

### Step 3: Enhanced UI

1. Improve dialog with better UX
2. Add lineage tracking to data model
3. Display parent/child relationships
4. Add visual indicators

### Step 4: Advanced Features

1. Add cascade operations
2. Implement cross-repository support
3. Add performance optimizations
4. Comprehensive testing

## API Design

### New Endpoint

```
POST /v1/git/worktrees
Content-Type: application/json

{
  "source": "worktree-id-123",
  "source_type": "worktree",
  "name": "fix-user-auth"
}
```

### Response

```json
{
  "id": "new-worktree-id",
  "repo_id": "owner/repo",
  "name": "fix-user-auth",
  "branch": "fix-user-auth",
  "source_branch": "feature-login",
  "parent_worktree_id": "worktree-id-123",
  "commit_hash": "abc123def456",
  "path": "/workspace/repo/fix-user-auth",
  "created_at": "2024-01-15T14:00:00Z"
}
```

## Testing Strategy

### Unit Tests

- Source resolution logic
- Worktree creation from different source types
- Error handling for invalid sources

### Integration Tests

- Full workflow: create worktree from existing worktree
- API endpoint testing
- Database state validation

### Frontend Tests

- Dialog component behavior
- API integration
- User interaction flows

## Migration Strategy

### Backward Compatibility

- Existing worktrees continue to work unchanged
- Default behavior remains the same (branch from remote)
- New feature is additive, not replacing existing functionality

### Data Migration

- No migration needed for existing worktrees
- New fields are optional and nullable
- Lineage tracking starts from new worktrees created after feature deployment

## Risk Assessment

### Technical Risks

- **Complexity**: Adding worktree relationships increases system complexity
- **Performance**: Lineage queries could become slow with many worktrees
- **Conflicts**: Branching from uncommitted work could cause issues

### Mitigation Strategies

- Implement incrementally with fallbacks
- Add comprehensive validation
- Provide clear error messages
- Add performance monitoring

### User Experience Risks

- **Confusion**: Users might not understand the difference between source types
- **Mistakes**: Accidentally creating complex lineages

### Mitigation Strategies

- Clear UI labels and tooltips
- Confirmation dialogs for complex operations
- Ability to view and understand lineage

## Success Metrics

### Functional Metrics

- Users can successfully create worktrees from existing worktrees
- API response times remain under 200ms
- Error rates stay below 1%

### User Experience Metrics

- Feature adoption rate
- User satisfaction with the workflow
- Reduction in support tickets about worktree management

## Timeline

### Week 1-2: Backend Implementation

- API endpoint development
- Source resolution logic
- Basic testing

### Week 3-4: Frontend Integration

- UI components
- API integration
- Basic user testing

### Week 5-6: Enhanced Features

- Lineage tracking
- Visual improvements
- Advanced testing

### Week 7-8: Polish and Launch

- Performance optimization
- Documentation
- Production deployment

## Future Enhancements

### Potential Features

- Worktree templates for common branching patterns
- Automated cleanup of deep lineage chains
- Integration with PR workflows
- Branching policies and restrictions

### Long-term Vision

- Full worktree lifecycle management
- Advanced collaboration features
- Integration with CI/CD pipelines
- Analytics and insights about branching patterns

---

_This document will be updated as the implementation progresses and requirements evolve._
