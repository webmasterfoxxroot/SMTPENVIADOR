package api

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

// User represents a user in the system
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateUserRequest represents the request to create a user
type CreateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Role     string `json:"role"`
}

// UpdateUserRequest represents the request to update a user
type UpdateUserRequest struct {
	Email  string `json:"email,omitempty"`
	Name   string `json:"name,omitempty"`
	Role   string `json:"role,omitempty"`
	Active *bool  `json:"active,omitempty"`
}

// ChangePasswordRequest represents the request to change password
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password,omitempty"`
	NewPassword     string `json:"new_password"`
}

// listUsers returns all users (admin only)
func (s *Server) listUsers(c *fiber.Ctx) error {
	// Check if user is admin
	role := c.Locals("userRole")
	if role != "admin" {
		return c.Status(403).JSON(fiber.Map{"error": "Admin access required"})
	}

	rows, err := s.db.Query(`
		SELECT id, email, name, role, active, created_at, updated_at
		FROM users
		ORDER BY created_at DESC
	`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch users"})
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}

	if users == nil {
		users = []User{}
	}

	return c.JSON(users)
}

// getUser returns a specific user
func (s *Server) getUser(c *fiber.Ctx) error {
	id := c.Params("id")
	currentUserId := c.Locals("userId")
	role := c.Locals("userRole")

	// Users can only view themselves unless admin
	if role != "admin" && id != currentUserId {
		return c.Status(403).JSON(fiber.Map{"error": "Access denied"})
	}

	var u User
	err := s.db.QueryRow(`
		SELECT id, email, name, role, active, created_at, updated_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt)

	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch user"})
	}

	return c.JSON(u)
}

// getCurrentUser returns the current logged-in user
func (s *Server) getCurrentUser(c *fiber.Ctx) error {
	userId := c.Locals("userId")

	var u User
	err := s.db.QueryRow(`
		SELECT id, email, name, role, active, created_at, updated_at
		FROM users WHERE id = $1
	`, userId).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch user"})
	}

	return c.JSON(u)
}

// createUser creates a new user (admin only)
func (s *Server) createUser(c *fiber.Ctx) error {
	// Check if user is admin
	role := c.Locals("userRole")
	if role != "admin" {
		return c.Status(403).JSON(fiber.Map{"error": "Admin access required"})
	}

	var req CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Validate required fields
	if req.Email == "" || req.Password == "" || req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Email, password and name are required"})
	}

	// Validate password length
	if len(req.Password) < 6 {
		return c.Status(400).JSON(fiber.Map{"error": "Password must be at least 6 characters"})
	}

	// Default role to 'user'
	if req.Role == "" {
		req.Role = "user"
	}

	// Validate role
	if req.Role != "admin" && req.Role != "user" {
		return c.Status(400).JSON(fiber.Map{"error": "Role must be 'admin' or 'user'"})
	}

	// Check if email already exists
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`, req.Email).Scan(&exists)
	if exists {
		return c.Status(400).JSON(fiber.Map{"error": "Email already exists"})
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to hash password"})
	}

	// Create user
	var u User
	err = s.db.QueryRow(`
		INSERT INTO users (email, password_hash, name, role, active)
		VALUES ($1, $2, $3, $4, true)
		RETURNING id, email, name, role, active, created_at, updated_at
	`, req.Email, string(hashedPassword), req.Name, req.Role).Scan(
		&u.ID, &u.Email, &u.Name, &u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt,
	)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create user"})
	}

	return c.Status(201).JSON(u)
}

