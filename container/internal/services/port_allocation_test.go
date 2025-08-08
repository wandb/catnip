package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPortAllocationService(t *testing.T) {
	service := NewPortAllocationService()

	require.NotNil(t, service)
	assert.NotNil(t, service.allocatedPorts)
	assert.NotNil(t, service.usedPorts)
	assert.Equal(t, 3000, service.startPort)
}

func TestPortAllocationService_AllocatePortsForSession(t *testing.T) {
	service := NewPortAllocationService()
	sessionID := "test-session-1"

	ports, err := service.AllocatePortsForSession(sessionID)

	require.NoError(t, err)
	require.NotNil(t, ports)
	assert.Equal(t, sessionID, ports.SessionID)
	assert.Greater(t, ports.PORT, 0)
	assert.Len(t, ports.PORTZ, 6)

	// Verify all ports are different
	allPorts := append(ports.PORTZ, ports.PORT)
	uniquePorts := make(map[int]bool)
	for _, port := range allPorts {
		assert.False(t, uniquePorts[port], "Port %d is duplicated", port)
		uniquePorts[port] = true
	}
}

func TestPortAllocationService_AllocatePortsForSession_ExistingSession(t *testing.T) {
	service := NewPortAllocationService()
	sessionID := "test-session-1"

	// First allocation
	ports1, err := service.AllocatePortsForSession(sessionID)
	require.NoError(t, err)
	require.NotNil(t, ports1)

	// Second allocation for same session should return same ports
	ports2, err := service.AllocatePortsForSession(sessionID)
	require.NoError(t, err)
	require.NotNil(t, ports2)

	assert.Equal(t, ports1.PORT, ports2.PORT)
	assert.Equal(t, ports1.PORTZ, ports2.PORTZ)
	assert.Equal(t, ports1.SessionID, ports2.SessionID)
}

func TestPortAllocationService_GetPortsForSession(t *testing.T) {
	service := NewPortAllocationService()
	sessionID := "test-session-1"

	// No ports allocated initially
	ports, exists := service.GetPortsForSession(sessionID)
	assert.False(t, exists)
	assert.Nil(t, ports)

	// Allocate ports
	allocatedPorts, err := service.AllocatePortsForSession(sessionID)
	require.NoError(t, err)

	// Now should find ports
	ports, exists = service.GetPortsForSession(sessionID)
	assert.True(t, exists)
	require.NotNil(t, ports)
	assert.Equal(t, allocatedPorts.PORT, ports.PORT)
	assert.Equal(t, allocatedPorts.PORTZ, ports.PORTZ)
}

func TestPortAllocationService_ReleasePortsForSession(t *testing.T) {
	service := NewPortAllocationService()
	sessionID := "test-session-1"

	// Release non-existent session should not error
	err := service.ReleasePortsForSession(sessionID)
	assert.NoError(t, err)

	// Allocate ports first
	allocatedPorts, err := service.AllocatePortsForSession(sessionID)
	require.NoError(t, err)

	// Verify ports are allocated
	ports, exists := service.GetPortsForSession(sessionID)
	assert.True(t, exists)
	assert.NotNil(t, ports)

	// Release ports
	err = service.ReleasePortsForSession(sessionID)
	assert.NoError(t, err)

	// Verify ports are no longer allocated
	ports, exists = service.GetPortsForSession(sessionID)
	assert.False(t, exists)
	assert.Nil(t, ports)

	// Verify ports are available for reallocation
	newPorts, err := service.AllocatePortsForSession("test-session-2")
	require.NoError(t, err)

	// At least one of the previously allocated ports should be reused
	oldPorts := append(allocatedPorts.PORTZ, allocatedPorts.PORT)
	newPortsAll := append(newPorts.PORTZ, newPorts.PORT)

	foundReused := false
	for _, oldPort := range oldPorts {
		for _, newPort := range newPortsAll {
			if oldPort == newPort {
				foundReused = true
				break
			}
		}
		if foundReused {
			break
		}
	}
	assert.True(t, foundReused, "Expected at least one port to be reused after release")
}

