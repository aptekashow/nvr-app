package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"nvr-app/internal/models"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrInvalidCreds  = errors.New("invalid credentials")
)

type MemoryStore struct {
	mu         sync.RWMutex
	users      map[string]*models.User
	cameras    map[string]*models.Camera
	camerasFile string
}

type storedCamera struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	RTSPUrl   string    `json:"rtsp_url"`
	Username  string    `json:"username,omitempty"`
	Password  string    `json:"password,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewMemoryStore() *MemoryStore {
	s := &MemoryStore{
		users:   make(map[string]*models.User),
		cameras: make(map[string]*models.Camera),
	}
	s.addDefaultAdmin()
	return s
}

// NewPersistentStore loads camera configuration from disk and saves every
// camera change atomically. User credentials remain in-memory for now.
func NewPersistentStore(path string) (*MemoryStore, error) {
	s := NewMemoryStore()
	s.camerasFile = strings.TrimSpace(path)
	if s.camerasFile == "" {
		return nil, fmt.Errorf("camera storage path is required")
	}
	data, err := os.ReadFile(s.camerasFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read camera storage: %w", err)
	}
	if err == nil {
		var saved []storedCamera
		if err := json.Unmarshal(data, &saved); err != nil {
			return nil, fmt.Errorf("decode camera storage: %w", err)
		}
		for _, item := range saved {
			if item.ID == "" || item.Name == "" || item.RTSPUrl == "" {
				continue
			}
			s.cameras[item.ID] = &models.Camera{
				ID: item.ID, Name: item.Name, RTSPUrl: item.RTSPUrl,
				Username: item.Username, Password: item.Password,
				Enabled: item.Enabled, Recording: false,
				CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
			}
		}
	}
	return s, nil
}

func (s *MemoryStore) addDefaultAdmin() {
	// Temporary bootstrap credentials; change before exposing the service.
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	admin := &models.User{
		ID: uuid.NewString(), Username: "admin",
		PasswordHash: string(hash), CreatedAt: time.Now(),
	}
	s.users[admin.Username] = admin
}

func (s *MemoryStore) persistCamerasLocked() error {
	if s.camerasFile == "" {
		return nil
	}
	items := make([]storedCamera, 0, len(s.cameras))
	for _, c := range s.cameras {
		items = append(items, storedCamera{
			ID: c.ID, Name: c.Name, RTSPUrl: c.RTSPUrl,
			Username: c.Username, Password: c.Password,
			Enabled: c.Enabled, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		})
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.camerasFile), 0700); err != nil {
		return err
	}
	tmp := s.camerasFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.camerasFile); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (s *MemoryStore) GetUserByUsername(username string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[username]
	if !ok { return nil, ErrNotFound }
	return u, nil
}

func (s *MemoryStore) CheckPassword(username, password string) (*models.User, error) {
	u, err := s.GetUserByUsername(username)
	if err != nil { return nil, ErrInvalidCreds }
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCreds
	}
	return u, nil
}

func (s *MemoryStore) ListCameras() []*models.Camera {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]*models.Camera, 0, len(s.cameras))
	for _, c := range s.cameras { list = append(list, c) }
	return list
}

func (s *MemoryStore) GetCamera(id string) (*models.Camera, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.cameras[id]
	if !ok { return nil, ErrNotFound }
	return c, nil
}

func (s *MemoryStore) CreateCamera(req models.CreateCameraRequest) (*models.Camera, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	c := &models.Camera{
		ID: uuid.NewString(), Name: strings.TrimSpace(req.Name),
		RTSPUrl: strings.TrimSpace(req.RTSPUrl), Username: req.Username,
		Password: req.Password, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	s.cameras[c.ID] = c
	if err := s.persistCamerasLocked(); err != nil {
		delete(s.cameras, c.ID)
		return nil, fmt.Errorf("save camera: %w", err)
	}
	return c, nil
}

func (s *MemoryStore) UpdateCamera(id string, upd *models.Camera) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.cameras[id]
	if !ok { return ErrNotFound }
	upd.UpdatedAt = time.Now()
	s.cameras[id] = upd
	if err := s.persistCamerasLocked(); err != nil {
		s.cameras[id] = old
		return fmt.Errorf("save camera: %w", err)
	}
	return nil
}

func (s *MemoryStore) DeleteCamera(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.cameras[id]
	if !ok { return ErrNotFound }
	delete(s.cameras, id)
	if err := s.persistCamerasLocked(); err != nil {
		s.cameras[id] = old
		return fmt.Errorf("save camera deletion: %w", err)
	}
	return nil
}

func (s *MemoryStore) SetRecording(id string, recording bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cameras[id]
	if !ok { return ErrNotFound }
	c.Recording = recording
	c.UpdatedAt = time.Now()
	return nil
}
