package models

import "time"

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Camera struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	RTSPUrl   string    `json:"-"`
	Username  string    `json:"-"`
	Password  string    `json:"-"`
	Enabled   bool      `json:"enabled"`
	Recording bool      `json:"recording"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CameraStatus struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Online      bool      `json:"online"`
	Recording   bool      `json:"recording"`
	PreviewURL  string    `json:"preview_url"`
	LastFrameAt time.Time `json:"last_frame_at"`
	HLSUrl      string    `json:"hls_url"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type CreateCameraRequest struct {
	Name     string `json:"name"`
	RTSPUrl  string `json:"rtsp_url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type ExportRequest struct {
	CameraID  string    `json:"camera_id"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}
