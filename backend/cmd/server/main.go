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

	// Хранилище (пока in-memory + файлы, потом можно заменить на SQLite/Postgres)
	st := store.NewMemoryStore()

	// Менеджер потоков и записи
	sm := stream.NewManager(cfg, st)

	// Запускаем фоновые задачи (превью, запись)
	go sm.Start()

	e := api.NewRouter(cfg, st, sm)

	log.Printf("NVR backend listening on :%s", cfg.HTTPPort)
	if err := e.Start(":" + cfg.HTTPPort); err != nil {
		log.Fatal(err)
	}
}
