package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/your-org/mock-gateway/internal/config"
	"github.com/your-org/mock-gateway/internal/router"
	"github.com/your-org/mock-gateway/internal/seed"
	"github.com/your-org/mock-gateway/internal/store"
)

func main() {
	configPath := flag.String("config", envOr("MOCK_CONFIG", "config.yaml"), "path to config.yaml")
	snapshotPath := flag.String("snapshot", envOr("MOCK_SNAPSHOT", "data/store.json"), "path to persist mock state")
	usersPath := flag.String("users", envOr("MOCK_USERS", "data/users.json"), "path to persist dynamic users")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Override individual user tokens from environment variables.
	// Matches by role: MOCK_ADMIN_TOKEN sets the first admin user's token, etc.
	for i := range cfg.Users {
		switch cfg.Users[i].Role {
		case "admin":
			if v := os.Getenv("MOCK_ADMIN_TOKEN"); v != "" {
				cfg.Users[i].Token = v
			}
		case "tester":
			if v := os.Getenv("MOCK_TESTER_TOKEN"); v != "" {
				cfg.Users[i].Token = v
			}
		case "viewer":
			if v := os.Getenv("MOCK_VIEWER_TOKEN"); v != "" {
				cfg.Users[i].Token = v
			}
		}
	}

	// Store with auto-snapshot on every write
	st := store.NewWithSnapshot(*snapshotPath, *usersPath)
	logs := store.NewLogRing(200)

	// Load saved state FIRST — preserves active toggles, bodies, delays across restarts
	if err := st.LoadSnapshot(*snapshotPath); err != nil {
		log.Printf("warning: could not load snapshot: %v", err)
	}
	if err := st.LoadUsers(*usersPath); err != nil {
		log.Printf("warning: could not load users: %v", err)
	}

	// Seed from swagger specs — fills in any NEW routes not already in snapshot
	seed.All(cfg, st)

	h := router.New(cfg, st, logs)

	addr := fmt.Sprintf("%s:%d", cfg.Gateway.Host, cfg.Gateway.Port)
	fmt.Printf("\n  Mock gateway running\n")
	fmt.Printf("  %-20s http://%s\n", "Gateway:", addr)
	fmt.Printf("  %-20s http://localhost:%d/mock-ui/\n", "Developer UI:", cfg.Gateway.Port)
	fmt.Printf("  %-20s http://localhost:%d/admin/services\n", "Admin API:", cfg.Gateway.Port)
	fmt.Printf("  %-20s http://localhost:%d/health\n", "Health:", cfg.Gateway.Port)
	fmt.Printf("  %-20s %s\n\n", "Snapshot:", *snapshotPath)
	fmt.Printf("  Services registered:\n")
	for _, svc := range cfg.Services {
		fmt.Printf("    %-16s -> %s\n", svc.Name, svc.URL)
	}
	fmt.Printf("\n  Headers required:\n")
	fmt.Printf("    X-Mock-Service:  <service-name>\n")
	fmt.Printf("    X-Mock-Enabled:  true\n")
	fmt.Printf("    X-Mock-Env:      uat-api|uat-ui|uat-dev|prod-api|prod-ui|prod-dev\n\n")

	srv := &http.Server{
		Addr:           addr,
		Handler:        h,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   60 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("\n  Shutting down gracefully...")

	// Final snapshot before exit
	if err := st.SaveSnapshot(*snapshotPath); err != nil {
		log.Printf("warning: final snapshot failed: %v", err)
	} else {
		fmt.Printf("  Snapshot saved to %s\n", *snapshotPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	fmt.Println("  Done.")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
