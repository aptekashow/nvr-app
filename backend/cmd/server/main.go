package main

import (
	"log"
	"nvr-app/internal/api"
	"nvr-app/internal/config"
	"nvr-app/internal/store"
	"nvr-app/internal/stream"
)

func main() {
	cfg := config.Load()
	st, err := store.NewPersistentStore(cfg.CamerasFile)
	if err != nil {
		log.Fatalf("initialize camera storage: %v", err)
	}

	sm := stream.NewManager(cfg, st)
	go sm.Start()

	e := api.NewRouter(cfg, st, sm)
	log.Printf("NVR backend listening on :%s", cfg.HTTPPort)
	if err := e.Start(":" + cfg.HTTPPort); err != nil {
		log.Fatal(err)
	}
}
