package git

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	// Cat names for branch generation (max 7 characters)
	catNames = []string{
		// Classic cat names
		"felix", "tom", "salem", "luna", "max", "oliver", "leo", "milo",
		"jack", "loki", "simba", "tigger", "smokey", "oscar", "toby", "george",
		"boots", "simon", "charlie", "jasper", "tiger", "shadow", "mittens",

		// Short and sweet names
		"oreo", "gizmo", "bandit", "muffin", "cookie", "pepper", "ziggy",
		"cosmo", "socks", "patches", "ginger", "rusty", "dusty", "midnight",

		// Playful names
		"noodle", "pickle", "taco", "bingo", "jinx", "pixel", "widget",
		"gadget", "fidget", "nugget", "ninja", "pirate", "scout", "rascal",

		// Regal names
		"duchess", "prince", "king", "baron", "duke", "earl", "lady",

		// Nature-inspired
		"willow", "storm", "misty", "sunny", "cloud", "rain", "snow",

		// Color-based
		"ebony", "ivory", "ash", "coal", "pearl", "ruby", "amber",

		// Food-inspired
		"mochi", "sushi", "nacho", "chip", "bean", "peanut", "olive",

		// Mythology/Magic
		"merlin", "thor", "zeus", "apollo", "magic", "mystic", "spirit",

		// Additional names to reach 50+
		"fluffy", "whisker", "paws", "velvet", "silk", "cotton", "fuzzy",
		"buddy", "chester", "dexter", "finn", "henry", "jasper", "murphy",
		"percy", "rocky", "teddy", "winston", "zigzag", "zorro", "ace",
	}

	// Cute adjectives for collision handling (max 5-6 chars to leave room for cat names)
	adjectives = []string{
		// Size descriptors
		"tiny", "little", "small", "mini", "big", "giant",

		// Personality traits
		"silly", "happy", "sleepy", "lazy", "brave", "smart",
		"wise", "sassy", "feisty", "calm", "wild", "shy",

		// Texture/Appearance
		"fuzzy", "fluffy", "soft", "silky", "shiny", "sleek",
		"furry", "puffy", "round", "chubby",

		// Endearing terms
		"sweet", "cute", "lovely", "pretty", "nice", "dear",
		"baby", "super", "mega", "ultra",

		// Playful descriptors
		"bouncy", "wiggly", "jumpy", "zippy", "perky", "peppy",

		// Color-ish (but short)
		"dark", "light", "bright", "pale", "misty",

		// Speed/Energy
		"fast", "quick", "swift", "speedy", "slow", "chill",

		// Temperature/Comfort
		"warm", "cozy", "cool", "toasty", "snug",

		// Magic/Special
		"magic", "cosmic", "astro", "dream", "fancy", "royal",
	}

	// Regex patterns
	githubURLPattern = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/\s]+?)(?:\.git)?(?:/|$)`)
	sshURLPattern    = regexp.MustCompile(`^(?:ssh://)?git@([^:]+):(.+)$`)
)

// GenerateSessionName creates a random branch name with format catnip/catname
func GenerateSessionName() string {
	catIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(catNames))))
	catName := catNames[catIndex.Int64()]
	return fmt.Sprintf("catnip/%s", catName)
}

// GenerateSessionNameWithAdjective creates a branch name with format catnip/adjective-catname
// Used for collision handling when simple cat names are taken
func GenerateSessionNameWithAdjective() string {
	catIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(catNames))))
	adjIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(adjectives))))
	catName := catNames[catIndex.Int64()]
	adjective := adjectives[adjIndex.Int64()]
	return fmt.Sprintf("catnip/%s-%s", adjective, catName)
}

// IsCatnipBranch checks if a branch name follows the catnip/catname or catnip/adjective-catname pattern
func IsCatnipBranch(branchName string) bool {
	// Check if it starts with catnip/ prefix
	if !strings.HasPrefix(branchName, "catnip/") {
		return false
	}

	// Extract the part after catnip/
	namePart := strings.TrimPrefix(branchName, "catnip/")

	// Check if it's a simple cat name
	for _, name := range catNames {
		if namePart == name {
			return true
		}
	}

	// Check if it's adjective-catname format
	if strings.Contains(namePart, "-") {
		parts := strings.SplitN(namePart, "-", 2)
		if len(parts) == 2 {
			adjective := parts[0]
			catName := parts[1]

			// Verify both parts are valid
			validAdj := false
			for _, adj := range adjectives {
				if adjective == adj {
					validAdj = true
					break
				}
			}

			validCat := false
			for _, cat := range catNames {
				if catName == cat {
					validCat = true
					break
				}
			}

			return validAdj && validCat
		}
	}

	return false
}

// ParseGitHubURL extracts owner and repo from a GitHub URL
func ParseGitHubURL(url string) (owner, repo string, err error) {
	// Handle SSH URLs
	if strings.HasPrefix(url, "git@") || strings.HasPrefix(url, "ssh://git@") {
		matches := sshURLPattern.FindStringSubmatch(url)
		if len(matches) > 2 && strings.Contains(matches[1], "github.com") {
			parts := strings.Split(matches[2], "/")
			if len(parts) == 2 {
				owner = parts[0]
				repo = strings.TrimSuffix(parts[1], ".git")
				return owner, repo, nil
			}
		}
	}

	// Handle HTTPS URLs
	matches := githubURLPattern.FindStringSubmatch(url)
	if len(matches) > 2 {
		owner = matches[1]
		repo = strings.TrimSuffix(matches[2], ".git")
		return owner, repo, nil
	}

	return "", "", fmt.Errorf("unable to parse GitHub URL: %s", url)
}

