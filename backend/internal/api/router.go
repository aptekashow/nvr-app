package api

import (
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
	api.GET("/archive", s.archiveStub)
	api.GET("/archive/:cameraId", s.listRecordings)

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
		"user": map[string]string{
			"id":       user.ID,
			"username": user.Username,
		},
	})
}

func (s *Server) jwtMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		auth := c.Request().Header.Get("Authorization")
		if auth == "" || len(auth) < 8 {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing token"})
		}
		tokenStr := auth[7:]
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			return []byte(s.cfg.JWTSecret), nil
		})
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
		status := s.stream.GetStatus(cam)
		result = append(result, status)
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
	id := c.Param("id")
	cam, err := s.store.GetCamera(id)
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
	id := c.Param("id")
	if err := s.stream.StartRecording(id); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "recording started"})
}

func (s *Server) stopRecording(c echo.Context) error {
	id := c.Param("id")
	if err := s.stream.StopRecording(id); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "recording stopped"})
}

func (s *Server) exportFragment(c echo.Context) error {
	var req struct {
		CameraID  string    `json:"camera_id"`
		StartTime time.Time `json:"start_time"`
		EndTime   time.Time `json:"end_time"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}

	path, err := s.stream.ExportFragment(req.CameraID, req.StartTime, req.EndTime)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.Attachment(path, filepath.Base(path))
}

func (s *Server) getPreview(c echo.Context) error {
	id := c.Param("id")
	path := filepath.Join(s.cfg.PreviewsDir, id)
	if _, err := os.Stat(path); err != nil {
		return c.NoContent(http.StatusNotFound)
	}
	return c.File(path)
}

func (s *Server) archiveStub(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"message": "Архив в разработке",
	})
}

// listRecordings returns list of recording files for a camera (for Archive page)
func (s *Server) listRecordings(c echo.Context) error {
	cameraID := c.Param("cameraId")
	list, err := s.stream.ListRecordings(cameraID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, list)
}
