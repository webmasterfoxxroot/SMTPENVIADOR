package api

import (
	"database/sql"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"

	"smtpenviador/internal/config"
	"smtpenviador/internal/engine"
	"smtpenviador/internal/queue"
)

// Server represents the API server
type Server struct {
	app    *fiber.App
	cfg    *config.Config
	db     *sql.DB
	queue  *queue.Manager
	engine *engine.Engine
}

// NewServer creates a new API server
func NewServer(cfg *config.Config, db *sql.DB, q *queue.Manager, eng *engine.Engine) *Server {
	app := fiber.New(fiber.Config{
		AppName:      "SMTPENVIADOR API",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		BodyLimit:    50 * 1024 * 1024, // 50MB for list uploads
	})

	server := &Server{
		app:    app,
		cfg:    cfg,
		db:     db,
		queue:  q,
		engine: eng,
	}

	server.setupMiddlewares()
	server.setupRoutes()

	return server
}

// setupMiddlewares configures middlewares
func (s *Server) setupMiddlewares() {
	s.app.Use(recover.New())
	s.app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} (${latency})\n",
	}))
	s.app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization",
	}))
}

// setupRoutes configures API routes
func (s *Server) setupRoutes() {
	// Health check
	s.app.Get("/health", s.healthCheck)

	// API v1
	api := s.app.Group("/api/v1")

	// Auth routes
	auth := api.Group("/auth")
	auth.Post("/login", s.login)
	auth.Post("/logout", s.logout)

	// Protected routes
	protected := api.Group("/", s.authMiddleware)

	// Dashboard stats
	protected.Get("/stats", s.getStats)
	protected.Get("/stats/realtime", websocket.New(s.realtimeStats))

	// SMTP Servers
	smtp := protected.Group("/smtp")
	smtp.Get("/", s.listSMTPs)
	smtp.Post("/", s.createSMTP)
	smtp.Get("/:id", s.getSMTP)
	smtp.Put("/:id", s.updateSMTP)
	smtp.Delete("/:id", s.deleteSMTP)
	smtp.Post("/:id/test", s.testSMTP)
	smtp.Post("/refresh", s.refreshSMTPs)

	// Email Lists
	lists := protected.Group("/lists")
	lists.Get("/", s.listEmailLists)
	lists.Post("/", s.createEmailList)
	lists.Get("/:id", s.getEmailList)
	lists.Put("/:id", s.updateEmailList)
	lists.Delete("/:id", s.deleteEmailList)
	lists.Post("/:id/upload", s.uploadEmails)
	lists.Get("/:id/emails", s.getListEmails)
	lists.Delete("/:id/emails/:emailId", s.deleteEmail)

	// Templates
	templates := protected.Group("/templates")
	templates.Get("/", s.listTemplates)
	templates.Post("/", s.createTemplate)
	templates.Get("/:id", s.getTemplate)
	templates.Put("/:id", s.updateTemplate)
	templates.Delete("/:id", s.deleteTemplate)

	// Campaigns
	campaigns := protected.Group("/campaigns")
	campaigns.Get("/", s.listCampaigns)
	campaigns.Post("/", s.createCampaign)
	campaigns.Get("/:id", s.getCampaign)
	campaigns.Put("/:id", s.updateCampaign)
	campaigns.Delete("/:id", s.deleteCampaign)
	campaigns.Post("/:id/start", s.startCampaign)
	campaigns.Post("/:id/pause", s.pauseCampaign)
	campaigns.Post("/:id/resume", s.resumeCampaign)
	campaigns.Post("/:id/cancel", s.cancelCampaign)
	campaigns.Get("/:id/stats", s.getCampaignStats)

	// Blacklist
	blacklist := protected.Group("/blacklist")
	blacklist.Get("/", s.listBlacklist)
	blacklist.Post("/", s.addToBlacklist)
	blacklist.Delete("/:id", s.removeFromBlacklist)
	blacklist.Post("/import", s.importBlacklist)

	// Tracking endpoints (public)
	s.app.Get("/track/open/:campaignId/:emailId", s.trackOpen)
	s.app.Get("/track/click/:campaignId/:emailId", s.trackClick)
	s.app.Get("/unsubscribe/:campaignId/:emailId", s.unsubscribe)

	// Logs
	protected.Get("/logs", s.getLogs)
}

// Start starts the API server
func (s *Server) Start() error {
	return s.app.Listen(":" + s.cfg.APIPort)
}

// Stop stops the API server
func (s *Server) Stop() error {
	return s.app.Shutdown()
}

// healthCheck returns server health status
func (s *Server) healthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "ok",
		"version": "1.0.0",
		"engine":  s.engine.IsRunning(),
	})
}