// ConvertSSHToHTTPS converts a Git SSH URL to HTTPS format
func ConvertSSHToHTTPS(url string) string {
	// Handle ssh://git@github.com/owner/repo.git format
	if strings.HasPrefix(url, "ssh://git@") {
		url = strings.TrimPrefix(url, "ssh://")
	}

	// Handle git@github.com:owner/repo.git format
	if strings.HasPrefix(url, "git@") {
		parts := strings.SplitN(url, ":", 2)
		if len(parts) == 2 {
			host := strings.TrimPrefix(parts[0], "git@")
			path := parts[1]
			return fmt.Sprintf("https://%s/%s", host, path)
		}
	}

	return url
}

// ExtractConflictFiles parses conflict information from git output
func ExtractConflictFiles(output string) []string {
	var files []string
	seen := make(map[string]bool)

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for merge conflict markers
		if strings.HasPrefix(line, "CONFLICT") {
			// Extract filename from CONFLICT messages
			if idx := strings.Index(line, " in "); idx != -1 {
				file := strings.TrimSpace(line[idx+4:])
				if !seen[file] {
					files = append(files, file)
					seen[file] = true
				}
			}
		}

		// Check for "both modified:" pattern
		if strings.Contains(line, "both modified:") {
			parts := strings.Split(line, "both modified:")
			if len(parts) > 1 {
				file := strings.TrimSpace(parts[1])
				if !seen[file] {
					files = append(files, file)
					seen[file] = true
				}
			}
		}

		// Check for "both added:" pattern
		if strings.Contains(line, "both added:") {
			parts := strings.Split(line, "both added:")
			if len(parts) > 1 {
				file := strings.TrimSpace(parts[1])
				if !seen[file] {
					files = append(files, file)
					seen[file] = true
				}
			}
		}
	}

	return files
}

// HasConflictMarkers checks if output contains Git conflict markers
func HasConflictMarkers(output string) bool {
	conflictPatterns := []string{
		"<<<<<<<",
		"=======",
		">>>>>>>",
		"CONFLICT",
		"Automatic merge failed",
		"fix conflicts and then commit",
	}

	for _, pattern := range conflictPatterns {
		if strings.Contains(output, pattern) {
			return true
		}
	}
	return false
}

// IsMergeConflict determines if an error indicates a merge conflict
func IsMergeConflict(output string) bool {
	conflictIndicators := []string{
		"CONFLICT",
		"Automatic merge failed",
		"fix conflicts and then commit the result",
		"Merge conflict in",
		"error: could not apply",
		"hint: after resolving the conflicts",
	}

	lowerOutput := strings.ToLower(output)
	for _, indicator := range conflictIndicators {
		if strings.Contains(lowerOutput, strings.ToLower(indicator)) {
			return true
		}
	}

	return false
}

// IsPushRejected checks if push was rejected due to upstream changes
func IsPushRejected(err error, output string) bool {
	if err == nil {
		return false
	}

	rejectionPatterns := []string{
		"[rejected]",
		"non-fast-forward",
		"fetch first",
		"Updates were rejected",
	}

	outputLower := strings.ToLower(output)
	for _, pattern := range rejectionPatterns {
		if strings.Contains(outputLower, strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// Contains checks if a string slice contains a specific item
func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GenerateUniqueSessionName generates a unique session name by checking branch existence
// It accepts a function that checks if a branch exists, allowing different implementations
func GenerateUniqueSessionName(branchExists func(string) bool) string {
	// First, try simple cat names
	maxSimpleAttempts := 20
	for i := 0; i < maxSimpleAttempts; i++ {
		name := GenerateSessionName()
		if !branchExists(name) {
			return name
		}
	}

	// If simple names are exhausted, try with adjectives
	maxAdjectiveAttempts := 50
	for i := 0; i < maxAdjectiveAttempts; i++ {
		name := GenerateSessionNameWithAdjective()
		if !branchExists(name) {
			return name
		}
	}

	// Final fallback: add timestamp to ensure uniqueness
	return fmt.Sprintf("catnip/special-%d", time.Now().Unix())
}

// ExtractWorkspaceName extracts the workspace-friendly name from a branch name
// For catnip branches, removes the "catnip/" prefix
// Examples: "catnip/felix" -> "felix", "catnip/fuzzy-luna" -> "fuzzy-luna"
func ExtractWorkspaceName(branchName string) string {
	if strings.HasPrefix(branchName, "catnip/") {
		return strings.TrimPrefix(branchName, "catnip/")
	}
	return branchName
}

// FindGitRoot finds the git repository root starting from the given directory
func FindGitRoot(startDir string) (string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false
	}

	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil {
			// Check if it's a directory (normal repo) or file (worktree)
			if info.IsDir() {
				return dir, true
			}
			// If it's a file, it might be a git worktree
			if content, err := os.ReadFile(gitDir); err == nil {
				// Git worktree file contains "gitdir: path/to/git/dir"
				if strings.HasPrefix(string(content), "gitdir: ") {
					return dir, true
				}
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root directory
			break
		}
		dir = parent
	}

	return "", false
}
