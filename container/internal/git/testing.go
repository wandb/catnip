package git

import (
	"fmt"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

// TestRepository provides utilities for creating in-memory Git repositories for testing
type TestRepository struct {
	repo    *git.Repository
	storage *memory.Storage
	fs      billy.Filesystem
	path    string
}

// NewTestRepository creates a new in-memory repository for testing
func NewTestRepository(path string) (*TestRepository, error) {
	storage := memory.NewStorage()
	fs := memfs.New()

	repo, err := git.Init(storage, fs)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize test repository: %w", err)
	}

	return &TestRepository{
		repo:    repo,
		storage: storage,
		fs:      fs,
		path:    path,
	}, nil
}

// CloneTestRepository clones a repository into memory for testing
func CloneTestRepository(url, path string) (*TestRepository, error) {
	storage := memory.NewStorage()
	fs := memfs.New()

	repo, err := git.Clone(storage, fs, &git.CloneOptions{
		URL: url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to clone test repository: %w", err)
	}

	return &TestRepository{
		repo:    repo,
		storage: storage,
		fs:      fs,
		path:    path,
	}, nil
}

// GetRepository returns the underlying go-git repository
func (tr *TestRepository) GetRepository() *git.Repository {
	return tr.repo
}

// GetFilesystem returns the in-memory filesystem
func (tr *TestRepository) GetFilesystem() billy.Filesystem {
	return tr.fs
}

// CreateFile creates a file with the given content
func (tr *TestRepository) CreateFile(filename, content string) error {
	file, err := tr.fs.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filename, err)
	}
	defer file.Close()

	_, err = file.Write([]byte(content))
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", filename, err)
	}

	return nil
}

// CommitFile creates a file and commits it
func (tr *TestRepository) CommitFile(filename, content, message string) error {
	// Create the file
	err := tr.CreateFile(filename, content)
	if err != nil {
		return err
	}

	// Get worktree
	worktree, err := tr.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Add file
	_, err = worktree.Add(filename)
	if err != nil {
		return fmt.Errorf("failed to add file %s: %w", filename, err)
	}

	// Commit
	_, err = worktree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// CreateBranch creates a new branch from the current HEAD
func (tr *TestRepository) CreateBranch(branchName string) error {
	// Get current HEAD
	head, err := tr.repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Create branch reference
	ref := plumbing.NewHashReference(plumbing.ReferenceName("refs/heads/"+branchName), head.Hash())
	err = tr.repo.Storer.SetReference(ref)
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %w", branchName, err)
	}

	return nil
}

// CheckoutBranch checks out a branch
func (tr *TestRepository) CheckoutBranch(branchName string) error {
	worktree, err := tr.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName("refs/heads/" + branchName),
	})
	if err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w", branchName, err)
	}

	return nil
}

// RenameBranch renames a branch from oldName to newName
func (tr *TestRepository) RenameBranch(oldName, newName string) error {
	// Get the current HEAD of the old branch
	oldRef, err := tr.repo.Reference(plumbing.ReferenceName("refs/heads/"+oldName), true)
	if err != nil {
		return fmt.Errorf("failed to get reference for branch %s: %w", oldName, err)
	}

	// Create new branch reference pointing to the same hash
	newRef := plumbing.NewHashReference(plumbing.ReferenceName("refs/heads/"+newName), oldRef.Hash())
	err = tr.repo.Storer.SetReference(newRef)
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %w", newName, err)
	}

	// Update HEAD if we're renaming the current branch
	head, err := tr.repo.Head()
	if err == nil && head.Name() == plumbing.ReferenceName("refs/heads/"+oldName) {
		// Update HEAD to point to the new branch
		headRef := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/"+newName))
		err = tr.repo.Storer.SetReference(headRef)
		if err != nil {
			return fmt.Errorf("failed to update HEAD to new branch %s: %w", newName, err)
		}
	}

	// Remove the old branch reference
	err = tr.repo.Storer.RemoveReference(plumbing.ReferenceName("refs/heads/" + oldName))
	if err != nil {
		return fmt.Errorf("failed to remove old branch %s: %w", oldName, err)
	}

	return nil
}