// updateUser updates a user
func (s *Server) updateUser(c *fiber.Ctx) error {
	id := c.Params("id")
	currentUserId := c.Locals("userId")
	currentRole := c.Locals("userRole")

	// Users can only update themselves unless admin
	isAdmin := currentRole == "admin"
	isSelf := id == currentUserId

	if !isAdmin && !isSelf {
		return c.Status(403).JSON(fiber.Map{"error": "Access denied"})
	}

	var req UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Check if user exists
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, id).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	// Non-admins cannot change role or active status
	if !isAdmin && (req.Role != "" || req.Active != nil) {
		return c.Status(403).JSON(fiber.Map{"error": "Only admins can change role or status"})
	}

	// Check if email is being changed to an existing one
	if req.Email != "" {
		var emailExists bool
		s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 AND id != $2)`, req.Email, id).Scan(&emailExists)
		if emailExists {
			return c.Status(400).JSON(fiber.Map{"error": "Email already exists"})
		}
	}

	// Build update query dynamically
	updates := []string{}
	args := []interface{}{}
	argNum := 1

	if req.Email != "" {
		updates = append(updates, fmt.Sprintf("email = $%d", argNum))
		args = append(args, req.Email)
		argNum++
	}
	if req.Name != "" {
		updates = append(updates, fmt.Sprintf("name = $%d", argNum))
		args = append(args, req.Name)
		argNum++
	}
	if req.Role != "" && isAdmin {
		updates = append(updates, fmt.Sprintf("role = $%d", argNum))
		args = append(args, req.Role)
		argNum++
	}
	if req.Active != nil && isAdmin {
		updates = append(updates, fmt.Sprintf("active = $%d", argNum))
		args = append(args, *req.Active)
		argNum++
	}

	if len(updates) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "No fields to update"})
	}

	// Add updated_at
	updates = append(updates, "updated_at = NOW()")

	// Build and execute query
	query := "UPDATE users SET "
	for i, u := range updates {
		if i > 0 {
			query += ", "
		}
		query += u
	}
	query += fmt.Sprintf(" WHERE id = $%d", argNum)
	args = append(args, id)

	_, err := s.db.Exec(query, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update user"})
	}

	// Return updated user
	var u User
	s.db.QueryRow(`
		SELECT id, email, name, role, active, created_at, updated_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt)

	return c.JSON(u)
}

// deleteUser deletes a user (admin only)
func (s *Server) deleteUser(c *fiber.Ctx) error {
	// Check if user is admin
	role := c.Locals("userRole")
	if role != "admin" {
		return c.Status(403).JSON(fiber.Map{"error": "Admin access required"})
	}

	id := c.Params("id")
	currentUserId := c.Locals("userId")

	// Cannot delete yourself
	if id == currentUserId {
		return c.Status(400).JSON(fiber.Map{"error": "Cannot delete your own account"})
	}

	// Check if user exists
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, id).Scan(&exists)
	if !exists {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	// Delete user
	_, err := s.db.Exec(`DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete user"})
	}

	return c.JSON(fiber.Map{"message": "User deleted successfully"})
}

// changePassword changes a user's password
func (s *Server) changePassword(c *fiber.Ctx) error {
	id := c.Params("id")
	currentUserId := c.Locals("userId")
	currentRole := c.Locals("userRole")

	isAdmin := currentRole == "admin"
	isSelf := id == currentUserId

	// Only self or admin can change password
	if !isAdmin && !isSelf {
		return c.Status(403).JSON(fiber.Map{"error": "Access denied"})
	}

	var req ChangePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Validate new password
	if len(req.NewPassword) < 6 {
		return c.Status(400).JSON(fiber.Map{"error": "Password must be at least 6 characters"})
	}

	// If changing own password, require current password
	if isSelf && !isAdmin {
		if req.CurrentPassword == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Current password is required"})
		}

		// Verify current password
		var passwordHash string
		err := s.db.QueryRow(`SELECT password_hash FROM users WHERE id = $1`, id).Scan(&passwordHash)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to verify password"})
		}

		if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.CurrentPassword)); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Current password is incorrect"})
		}
	}

	// If admin changing someone else's password, current password not required
	// But if self is also admin, still don't require current password

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 10)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to hash password"})
	}

	// Update password
	_, err = s.db.Exec(`UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`, string(hashedPassword), id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update password"})
	}

	return c.JSON(fiber.Map{"message": "Password changed successfully"})
}
