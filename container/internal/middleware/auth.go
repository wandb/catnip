package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/vanpelt/catnip/internal/logger"
)

type Claims struct {
	Source    string `json:"source"` // "cli" or "browser"
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

type AuthMiddleware struct {
	secret []byte
}

// NewAuthMiddleware creates a new auth middleware instance
func NewAuthMiddleware() *AuthMiddleware {
	secret := os.Getenv("CATNIP_AUTH_SECRET")
	if secret == "" {
		return nil // No auth required
	}
	return &AuthMiddleware{
		secret: []byte(secret),
	}
}

// RequireAuth is a middleware that checks for valid authentication
func (am *AuthMiddleware) RequireAuth(c *fiber.Ctx) error {
	// If no auth middleware (no secret), pass through
	if am == nil {
		return c.Next()
	}

	// Skip auth for health check and settings endpoints
	path := c.Path()
	if path == "/health" || path == "/v1/settings" || path == "/v1/auth/token" {
		return c.Next()
	}

	// Try to get token from various sources
	token := am.extractToken(c)
	if token == "" {
		return c.Status(401).JSON(fiber.Map{
			"error": "authentication required",
		})
	}

	// Validate token
	claims, err := am.ValidateToken(token)
	if err != nil {
		logger.Debugf("Auth failed: %v", err)
		return c.Status(401).JSON(fiber.Map{
			"error": "invalid or expired token",
		})
	}

	// Store claims in context for later use
	c.Locals("claims", claims)
	return c.Next()
}

// extractToken tries to get the token from various sources
func (am *AuthMiddleware) extractToken(c *fiber.Ctx) string {
	// 1. Try Authorization header
	authHeader := c.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return parts[1]
		}
	}

	// 2. Try cookie
	if cookie := c.Cookies("catnip_token"); cookie != "" {
		return cookie
	}

	// 3. Try query parameter (for initial browser handoff)
	if token := c.Query("token"); token != "" {
		return token
	}

	return ""
}

// ValidateToken validates the JWT token (exported for use in handlers)
func (am *AuthMiddleware) ValidateToken(tokenString string) (*Claims, error) {
	// Split token into parts
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Decode header and payload (we don't need to parse the header for validation)
	_, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("failed to decode header: %w", err)
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	// Parse claims
	var claims Claims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	// Check expiration
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("token expired")
	}

	// Verify signature
	signatureInput := parts[0] + "." + parts[1]
	h := hmac.New(sha256.New, am.secret)
	h.Write([]byte(signatureInput))
	expectedSignature := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	if expectedSignature != parts[2] {
		return nil, fmt.Errorf("invalid signature")
	}

	return &claims, nil
}

// GenerateToken generates a new JWT token
func GenerateToken(source string, duration time.Duration) (string, error) {
	secret := os.Getenv("CATNIP_AUTH_SECRET")
	if secret == "" {
		return "", fmt.Errorf("CATNIP_AUTH_SECRET not set")
	}

	now := time.Now()
	claims := Claims{
		Source:    source,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(duration).Unix(),
	}

	// Create header
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	// Encode header and claims
	headerEncoded := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsEncoded := base64.RawURLEncoding.EncodeToString(claimsJSON)

	// Create signature
	signatureInput := headerEncoded + "." + claimsEncoded
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signatureInput))
	signature := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	// Combine all parts
	token := headerEncoded + "." + claimsEncoded + "." + signature
	return token, nil
}