// AddRemote adds a remote to the repository
func (tr *TestRepository) AddRemote(name, url string) error {
	_, err := tr.repo.CreateRemote(&config.RemoteConfig{
		Name: name,
		URLs: []string{url},
	})
	if err != nil {
		return fmt.Errorf("failed to add remote %s: %w", name, err)
	}

	return nil
}

// ToRepository converts the test repository to our Repository interface
func (tr *TestRepository) ToRepository() Repository {
	return NewGoGitRepositoryFromExisting(tr.repo, tr.path)
}

// ToWorktree converts the test repository to our Worktree interface
func (tr *TestRepository) ToWorktree(branch string) (Worktree, error) {
	return NewGoGitWorktree(tr.repo, tr.path, branch)
}

// CreateTestRepositoryWithHistory creates a repository with sample commit history
func CreateTestRepositoryWithHistory() (*TestRepository, error) {
	repo, err := NewTestRepository("/test/repo")
	if err != nil {
		return nil, err
	}

	// Create initial commit on master (go-git default)
	err = repo.CommitFile("README.md", "# Test Repository\n\nThis is a test.", "Initial commit")
	if err != nil {
		return nil, err
	}

	// Rename master branch to main
	err = repo.RenameBranch("master", "main")
	if err != nil {
		return nil, err
	}

	// Create a feature branch
	err = repo.CreateBranch("feature/test")
	if err != nil {
		return nil, err
	}

	err = repo.CheckoutBranch("feature/test")
	if err != nil {
		return nil, err
	}

	// Add commits to feature branch
	err = repo.CommitFile("feature.txt", "New feature implementation", "Add new feature")
	if err != nil {
		return nil, err
	}

	err = repo.CommitFile("test.txt", "Test file content", "Add test file")
	if err != nil {
		return nil, err
	}

	// Switch back to main
	err = repo.CheckoutBranch("main")
	if err != nil {
		return nil, err
	}

	// Add another commit to main
	err = repo.CommitFile("main.txt", "Main branch changes", "Update main branch")
	if err != nil {
		return nil, err
	}

	// Add remote
	err = repo.AddRemote("origin", "https://github.com/test/repo.git")
	if err != nil {
		return nil, err
	}

	return repo, nil
}

// CreateTestRepositoryWithConflicts creates a repository set up for merge conflicts
func CreateTestRepositoryWithConflicts() (*TestRepository, error) {
	repo, err := NewTestRepository("/test/conflicts")
	if err != nil {
		return nil, err
	}

	// Create initial file
	err = repo.CommitFile("conflict.txt", "line 1\nline 2\nline 3\n", "Initial commit")
	if err != nil {
		return nil, err
	}

	// Rename master branch to main
	err = repo.RenameBranch("master", "main")
	if err != nil {
		return nil, err
	}

	// Create branch A
	err = repo.CreateBranch("branch-a")
	if err != nil {
		return nil, err
	}

	err = repo.CheckoutBranch("branch-a")
	if err != nil {
		return nil, err
	}

	// Modify file in branch A
	err = repo.CommitFile("conflict.txt", "line 1 modified\nline 2\nline 3\n", "Modify line 1 in branch A")
	if err != nil {
		return nil, err
	}

	// Switch back to main and create branch B
	err = repo.CheckoutBranch("main")
	if err != nil {
		return nil, err
	}

	err = repo.CreateBranch("branch-b")
	if err != nil {
		return nil, err
	}

	err = repo.CheckoutBranch("branch-b")
	if err != nil {
		return nil, err
	}

	// Modify same line in branch B (this will cause conflict)
	err = repo.CommitFile("conflict.txt", "line 1 different modification\nline 2\nline 3\n", "Modify line 1 in branch B")
	if err != nil {
		return nil, err
	}

	return repo, nil
}
