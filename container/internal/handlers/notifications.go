package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/logger"
)

// NotificationEvent is already defined in events.go

// NotificationPayload represents a notification request
type NotificationPayload struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Subtitle string `json:"subtitle,omitempty"`
	URL      string `json:"url,omitempty"`
}

// NotificationHandler handles notification requests
type NotificationHandler struct {
	eventsHandler *EventsHandler
}

// NewNotificationHandler creates a new notification handler
func NewNotificationHandler(eventsHandler *EventsHandler) *NotificationHandler {
	return &NotificationHandler{
		eventsHandler: eventsHandler,
	}
}

// HandleNotification sends a notification event via SSE
// @Summary Send notification
// @Description Sends a notification event to all connected SSE clients, including the TUI app which can display native macOS notifications
// @Tags notifications
// @Accept json
// @Produce json
// @Param notification body NotificationPayload true "Notification details"
// @Success 200 {object} map[string]string "Success response"
// @Failure 400 {object} map[string]string "Bad request"
// @Router /v1/notifications [post]
func (h *NotificationHandler) HandleNotification(c *fiber.Ctx) error {
	var payload NotificationPayload

	if err := c.BodyParser(&payload); err != nil {
		logger.Warnf("Failed to parse notification request: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid notification payload",
		})
	}

	// Validate required fields
	if payload.Title == "" || payload.Body == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Title and body are required",
		})
	}

	// Broadcast notification event via SSE
	h.eventsHandler.broadcastEvent(AppEvent{
		Type: NotificationEvent,
		Payload: NotificationPayload{
			Title:    payload.Title,
			Body:     payload.Body,
			Subtitle: payload.Subtitle,
		},
	})

	logger.Infof("Notification sent: %s", payload.Title)

	return c.JSON(fiber.Map{
		"status": "sent",
	})
}
