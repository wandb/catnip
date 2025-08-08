package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vanpelt/catnip/internal/services"
)

func TestNewPortsHandler(t *testing.T) {
	monitor := services.NewPortMonitor()
	defer monitor.Stop()
	handler := NewPortsHandler(monitor)

	require.NotNil(t, handler)
	assert.NotNil(t, handler.monitor)
}

func TestPortsHandler_GetPorts_EmptyServices(t *testing.T) {
	monitor := services.NewPortMonitor()
	defer monitor.Stop()
	handler := NewPortsHandler(monitor)

	app := fiber.New()
	app.Get("/ports", handler.GetPorts)

	req := httptest.NewRequest("GET", "/ports", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, float64(0), response["count"])
	assert.NotNil(t, response["ports"])
}

	app := fiber.New()
	app.Get("/ports", handler.GetPorts)

	req := httptest.NewRequest("GET", "/ports", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, float64(2), response["count"])
	assert.NotNil(t, response["ports"])

	// Verify the ports data structure
	ports, ok := response["ports"].(map[string]interface{})
	require.True(t, ok)

	// Note: JSON unmarshaling converts keys to strings, so we check for "3000" not 3000
	port3000, exists := ports["3000"]
	assert.True(t, exists)

	port3000Data, ok := port3000.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "React Dev Server", port3000Data["Name"])

	mockMonitor.AssertExpectations(t)
}

func TestPortsHandler_GetPorts_EmptyServices(t *testing.T) {
	mockMonitor := new(MockPortMonitor)
	handler := NewPortsHandler(mockMonitor)

	// Mock empty services
	mockServices := map[int]*services.ServiceInfo{}
	mockMonitor.On("GetServices").Return(mockServices)

	app := fiber.New()
	app.Get("/ports", handler.GetPorts)

	req := httptest.NewRequest("GET", "/ports", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, float64(0), response["count"])
	assert.NotNil(t, response["ports"])

	mockMonitor.AssertExpectations(t)
}

func TestPortsHandler_GetPortInfo_ValidPort(t *testing.T) {
	mockMonitor := new(MockPortMonitor)
	handler := NewPortsHandler(mockMonitor)

	// Mock service data
	mockServices := map[int]*services.ServiceInfo{
		3000: {
			Port:        3000,
			Name:        "React Dev Server",
			Framework:   "React",
			ProcessName: "npm",
			LastSeen:    "2024-01-15T10:30:00Z",
		},
	}

	mockMonitor.On("GetServices").Return(mockServices)

	app := fiber.New()
	app.Get("/ports/:port", handler.GetPortInfo)

	req := httptest.NewRequest("GET", "/ports/3000", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var response services.ServiceInfo
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, 3000, response.Port)
	assert.Equal(t, "React Dev Server", response.Name)
	assert.Equal(t, "React", response.Framework)

	mockMonitor.AssertExpectations(t)
}

func TestPortsHandler_GetPortInfo_PortNotFound(t *testing.T) {
	mockMonitor := new(MockPortMonitor)
	handler := NewPortsHandler(mockMonitor)

	// Mock service data without port 9999
	mockServices := map[int]*services.ServiceInfo{
		3000: {
			Port:        3000,
			Name:        "React Dev Server",
			Framework:   "React",
			ProcessName: "npm",
			LastSeen:    "2024-01-15T10:30:00Z",
		},
	}

	mockMonitor.On("GetServices").Return(mockServices)

	app := fiber.New()
	app.Get("/ports/:port", handler.GetPortInfo)

	req := httptest.NewRequest("GET", "/ports/9999", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)

	var response map[string]string
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "Port not found", response["error"])

	mockMonitor.AssertExpectations(t)
}

func TestPortsHandler_GetPortInfo_InvalidPort(t *testing.T) {
	mockMonitor := new(MockPortMonitor)
	handler := NewPortsHandler(mockMonitor)

	app := fiber.New()
	app.Get("/ports/:port", handler.GetPortInfo)

	req := httptest.NewRequest("GET", "/ports/invalid", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)

	var response map[string]string
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "Invalid port number", response["error"])

	// No expectations to assert since GetServices wasn't called
}
