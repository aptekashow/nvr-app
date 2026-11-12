package api

import (
	"fmt"
	"net/http"
	"nvr-app/internal/config"
	"nvr-app/internal/models"
	"nvr-app/internal/store"
	"nvr-app/internal/stream"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Server struct {
	cfg    *config.Config
	store  *store.MemoryStore
	stream *stream.Manager
}

func NewRouter(cfg *config.Config, st *store.MemoryStore, sm *stream.Manager) *echo.Echo {
	s := &Server{cfg: cfg, store: st, stream: sm}

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	e.POST("/api/login", s.login)
	e.GET("/api/previews/:id", s.getPreview)

	api := e.Group("/api")
	api.Use(s.jwtMiddleware)

	api.GET("/cameras", s.listCameras)
	api.POST("/cameras", s.createCamera)
	api.GET("/cameras/:id", s.getCamera)
	api.DELETE("/cameras/:id", s.deleteCamera)

	api.POST("/cameras/:id/record/start", s.startRecording)
	api.POST("/cameras/:id/record/stop", s.stopRecording)

	api.POST("/export", s.exportFragment)

	api.GET("/archive/:cameraId/days", s.archiveDays)
	api.GET("/archive/:cameraId/timeline", s.archiveTimeline)
	api.GET("/archive/:cameraId/thumb", s.archiveThumb)

	return e
}

func (s *Server) login(c echo.Context) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}
	user, err := s.store.CheckPassword(req.Username, req.Password)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  user.ID,
		"user": user.Username,
		"exp":  time.Now().Add(72 * time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "token error"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"token": tokenStr,
		"user": map[string]string{"id": user.ID, "username": user.Username},
	})
}

func (s *Server) jwtMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		auth := c.Request().Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || len(auth) <= len("Bearer ") {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
		}
		tokenStr := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if t.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return []byte(s.cfg.JWTSecret), nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
		if err != nil || !token.Valid {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		}
		return next(c)
	}
}

func (s *Server) listCameras(c echo.Context) error {
	cams := s.store.ListCameras()
	result := make([]interface{}, 0, len(cams))
	for _, cam := range cams {
		result = append(result, s.stream.GetStatus(cam))
	}
	return c.JSON(http.StatusOK, result)
}

func (s *Server) createCamera(c echo.Context) error {
	var req struct {
		Name     string `json:"name"`
		RTSPUrl  string `json:"rtsp_url"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}
	if req.Name == "" || req.RTSPUrl == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name and rtsp_url required"})
	}
	cam, err := s.store.CreateCamera(models.CreateCameraRequest{
		Name:     req.Name,
		RTSPUrl:  req.RTSPUrl,
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	s.stream.PublishToMediaMTX(cam)
	return c.JSON(http.StatusCreated, s.stream.GetStatus(cam))
}

func (s *Server) getCamera(c echo.Context) error {
	cam, err := s.store.GetCamera(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}
	return c.JSON(http.StatusOK, s.stream.GetStatus(cam))
}

func (s *Server) deleteCamera(c echo.Context) error {
	id := c.Param("id")
	_ = s.stream.StopRecording(id)
	if err := s.store.DeleteCamera(id); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) startRecording(c echo.Context) error {
	if err := s.stream.StartRecording(c.Param("id")); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "recording started"})
}

func (s *Server) stopRecording(c echo.Context) error {
	if err := s.stream.StopRecording(c.Param("id")); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "recording stopped"})
}

func (s *Server) exportFragment(c echo.Context) error {
	var req struct {
		CameraID  string    `json:"camera_id"`
		Day       string    `json:"day"`
		StartSec  float64   `json:"start_sec"`
		EndSec    float64   `json:"end_sec"`
		StartTime time.Time `json:"start_time"`
		EndTime   time.Time `json:"end_time"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}
	var path string
	var err error
	if req.Day != "" {
		path, err = s.stream.ExportFragmentByOffset(req.CameraID, req.Day, req.StartSec, req.EndSec)
	} else {
		path, err = s.stream.ExportFragment(req.CameraID, req.StartTime, req.EndTime)
	}
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.Attachment(path, filepath.Base(path))
}

func (s *Server) getPreview(c echo.Context) error {
	name := c.Param("id")
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, "/\\") {
		return c.NoContent(http.StatusBadRequest)
	}
	path := filepath.Join(s.cfg.PreviewsDir, name)
	if _, err := os.Stat(path); err != nil {
		return c.NoContent(http.StatusNotFound)
	}
	return c.File(path)
}

func (s *Server) archiveDays(c echo.Context) error {
	days, err := s.stream.ListRecordingDays(c.Param("cameraId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, days)
}

func (s *Server) archiveTimeline(c echo.Context) error {
	day := c.QueryParam("day")
	if day == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "day required (YYYY-MM-DD)"})
	}
	info, err := s.stream.GetTimeline(c.Param("cameraId"), day)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, info)
}

func (s *Server) archiveThumb(c echo.Context) error {
	day := c.QueryParam("day")
	offset := c.QueryParam("offset")
	if day == "" || offset == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "day and offset required"})
	}
	var sec float64
	if _, err := fmt.Sscanf(offset, "%f", &sec); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad offset"})
	}
	path, err := s.stream.ExtractThumb(c.Param("cameraId"), day, sec)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.File(path)
}
