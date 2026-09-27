package localserver

import (
	"context"
	"log"
	"net/http"
	"os"
)

// Run starts the disk-backed development server.
func Run() error {
	address := os.Getenv("CANVAS_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	dataDir := os.Getenv("CANVAS_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	hub, err := newDemoHubWithStore(dataDir)
	if err != nil {
		return err
	}
	go hub.runDailyMaintenance(context.Background())
	go hub.runCustomMaintenance(context.Background())
	handler := newDemoHandlerWithHub(hub)
	log.Printf("Canvas listening on %s", address)
	return http.ListenAndServe(address, handler)
}
