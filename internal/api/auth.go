package api

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	} `json:"user"`
}

// login handles user authentication
func (s *Server) login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Get user from database
	var id, email, name, role, passwordHash string
	err := s.db.QueryRow(`
		SELECT id, email, name, role, password_hash
		FROM users
		WHERE email = $1 AND active = true
	`, req.Email).Scan(&id, &email, &name, &role, &passwordHash)

	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid credentials"})
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid credentials"})
	}

	// Generate JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":    id,
		"email": email,
		"role":  role,
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenString, err := token.SignedString([]byte(s.cfg.APISecret))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
	}

	return c.JSON(LoginResponse{
		Token: tokenString,
		User: struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Name  string `json:"name"`
			Role  string `json:"role"`
		}{
			ID:    id,
			Email: email,
			Name:  name,
			Role:  role,
		},
	})
}

// logout handles user logout
func (s *Server) logout(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"message": "Logged out successfully"})
}

// authMiddleware validates JWT tokens
func (s *Server) authMiddleware(c *fiber.Ctx) error {
	var tokenString string

	// Try Authorization header first
	authHeader := c.Get("Authorization")
	if authHeader != "" {
		// Extract token from "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && parts[0] == "Bearer" {
			tokenString = parts[1]
		}
	}

	// Fallback to query param (for downloads)
	if tokenString == "" {
		tokenString = c.Query("token")
	}

	if tokenString == "" {
		return c.Status(401).JSON(fiber.Map{"error": "Missing authorization"})
	}

	// Parse and validate token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fiber.NewError(401, "Invalid token")
		}
		return []byte(s.cfg.APISecret), nil
	})

	if err != nil || !token.Valid {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid token"})
	}

	// Store user info in context
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		c.Locals("userId", claims["id"])
		c.Locals("userEmail", claims["email"])
		c.Locals("userRole", claims["role"])
	}

	return c.Next()
}

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	return string(bytes), err
}

// getUserID gets the user ID from context as a string
func getUserID(c *fiber.Ctx) string {
	if id := c.Locals("userId"); id != nil {
		if strID, ok := id.(string); ok {
			return strID
		}
	}
	return ""
}

// isAdmin checks if the current user is an admin
func isAdmin(c *fiber.Ctx) bool {
	if role := c.Locals("userRole"); role != nil {
		if strRole, ok := role.(string); ok {
			return strRole == "admin"
		}
	}
	return false
}

// resetAdmin creates first admin user (only works when no users exist)
func (s *Server) resetAdmin(c *fiber.Ctx) error {
	// SECURITY: Only allow if NO users exist in the database
	var userCount int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&userCount)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to check users"})
	}

	if userCount > 0 {
		return c.Status(403).JSON(fiber.Map{
			"error": "Setup already completed. Users already exist in the system.",
		})
	}

	// Generate hash for admin123
	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), 10)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate hash"})
	}

	// Create first admin user
	_, err = s.db.Exec(`
		INSERT INTO users (id, email, password_hash, name, role, active)
		VALUES (gen_random_uuid(), 'admin@admin.com', $1, 'Admin', 'admin', true)
	`, string(hash))

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message":  "Admin user created successfully! Please change the password after login.",
		"email":    "admin@admin.com",
		"password": "admin123",
	})
}
