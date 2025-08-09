package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/services"
)

// PortsHandler handles port-related API endpoints
type PortsHandler struct {
	monitor *services.PortMonitor
	events  *EventsHandler
}

// NewPortsHandler creates a new ports handler
func NewPortsHandler(monitor *services.PortMonitor) *PortsHandler {
	return &PortsHandler{
		monitor: monitor,
	}
}

// WithEvents attaches an events handler for broadcasting mapping changes
func (h *PortsHandler) WithEvents(events *EventsHandler) *PortsHandler {
	h.events = events
	return h
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

// SetPortMapping sets or updates a host port mapping for a container port
// @Summary Set host port mapping for a container port
// @Description Records a mapping from container port to host port and broadcasts an SSE event
// @Tags ports
// @Accept json
// @Produce json
// @Param mapping body map[string]int true "Mapping object with 'port' and 'host_port'"
// @Success 200 {object} map[string]string "Mapping set"
// @Failure 400 {object} map[string]string "Invalid request"
// @Router /v1/ports/mappings [post]
func (h *PortsHandler) SetPortMapping(c *fiber.Ctx) error {
	if h.events == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "events handler not configured"})
	}

	var payload struct {
		Port     int `json:"port"`
		HostPort int `json:"host_port"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}
	if payload.Port <= 0 || payload.HostPort <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "port and host_port must be > 0"})
	}

	h.events.SetPortMapping(payload.Port, payload.HostPort)
	return c.JSON(fiber.Map{"status": "ok"})
}

// DeletePortMapping clears a host mapping for a container port
// @Summary Delete host port mapping for a container port
// @Description Removes a mapping and broadcasts an SSE event with host_port=0
// @Tags ports
// @Accept json
// @Produce json
// @Param port path int true "Container port"
// @Success 200 {object} map[string]string "Mapping deleted"
// @Failure 400 {object} map[string]string "Invalid port"
// @Router /v1/ports/mappings/{port} [delete]
func (h *PortsHandler) DeletePortMapping(c *fiber.Ctx) error {
	if h.events == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "events handler not configured"})
	}
	port, err := c.ParamsInt("port")
	if err != nil || port <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid port"})
	}
	h.events.ClearPortMapping(port)
	return c.JSON(fiber.Map{"status": "ok"})
}
