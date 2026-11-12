package store

import (
	"path/filepath"
	"testing"

	"nvr-app/internal/models"
)

func TestPersistentStoreKeepsCamerasAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "cameras.json")
	first, err := NewPersistentStore(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	created, err := first.CreateCamera(models.CreateCameraRequest{
		Name: "Entrance", RTSPUrl: "rtsp://192.168.1.10:554/stream",
		Username: "camera-user", Password: "camera-pass",
	})
	if err != nil {
		t.Fatalf("create camera: %v", err)
	}

	restarted, err := NewPersistentStore(path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	loaded, err := restarted.GetCamera(created.ID)
	if err != nil {
		t.Fatalf("camera not restored after restart: %v", err)
	}
	if loaded.Name != created.Name || loaded.RTSPUrl != created.RTSPUrl ||
		loaded.Username != "camera-user" || loaded.Password != "camera-pass" {
		t.Fatalf("restored camera does not match: %#v", loaded)
	}

	if err := restarted.DeleteCamera(created.ID); err != nil {
		t.Fatalf("delete camera: %v", err)
	}
	afterDelete, err := NewPersistentStore(path)
	if err != nil {
		t.Fatalf("reopen after delete: %v", err)
	}
	if _, err := afterDelete.GetCamera(created.ID); err != ErrNotFound {
		t.Fatalf("deleted camera unexpectedly exists, error: %v", err)
	}
}
