package cmd

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name           string
		version        string
		expectedBase   string
		expectedSuffix string
	}{
		{
			name:           "version with v prefix and dev suffix",
			version:        "v0.11.1-dev",
			expectedBase:   "0.11.1",
			expectedSuffix: "-dev",
		},
		{
			name:           "version without v prefix",
			version:        "0.11.2",
			expectedBase:   "0.11.2",
			expectedSuffix: "",
		},
		{
			name:           "version with v prefix no suffix",
			version:        "v1.2.3",
			expectedBase:   "1.2.3",
			expectedSuffix: "",
		},
		{
			name:           "version with rc suffix",
			version:        "v1.2.3-rc.1",
			expectedBase:   "1.2.3",
			expectedSuffix: "-rc.1",
		},
		{
			name:           "version with alpha suffix",
			version:        "0.5.0-alpha",
			expectedBase:   "0.5.0",
			expectedSuffix: "-alpha",
		},
		{
			name:           "version with beta and metadata",
			version:        "v2.0.0-beta.1+build.123",
			expectedBase:   "2.0.0",
			expectedSuffix: "-beta.1+build.123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, suffix := parseVersion(tt.version)
			if base != tt.expectedBase {
				t.Errorf("parseVersion(%q) base = %q, want %q", tt.version, base, tt.expectedBase)
			}
			if suffix != tt.expectedSuffix {
				t.Errorf("parseVersion(%q) suffix = %q, want %q", tt.version, suffix, tt.expectedSuffix)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
		wantErr  bool
	}{
		// Equal versions
		{
			name:     "exact match with v prefix",
			v1:       "v0.11.1",
			v2:       "v0.11.1",
			expected: 0,
		},
		{
			name:     "exact match without v prefix",
			v1:       "0.11.1",
			v2:       "0.11.1",
			expected: 0,
		},
		{
			name:     "mixed v prefix - same version",
			v1:       "0.11.1",
			v2:       "v0.11.1",
			expected: 0,
		},
		{
			name:     "dev version vs stable - same base",
			v1:       "0.11.1-dev",
			v2:       "v0.11.1",
			expected: 0,
		},
		{
			name:     "rc version vs stable - same base",
			v1:       "v1.2.3-rc.1",
			v2:       "1.2.3",
			expected: 0,
		},

		// v1 < v2 (upgrade needed)
		{
			name:     "patch version older",
			v1:       "0.11.0",
			v2:       "v0.11.1",
			expected: -1,
		},
		{
			name:     "minor version older",
			v1:       "0.10.5",
			v2:       "v0.11.0",
			expected: -1,
		},
		{
			name:     "major version older",
			v1:       "0.5.0",
			v2:       "v1.0.0",
			expected: -1,
		},
		{
			name:     "dev version older base",
			v1:       "0.11.0-dev",
			v2:       "v0.11.1",
			expected: -1,
		},

		// v1 > v2 (current is newer)
		{
			name:     "patch version newer",
			v1:       "0.11.2",
			v2:       "v0.11.1",
			expected: 1,
		},
		{
			name:     "minor version newer",
			v1:       "0.12.0",
			v2:       "v0.11.5",
			expected: 1,
		},
		{
			name:     "major version newer",
			v1:       "2.0.0",
			v2:       "v1.9.9",
			expected: 1,
		},
		{
			name:     "dev version newer base",
			v1:       "0.11.2-dev",
			v2:       "v0.11.1",
			expected: 1,
		},

		// Edge cases
		{
			name:     "two part version vs three part",
			v1:       "1.2",
			v2:       "v1.2.0",
			expected: 0,
		},
		{
			name:     "single digit versions",
			v1:       "1.0.0",
			v2:       "v2.0.0",
			expected: -1,
		},
		{
			name:     "large version numbers",
			v1:       "10.20.30",
			v2:       "v10.20.29",
			expected: 1,
		},

		// Error cases
		{
			name:    "invalid version - letters in v1",
			v1:      "v1.2.x",
			v2:      "v1.2.3",
			wantErr: true,
		},
		{
			name:    "invalid version - letters in v2",
			v1:      "v1.2.3",
			v2:      "v1.2.beta",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := compareVersions(tt.v1, tt.v2)

			if tt.wantErr {
				if err == nil {
					t.Errorf("compareVersions(%q, %q) expected error, got nil", tt.v1, tt.v2)
				}
				return
			}

			if err != nil {
				t.Errorf("compareVersions(%q, %q) unexpected error: %v", tt.v1, tt.v2, err)
				return
			}

			if result != tt.expected {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

// TestUpgradeDecisionScenarios tests real-world upgrade decision scenarios
func TestUpgradeDecisionScenarios(t *testing.T) {
	scenarios := []struct {
		name          string
		currentVer    string
		latestVer     string
		shouldUpgrade bool
		description   string
	}{
		{
			name:          "same version - skip upgrade",
			currentVer:    "0.11.1",
			latestVer:     "v0.11.1",
			shouldUpgrade: false,
			description:   "Already running the latest version",
		},
		{
			name:          "dev version with same base - skip upgrade",
			currentVer:    "0.11.1-dev",
			latestVer:     "v0.11.1",
			shouldUpgrade: false,
			description:   "Already running newer version (dev/pre-release)",
		},
		{
			name:          "dev version with newer base - skip upgrade",
			currentVer:    "0.11.2-dev",
			latestVer:     "v0.11.1",
			shouldUpgrade: false,
			description:   "Already running newer version (dev/pre-release)",
		},
		{
			name:          "older patch version - upgrade",
			currentVer:    "0.11.0",
			latestVer:     "v0.11.1",
			shouldUpgrade: true,
			description:   "Upgrade from 0.11.0 to v0.11.1",
		},
		{
			name:          "dev with older base - upgrade",
			currentVer:    "0.11.0-dev",
			latestVer:     "v0.11.1",
			shouldUpgrade: true,
			description:   "Upgrade from dev version to newer stable",
		},
		{
			name:          "newer version installed - skip upgrade",
			currentVer:    "0.12.0",
			latestVer:     "v0.11.5",
			shouldUpgrade: false,
			description:   "Already running newer version",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			comparison, err := compareVersions(sc.currentVer, sc.latestVer)
			if err != nil {
				t.Fatalf("compareVersions(%q, %q) error: %v", sc.currentVer, sc.latestVer, err)
			}

			// Mimic the upgrade decision logic from runUpgrade
			shouldUpgrade := comparison < 0

			if shouldUpgrade != sc.shouldUpgrade {
				t.Errorf("Scenario %q:\n  Current: %s, Latest: %s\n  Expected shouldUpgrade=%v, got %v (comparison=%d)\n  Description: %s",
					sc.name, sc.currentVer, sc.latestVer, sc.shouldUpgrade, shouldUpgrade, comparison, sc.description)
			}
		})
	}
}
