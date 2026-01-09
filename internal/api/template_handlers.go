package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type TemplateRequest struct {
	Name        string `json:"name"`
	FromName    string `json:"from_name"`
	Subject     string `json:"subject"`
	HTMLContent string `json:"html_content"`
	TextContent string `json:"text_content"`
}

// listTemplates returns all templates
func (s *Server) listTemplates(c *fiber.Ctx) error {
	rows, err := s.db.Query(`
		SELECT id, name, COALESCE(from_name, ''), subject, created_at, updated_at
		FROM templates
		ORDER BY created_at DESC
	`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch templates"})
	}
	defer rows.Close()

	var templates []fiber.Map
	for rows.Next() {
		var id, name, fromName, subject string
		var createdAt, updatedAt time.Time

		rows.Scan(&id, &name, &fromName, &subject, &createdAt, &updatedAt)
		templates = append(templates, fiber.Map{
			"id":         id,
			"name":       name,
			"from_name":  fromName,
			"subject":    subject,
			"created_at": createdAt,
			"updated_at": updatedAt,
		})
	}

	return c.JSON(fiber.Map{
		"data":  templates,
		"total": len(templates),
	})
}

// createTemplate creates a new template
func (s *Server) createTemplate(c *fiber.Ctx) error {
	var req TemplateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Name == "" || req.Subject == "" || req.HTMLContent == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Missing required fields"})
	}

	id := uuid.New().String()

	_, err := s.db.Exec(`
		INSERT INTO templates (id, name, from_name, subject, html_content, text_content)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, req.Name, req.FromName, req.Subject, req.HTMLContent, req.TextContent)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create template"})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Template created",
		"id":      id,
	})
}

// getTemplate returns a single template
func (s *Server) getTemplate(c *fiber.Ctx) error {
	id := c.Params("id")

	var name, fromName, subject, htmlContent, textContent string
	var createdAt, updatedAt time.Time

	err := s.db.QueryRow(`
		SELECT name, COALESCE(from_name, ''), subject, html_content, text_content, created_at, updated_at
		FROM templates WHERE id = $1
	`, id).Scan(&name, &fromName, &subject, &htmlContent, &textContent, &createdAt, &updatedAt)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Template not found"})
	}

	return c.JSON(fiber.Map{
		"id":           id,
		"name":         name,
		"from_name":    fromName,
		"subject":      subject,
		"html_content": htmlContent,
		"text_content": textContent,
		"created_at":   createdAt,
		"updated_at":   updatedAt,
	})
}

// updateTemplate updates a template
func (s *Server) updateTemplate(c *fiber.Ctx) error {
	id := c.Params("id")

	var req TemplateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	result, err := s.db.Exec(`
		UPDATE templates SET name = $1, from_name = $2, subject = $3, html_content = $4, text_content = $5
		WHERE id = $6
	`, req.Name, req.FromName, req.Subject, req.HTMLContent, req.TextContent, id)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update template"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Template not found"})
	}

	return c.JSON(fiber.Map{"message": "Template updated"})
}

// deleteTemplate deletes a template
func (s *Server) deleteTemplate(c *fiber.Ctx) error {
	id := c.Params("id")

	result, err := s.db.Exec(`DELETE FROM templates WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete template"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Template not found"})
	}

	return c.JSON(fiber.Map{"message": "Template deleted"})
}
