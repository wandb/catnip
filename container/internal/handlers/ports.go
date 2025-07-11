package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/services"
)

// PortsHandler handles port-related API endpoints
type PortsHandler struct {
	monitor *services.PortMonitor
}

// NewPortsHandler creates a new ports handler
func NewPortsHandler(monitor *services.PortMonitor) *PortsHandler {
	return &PortsHandler{
		monitor: monitor,
	}
}

// GetPorts returns all detected ports and their service information
// @Summary Get detected ports
// @Description Returns a list of all currently detected ports with their service information
// @Tags ports
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "List of detected ports and services"
// @Router /v1/ports [get]
func (h *PortsHandler) GetPorts(c *fiber.Ctx) error {
	services := h.monitor.GetServices()
	
	// Convert to a more user-friendly format
	result := make(map[string]interface{})
	result["ports"] = services
	result["count"] = len(services)
	
	return c.JSON(result)
}

// GetPortInfo returns detailed information about a specific port
// @Summary Get port information
// @Description Returns detailed information about a specific port if it exists
// @Tags ports
// @Accept json
// @Produce json
// @Param port path int true "Port number"
// @Success 200 {object} services.ServiceInfo "Port information"
// @Failure 404 {object} map[string]string "Port not found"
// @Router /v1/ports/{port} [get]
func (h *PortsHandler) GetPortInfo(c *fiber.Ctx) error {
	port, err := c.ParamsInt("port")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid port number",
		})
	}
	
	services := h.monitor.GetServices()
	if service, exists := services[port]; exists {
		return c.JSON(service)
	}
	
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
		"error": "Port not found",
	})
}