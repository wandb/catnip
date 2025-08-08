package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

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

func TestPortsHandler_GetPortInfo_PortNotFound(t *testing.T) {
	monitor := services.NewPortMonitor()
	defer monitor.Stop()
	handler := NewPortsHandler(monitor)

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
}

func TestPortsHandler_GetPortInfo_InvalidPort(t *testing.T) {
	monitor := services.NewPortMonitor()
	defer monitor.Stop()
	handler := NewPortsHandler(monitor)

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
}
