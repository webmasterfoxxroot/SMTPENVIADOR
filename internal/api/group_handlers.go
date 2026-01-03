package api

import (
	"bufio"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// Group represents an email list group
type Group struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	Color       string     `json:"color"`
	ListCount   int        `json:"list_count"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// listGroups returns all groups
func (s *Server) listGroups(c *fiber.Ctx) error {
	rows, err := s.db.Query(`
		SELECT
			g.id, g.name, g.description, g.color, g.created_at, g.updated_at,
			COUNT(l.id) as list_count
		FROM email_list_groups g
		LEFT JOIN email_lists l ON l.group_id = g.id
		GROUP BY g.id
		ORDER BY g.name ASC
	`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch groups"})
	}
	defer rows.Close()

	groups := []Group{}
	for rows.Next() {
		var g Group
		err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.Color, &g.CreatedAt, &g.UpdatedAt, &g.ListCount)
		if err != nil {
			continue
		}
		groups = append(groups, g)
	}

	return c.JSON(fiber.Map{"data": groups})
}

// createGroup creates a new group
func (s *Server) createGroup(c *fiber.Ctx) error {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Color       string `json:"color"`
	}

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if body.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	if body.Color == "" {
		body.Color = "#3B82F6" // Default blue
	}

	id := uuid.New().String()
	_, err := s.db.Exec(`
		INSERT INTO email_list_groups (id, name, description, color)
		VALUES ($1, $2, $3, $4)
	`, id, body.Name, body.Description, body.Color)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create group"})
	}

	return c.JSON(fiber.Map{
		"id":      id,
		"message": "Group created successfully",
	})
}

// updateGroup updates a group
func (s *Server) updateGroup(c *fiber.Ctx) error {
	id := c.Params("id")

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Color       string `json:"color"`
	}

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	_, err := s.db.Exec(`
		UPDATE email_list_groups
		SET name = $1, description = $2, color = $3, updated_at = NOW()
		WHERE id = $4
	`, body.Name, body.Description, body.Color, id)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update group"})
	}

	return c.JSON(fiber.Map{"message": "Group updated successfully"})
}

// deleteGroup deletes a group (lists are moved to "no group")
func (s *Server) deleteGroup(c *fiber.Ctx) error {
	id := c.Params("id")

	// First, remove group_id from all lists in this group
	s.db.Exec(`UPDATE email_lists SET group_id = NULL WHERE group_id = $1`, id)

	// Then delete the group
	_, err := s.db.Exec(`DELETE FROM email_list_groups WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete group"})
	}

	return c.JSON(fiber.Map{"message": "Group deleted successfully"})
}

// moveListToGroup moves a list to a group
func (s *Server) moveListToGroup(c *fiber.Ctx) error {
	listID := c.Params("id")

	var body struct {
		GroupID *string `json:"group_id"` // null to remove from group
	}

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	var err error
	if body.GroupID == nil || *body.GroupID == "" {
		_, err = s.db.Exec(`UPDATE email_lists SET group_id = NULL WHERE id = $1`, listID)
	} else {
		_, err = s.db.Exec(`UPDATE email_lists SET group_id = $1 WHERE id = $2`, *body.GroupID, listID)
	}

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to move list"})
	}

	return c.JSON(fiber.Map{"message": "List moved successfully"})
}

// downloadList exports a list as CSV
func (s *Server) downloadList(c *fiber.Ctx) error {
	listID := c.Params("id")
	format := c.Query("format", "csv") // csv or txt

	// Get list name
	var listName string
	err := s.db.QueryRow(`SELECT name FROM email_lists WHERE id = $1`, listID).Scan(&listName)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "List not found"})
	}

	// Set headers for download BEFORE streaming
	contentType := "text/csv"
	ext := ".csv"
	if format == "txt" {
		contentType = "text/plain"
		ext = ".txt"
	}

	c.Set("Content-Type", contentType)
	c.Set("Content-Disposition", "attachment; filename=\""+listName+ext+"\"")

	// Stream response
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Write CSV header
		if format == "csv" {
			w.WriteString("email,name\n")
		}

		// Get emails - no ORDER BY for speed
		rows, err := s.db.Query(`
			SELECT email, COALESCE(name, '')
			FROM emails
			WHERE list_id = $1 AND valid = true
		`, listID)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var email, name string
			rows.Scan(&email, &name)
			if format == "txt" {
				w.WriteString(email + "\n")
			} else {
				if name != "" {
					w.WriteString(email + ",\"" + name + "\"\n")
				} else {
					w.WriteString(email + ",\n")
				}
			}
		}
		w.Flush()
	})

	return nil
}
