//go:build darwin

package macos

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <IOKit/pwr_mgt/IOPMLib.h>
#include <CoreFoundation/CoreFoundation.h>
*/
import "C"
import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/vanpelt/catnip/internal/logger"
)

// PowerAssertion manages macOS power assertions to prevent sleep
type PowerAssertion struct {
	assertionID C.IOPMAssertionID
	active      bool
	reason      string
}

// NewPowerAssertion creates a new power assertion to prevent system sleep
// This prevents the display from sleeping and the system from going to idle sleep
func NewPowerAssertion(reason string) (*PowerAssertion, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("power assertions are only supported on macOS")
	}

	// Convert Go string to CFString
	reasonCStr := C.CString(reason)
	defer C.free(unsafe.Pointer(reasonCStr))

	reasonCF := C.CFStringCreateWithCString(
		C.kCFAllocatorDefault,
		reasonCStr,
		C.kCFStringEncodingUTF8,
	)
	defer C.CFRelease(C.CFTypeRef(reasonCF))

	var assertionID C.IOPMAssertionID

	// Create assertion that prevents both display sleep and system idle sleep
	// kIOPMAssertionTypeNoDisplaySleep prevents display from sleeping
	// kIOPMAssertionTypeNoIdleSleep prevents system from going to sleep when idle
	result := C.IOPMAssertionCreateWithName(
		C.kIOPMAssertionTypeNoIdleSleep, // Prevents system sleep when idle
		C.kIOPMAssertionLevelOn,
		reasonCF,
		&assertionID,
	)

	if result != C.kIOReturnSuccess {
		return nil, fmt.Errorf("failed to create power assertion: IOReturn %d", result)
	}

	logger.Infof("🔋 Created macOS power assertion (ID: %d): %s", assertionID, reason)

	return &PowerAssertion{
		assertionID: assertionID,
		active:      true,
		reason:      reason,
	}, nil
}

// Release releases the power assertion, allowing the system to sleep normally
func (p *PowerAssertion) Release() error {
	if !p.active {
		return nil // Already released
	}

	result := C.IOPMAssertionRelease(p.assertionID)
	if result != C.kIOReturnSuccess {
		return fmt.Errorf("failed to release power assertion: IOReturn %d", result)
	}

	logger.Infof("🔋 Released macOS power assertion (ID: %d): %s", p.assertionID, p.reason)
	p.active = false
	return nil
}

// IsActive returns whether the power assertion is currently active
func (p *PowerAssertion) IsActive() bool {
	return p.active
}

// GetReason returns the reason string for this power assertion
func (p *PowerAssertion) GetReason() string {
	return p.reason
}
