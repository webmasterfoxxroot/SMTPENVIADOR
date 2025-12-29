package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"smtpenviador/internal/api"
	"smtpenviador/internal/config"
	"smtpenviador/internal/database"
	"smtpenviador/internal/engine"
	"smtpenviador/internal/queue"
)

func main() {
	log.Println("🚀 SMTPENVIADOR - Starting...")

	// Load configuration
	cfg := config.Load()
	log.Println("✅ Configuration loaded")

	// Connect to database
	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("❌ Database connection failed: %v", err)
	}
	defer db.Close()
	log.Println("✅ Database connected")

	// Connect to Redis
	rdb, err := queue.Connect(cfg)
	if err != nil {
		log.Fatalf("❌ Redis connection failed: %v", err)
	}
	defer rdb.Close()
	log.Println("✅ Redis connected")

	// Initialize queue manager
	queueManager := queue.NewManager(rdb)

	// Start email engine
	emailEngine := engine.New(cfg, db, queueManager)
	go emailEngine.Start()
	log.Println("✅ Email engine started")

	// Start API server
	server := api.NewServer(cfg, db, queueManager, emailEngine)
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("❌ API server failed: %v", err)
		}
	}()
	log.Printf("✅ API server running on port %s", cfg.APIPort)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down...")
	emailEngine.Stop()
	server.Stop()
	log.Println("👋 Goodbye!")
}
