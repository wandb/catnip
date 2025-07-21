package git

import (
	"fmt"
	"strings"

	"github.com/go-git/go-billy/v5"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage"
)

// GoGitRepository implements Repository interface using go-git
type GoGitRepository struct {
	repo    *gogit.Repository
	storage storage.Storer
	fs      billy.Filesystem
	path    string
}

// NewGoGitRepository creates a new go-git repository
func NewGoGitRepository(storage storage.Storer, fs billy.Filesystem, path string) Repository {
	return &GoGitRepository{
		storage: storage,
		fs:      fs,
		path:    path,
	}
}

// NewGoGitRepositoryFromExisting creates a repository from an existing go-git repo
func NewGoGitRepositoryFromExisting(repo *gogit.Repository, path string) Repository {
	return &GoGitRepository{
		repo: repo,
		path: path,
	}
}

// Clone clones a repository from a URL
func (r *GoGitRepository) Clone(url, path string) error {
	repo, err := gogit.Clone(r.storage, r.fs, &gogit.CloneOptions{
		URL: url,
	})
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	r.repo = repo
	r.path = path
	return nil
}

// GetPath returns the repository path
func (r *GoGitRepository) GetPath() string {
	return r.path
}

// GetRemoteURL returns the remote origin URL
func (r *GoGitRepository) GetRemoteURL() (string, error) {
	if r.repo == nil {
		return "", fmt.Errorf("repository not initialized")
	}

	remote, err := r.repo.Remote("origin")
	if err != nil {
		return "", fmt.Errorf("failed to get remote: %w", err)
	}

	if len(remote.Config().URLs) == 0 {
		return "", fmt.Errorf("no URLs configured for origin remote")
	}

	return remote.Config().URLs[0], nil
}

// GetDefaultBranch returns the default branch
func (r *GoGitRepository) GetDefaultBranch() (string, error) {
	if r.repo == nil {
		return "", fmt.Errorf("repository not initialized")
	}

	// Try to get HEAD reference
	head, err := r.repo.Head()
	if err != nil {
		return "main", nil // Default fallback
	}

	if head.Name().IsBranch() {
		return head.Name().Short(), nil
	}

	// Try to get default branch from remote
	remote, err := r.repo.Remote("origin")
	if err == nil {
		refs, err := remote.List(&gogit.ListOptions{})
		if err == nil {
			for _, ref := range refs {
				if ref.Name() == plumbing.ReferenceName("refs/heads/main") {
					return "main", nil
				}
				if ref.Name() == plumbing.ReferenceName("refs/heads/master") {
					return "master", nil
				}
			}
		}
	}

	return "main", nil
}

// ListBranches returns all branches
func (r *GoGitRepository) ListBranches() ([]string, error) {
	if r.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	refs, err := r.repo.References()
	if err != nil {
		return nil, fmt.Errorf("failed to get references: %w", err)
	}

	var branches []string
	seen := make(map[string]bool)

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name()

		// Handle local branches
		if name.IsBranch() {
			branch := name.Short()
			if !seen[branch] {
				branches = append(branches, branch)
				seen[branch] = true
			}
		}

		// Handle remote branches
		if name.IsRemote() {
			parts := strings.Split(name.Short(), "/")
			if len(parts) >= 2 && parts[0] == "origin" {
				branch := strings.Join(parts[1:], "/")
				if !seen[branch] && branch != "HEAD" {
					branches = append(branches, branch)
					seen[branch] = true
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to iterate references: %w", err)
	}

	return branches, nil
}

// BranchExists checks if a branch exists
func (r *GoGitRepository) BranchExists(branch string) bool {
	if r.repo == nil {
		return false
	}

	// Check local branch
	_, err := r.repo.Reference(plumbing.ReferenceName("refs/heads/"+branch), true)
	if err == nil {
		return true
	}

	// Check remote branch
	_, err = r.repo.Reference(plumbing.ReferenceName("refs/remotes/origin/"+branch), true)
	return err == nil
}

// CreateBranch creates a new branch
func (r *GoGitRepository) CreateBranch(branch, from string) error {
	if r.repo == nil {
		return fmt.Errorf("repository not initialized")
	}

	// Get the commit to branch from
	var hash plumbing.Hash
	if from != "" {
		ref, err := r.repo.Reference(plumbing.ReferenceName("refs/heads/"+from), true)
		if err != nil {
			// Try remote branch
			ref, err = r.repo.Reference(plumbing.ReferenceName("refs/remotes/origin/"+from), true)
			if err != nil {
				return fmt.Errorf("failed to find source branch %s: %w", from, err)
			}
		}
		hash = ref.Hash()
	} else {
		// Use HEAD
		head, err := r.repo.Head()
		if err != nil {
			return fmt.Errorf("failed to get HEAD: %w", err)
		}
		hash = head.Hash()
	}

	// Create the new branch reference
	ref := plumbing.NewHashReference(plumbing.ReferenceName("refs/heads/"+branch), hash)
	err := r.repo.Storer.SetReference(ref)
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %w", branch, err)
	}

	return nil
}

// Fetch fetches all references from remote
func (r *GoGitRepository) Fetch() error {
	if r.repo == nil {
		return fmt.Errorf("repository not initialized")
	}

	err := r.repo.Fetch(&gogit.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{"refs/*:refs/*"},
	})

	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch: %w", err)
	}

	return nil
}

// FetchBranch fetches a specific branch
func (r *GoGitRepository) FetchBranch(branch string) error {
	if r.repo == nil {
		return fmt.Errorf("repository not initialized")
	}

	refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/remotes/origin/%s", branch, branch))
	err := r.repo.Fetch(&gogit.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refSpec},
	})

	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch branch %s: %w", branch, err)
	}

	return nil
}

// FetchWithDepth fetches with a specific depth
func (r *GoGitRepository) FetchWithDepth(branch string, depth int) error {
	if r.repo == nil {
		return fmt.Errorf("repository not initialized")
	}

	refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/remotes/origin/%s", branch, branch))
	err := r.repo.Fetch(&gogit.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refSpec},
		Depth:      depth,
	})

	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch branch %s with depth %d: %w", branch, depth, err)
	}

	return nil
}

// IsBare checks if repository is bare
func (r *GoGitRepository) IsBare() bool {
	if r.repo == nil {
		return false
	}

	// go-git repositories with nil filesystem are considered bare
	return r.fs == nil
}

// IsShallow checks if repository is shallow
func (r *GoGitRepository) IsShallow() bool {
	if r.repo == nil {
		return false
	}

	// Check if there are shallow commits
	shallows, err := r.repo.Storer.Shallow()
	if err != nil {
		return false
	}

	return len(shallows) > 0
}

// Unshallow converts shallow repository to complete
func (r *GoGitRepository) Unshallow() error {
	if r.repo == nil {
		return fmt.Errorf("repository not initialized")
	}

	if !r.IsShallow() {
		return nil
	}

	// Fetch with no depth limit to unshallow
	err := r.repo.Fetch(&gogit.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{"refs/*:refs/*"},
		Depth:      0,
	})

	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to unshallow repository: %w", err)
	}

	return nil
}
