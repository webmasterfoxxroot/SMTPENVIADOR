package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"smtpenviador/internal/api"
	"smtpenviador/internal/clickhouse"
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

	// Connect to PostgreSQL (relational data)
	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("❌ PostgreSQL connection failed: %v", err)
	}
	defer db.Close()
	log.Println("✅ PostgreSQL connected")

	// Connect to ClickHouse (bulk data)
	ch, err := clickhouse.Connect(cfg)
	if err != nil {
		log.Printf("⚠️ ClickHouse connection failed: %v (will use PostgreSQL fallback)", err)
		ch = nil
	} else {
		defer ch.Close()
	}

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
	server := api.NewServer(cfg, db, ch, queueManager, emailEngine)
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
