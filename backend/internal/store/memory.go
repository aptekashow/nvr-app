package store

import (
	"errors"
	"nvr-app/internal/models"
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
	mu      sync.RWMutex
	users   map[string]*models.User
	cameras map[string]*models.Camera
}

func NewMemoryStore() *MemoryStore {
	s := &MemoryStore{
		users:   make(map[string]*models.User),
		cameras: make(map[string]*models.Camera),
	}

	// default admin: admin / admin
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	admin := &models.User{
		ID:           uuid.NewString(),
		Username:     "admin",
		PasswordHash: string(hash),
		CreatedAt:    time.Now(),
	}
	s.users[admin.Username] = admin

	return s
}

func (s *MemoryStore) GetUserByUsername(username string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[username]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

func (s *MemoryStore) CheckPassword(username, password string) (*models.User, error) {
	u, err := s.GetUserByUsername(username)
	if err != nil {
		return nil, ErrInvalidCreds
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCreds
	}
	return u, nil
}

func (s *MemoryStore) ListCameras() []*models.Camera {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]*models.Camera, 0, len(s.cameras))
	for _, c := range s.cameras {
		list = append(list, c)
	}
	return list
}

func (s *MemoryStore) GetCamera(id string) (*models.Camera, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.cameras[id]
	if !ok {
		return nil, ErrNotFound
	}
	return c, nil
}

func (s *MemoryStore) CreateCamera(req models.CreateCameraRequest) (*models.Camera, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	c := &models.Camera{
		ID:        uuid.NewString(),
		Name:      req.Name,
		RTSPUrl:   req.RTSPUrl,
		Username:  req.Username,
		Password:  req.Password,
		Enabled:   true,
		Recording: false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.cameras[c.ID] = c
	return c, nil
}

func (s *MemoryStore) UpdateCamera(id string, upd *models.Camera) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cameras[id]; !ok {
		return ErrNotFound
	}
	upd.UpdatedAt = time.Now()
	s.cameras[id] = upd
	return nil
}

func (s *MemoryStore) DeleteCamera(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cameras[id]; !ok {
		return ErrNotFound
	}
	delete(s.cameras, id)
	return nil
}

func (s *MemoryStore) SetRecording(id string, recording bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cameras[id]
	if !ok {
		return ErrNotFound
	}
	c.Recording = recording
	c.UpdatedAt = time.Now()
	return nil
}
