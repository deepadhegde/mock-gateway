package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/your-org/mock-gateway/internal/config"
	"github.com/your-org/mock-gateway/internal/router"
	"github.com/your-org/mock-gateway/internal/seed"
	"github.com/your-org/mock-gateway/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config.yaml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	st   := store.New()
	logs := store.NewLogRing(200)

	// Seed mock store from all swagger specs
	seed.All(cfg, st)

	// Build router
	h := router.New(cfg, st, logs)

	addr := fmt.Sprintf("%s:%d", cfg.Gateway.Host, cfg.Gateway.Port)
	fmt.Printf("\n  Mock gateway running\n")
	fmt.Printf("  %-20s http://%s\n", "Gateway:", addr)
	fmt.Printf("  %-20s http://%s/mock-ui/\n", "Developer UI:", addr)
	fmt.Printf("  %-20s http://%s/admin/services\n", "Admin API:", addr)
	fmt.Printf("  %-20s http://%s/health\n\n", "Health:", addr)
	fmt.Printf("  Services registered:\n")
	for _, svc := range cfg.Services {
		fmt.Printf("    %-16s → %s\n", svc.Name, svc.URL)
	}
	fmt.Printf("\n  Headers required on each request:\n")
	fmt.Printf("    X-Mock-Service:  <service-name>\n")
	fmt.Printf("    X-Mock-Enabled:  true          (activates mock)\n")
	fmt.Printf("    X-Mock-Env:      api|ui|dev     (picks response body)\n\n")

	log.Fatal(http.ListenAndServe(addr, h))
}