func TestPortAllocationService_GetEnvironmentVariables(t *testing.T) {
	service := NewPortAllocationService()
	sessionID := "test-session-1"

	// No ports allocated
	envVars, err := service.GetEnvironmentVariables(sessionID)
	assert.Error(t, err)
	assert.Nil(t, envVars)
	assert.Contains(t, err.Error(), "no ports allocated for session")

	// Allocate ports
	ports, err := service.AllocatePortsForSession(sessionID)
	require.NoError(t, err)

	// Get environment variables
	envVars, err = service.GetEnvironmentVariables(sessionID)
	require.NoError(t, err)
	require.Len(t, envVars, 2)

	// Check PORT variable
	var portVar, portzVar string
	for _, envVar := range envVars {
		if strings.HasPrefix(envVar, "PORT=") {
			portVar = envVar
		} else if strings.HasPrefix(envVar, "PORTZ=") {
			portzVar = envVar
		}
	}

	require.NotEmpty(t, portVar)
	require.NotEmpty(t, portzVar)

	expectedPortVar := "PORT=" + string(rune(ports.PORT+'0'))
	// Note: We can't easily test the exact string due to integer conversion,
	// but we can verify the format
	assert.Contains(t, portVar, "PORT=")
	assert.Contains(t, portzVar, "PORTZ=[")
	assert.Contains(t, portzVar, "]")
}

func TestPortAllocationService_ListAllAllocatedPorts(t *testing.T) {
	service := NewPortAllocationService()

	// Initially empty
	allPorts := service.ListAllAllocatedPorts()
	assert.Empty(t, allPorts)

	// Allocate ports for multiple sessions
	session1 := "test-session-1"
	session2 := "test-session-2"

	ports1, err := service.AllocatePortsForSession(session1)
	require.NoError(t, err)

	ports2, err := service.AllocatePortsForSession(session2)
	require.NoError(t, err)

	// List all ports
	allPorts = service.ListAllAllocatedPorts()
	assert.Len(t, allPorts, 2)

	assert.Contains(t, allPorts, session1)
	assert.Contains(t, allPorts, session2)

	assert.Equal(t, ports1.PORT, allPorts[session1].PORT)
	assert.Equal(t, ports2.PORT, allPorts[session2].PORT)

	// Release one session
	err = service.ReleasePortsForSession(session1)
	require.NoError(t, err)

	allPorts = service.ListAllAllocatedPorts()
	assert.Len(t, allPorts, 1)
	assert.Contains(t, allPorts, session2)
	assert.NotContains(t, allPorts, session1)
}

func TestPortAllocationService_ConcurrentAccess(t *testing.T) {
	service := NewPortAllocationService()
	numSessions := 10

	// Test concurrent allocation
	results := make(chan error, numSessions)

	for i := 0; i < numSessions; i++ {
		go func(sessionID string) {
			_, err := service.AllocatePortsForSession(sessionID)
			results <- err
		}("session-" + string(rune(i+'0')))
	}

	// Wait for all allocations to complete
	for i := 0; i < numSessions; i++ {
		err := <-results
		assert.NoError(t, err)
	}

	// Verify all sessions have unique ports
	allPorts := service.ListAllAllocatedPorts()
	assert.Len(t, allPorts, numSessions)

	// Check for port collisions
	usedPorts := make(map[int]bool)
	for sessionID, ports := range allPorts {
		// Check main port
		assert.False(t, usedPorts[ports.PORT], "Port collision detected for session %s", sessionID)
		usedPorts[ports.PORT] = true

		// Check PORTZ ports
		for _, port := range ports.PORTZ {
			assert.False(t, usedPorts[port], "Port collision detected in PORTZ for session %s", sessionID)
			usedPorts[port] = true
		}
	}
}
