//go:build !darwin

package macos

import (
	"github.com/vanpelt/catnip/internal/logger"
)

// PowerAssertion is a no-op implementation for non-macOS platforms
type PowerAssertion struct {
	reason string
}

// NewPowerAssertion creates a no-op power assertion on non-macOS platforms
func NewPowerAssertion(reason string) (*PowerAssertion, error) {
	logger.Debugf("🔋 Power assertions not supported on this platform: %s", reason)
	return &PowerAssertion{reason: reason}, nil
}

// Release is a no-op on non-macOS platforms
func (p *PowerAssertion) Release() error {
	logger.Debugf("🔋 Power assertion release (no-op): %s", p.reason)
	return nil
}

// IsActive always returns false on non-macOS platforms
func (p *PowerAssertion) IsActive() bool {
	return false
}

// GetReason returns the reason string
func (p *PowerAssertion) GetReason() string {
	return p.reason
}
