package api

import (
	"database/sql"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Setting represents a system setting
type Setting struct {
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// getSettings returns all system settings
func (s *Server) getSettings(c *fiber.Ctx) error {
	rows, err := s.db.Query(`
		SELECT key, value, description, updated_at
		FROM settings
		ORDER BY key
	`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to fetch settings",
		})
	}
	defer rows.Close()

	settings := make(map[string]Setting)
	for rows.Next() {
		var setting Setting
		if err := rows.Scan(&setting.Key, &setting.Value, &setting.Description, &setting.UpdatedAt); err != nil {
			continue
		}
		settings[setting.Key] = setting
	}

	return c.JSON(settings)
}

// getSetting returns a single setting by key
func (s *Server) getSetting(c *fiber.Ctx) error {
	key := c.Params("key")

	var setting Setting
	err := s.db.QueryRow(`
		SELECT key, value, description, updated_at
		FROM settings
		WHERE key = $1
	`, key).Scan(&setting.Key, &setting.Value, &setting.Description, &setting.UpdatedAt)

	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{
			"error": "Setting not found",
		})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to fetch setting",
		})
	}

	return c.JSON(setting)
}

// updateSetting updates a single setting
func (s *Server) updateSetting(c *fiber.Ctx) error {
	key := c.Params("key")

	var req struct {
		Value string `json:"value"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	result, err := s.db.Exec(`
		UPDATE settings
		SET value = $1, updated_at = CURRENT_TIMESTAMP
		WHERE key = $2
	`, req.Value, key)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to update setting",
		})
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{
			"error": "Setting not found",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Setting updated successfully",
	})
}

// updateSettings updates multiple settings at once
func (s *Server) updateSettings(c *fiber.Ctx) error {
	var req map[string]string

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	tx, err := s.db.Begin()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to start transaction",
		})
	}

	for key, value := range req {
		_, err := tx.Exec(`
			UPDATE settings
			SET value = $1, updated_at = CURRENT_TIMESTAMP
			WHERE key = $2
		`, value, key)

		if err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to update settings",
			})
		}
	}

	if err := tx.Commit(); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to commit changes",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Settings updated successfully",
	})
}

// getServerInfo returns server information including public IP
func (s *Server) getServerInfo(c *fiber.Ctx) error {
	ip := getPublicIP()

	return c.JSON(fiber.Map{
		"ip": ip,
	})
}

// getPublicIP fetches the server's public IP address
func getPublicIP() string {
	services := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	}

	client := &http.Client{Timeout: 5 * time.Second}

	for _, service := range services {
		resp, err := client.Get(service)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		ip := strings.TrimSpace(string(body))
		if ip != "" {
			return ip
		}
	}

	return ""
}

// restartServer restarts the email engine
func (s *Server) restartServer(c *fiber.Ctx) error {
	// Stop the engine
	if s.engine.IsRunning() {
		s.engine.Stop()
	}

	// Start the engine again
	go s.engine.Start()

	return c.JSON(fiber.Map{
		"message": "Server restarted successfully",
	})
}
