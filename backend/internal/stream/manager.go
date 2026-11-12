package stream

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"nvr-app/internal/config"
	"nvr-app/internal/models"
	"nvr-app/internal/store"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	cfg       *config.Config
	store     *store.MemoryStore
	mu        sync.Mutex
	recorders map[string]*recorder
	online    map[string]bool
	lastFrame map[string]time.Time
}

type recorder struct {
	cmd  *exec.Cmd
	file string
}

type RecordingInfo struct {
	Filename string    `json:"filename"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"mod_time"`
	Day      string    `json:"day"`
}

func NewManager(cfg *config.Config, st *store.MemoryStore) *Manager {
	_ = os.MkdirAll(cfg.RecordingsDir, 0755)
	_ = os.MkdirAll(cfg.PreviewsDir, 0755)
	return &Manager{
		cfg:       cfg,
		store:     st,
		recorders: make(map[string]*recorder),
		online:    make(map[string]bool),
		lastFrame: make(map[string]time.Time),
	}
}

func (m *Manager) Start() {
	for _, cam := range m.store.ListCameras() {
		go m.PublishToMediaMTX(cam)
	}
	ticker := time.NewTicker(time.Duration(m.cfg.PreviewInterval) * time.Second)
	defer ticker.Stop()
	for {
		m.updateAllPreviews()
		<-ticker.C
	}
}

func (m *Manager) updateAllPreviews() {
	for _, cam := range m.store.ListCameras() {
		if cam.Enabled {
			go m.capturePreview(cam)
		}
	}
}

func (m *Manager) capturePreview(cam *models.Camera) {
	outPath := filepath.Join(m.cfg.PreviewsDir, cam.ID+".jpg")
	src := buildRTSPURL(cam.RTSPUrl, cam.Username, cam.Password)
	cmd := exec.Command("ffmpeg", "-y", "-rtsp_transport", "tcp", "-i", src,
		"-frames:v", "1", "-q:v", "5", outPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	err := cmd.Run()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.online[cam.ID] = false
		return
	}
	m.online[cam.ID] = true
	m.lastFrame[cam.ID] = time.Now()
}

func (m *Manager) GetStatus(cam *models.Camera) models.CameraStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	online := m.online[cam.ID]
	preview := ""
	if online {
		preview = "/api/previews/" + cam.ID + ".jpg"
	}
	return models.CameraStatus{
		ID:          cam.ID,
		Name:        cam.Name,
		Online:      online,
		Recording:   cam.Recording,
		PreviewURL:  preview,
		LastFrameAt: m.lastFrame[cam.ID],
		HLSUrl:      fmt.Sprintf("/hls/%s/index.m3u8", cam.ID),
	}
}

func (m *Manager) StartRecording(cameraID string) error {
	cam, err := m.store.GetCamera(cameraID)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.recorders[cameraID]; exists {
		return fmt.Errorf("already recording")
	}
	day := time.Now().Format("2006-01-02")
	dir := filepath.Join(m.cfg.RecordingsDir, cameraID)
	_ = os.MkdirAll(dir, 0755)
	outFile := filepath.Join(dir, day+".mp4")
	if _, err := os.Stat(outFile); err == nil {
		outFile = filepath.Join(dir, day+"_"+time.Now().Format("150405")+".mp4")
	}
	src := buildRTSPURL(cam.RTSPUrl, cam.Username, cam.Password)
	args := []string{"-y", "-rtsp_transport", "tcp", "-i", src,
		"-c:v", "copy", "-c:a", "aac", "-movflags", "+faststart", "-f", "mp4", outFile}
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	m.recorders[cameraID] = &recorder{cmd: cmd, file: outFile}
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		delete(m.recorders, cameraID)
		m.mu.Unlock()
		_ = m.store.SetRecording(cameraID, false)
	}()
	_ = m.store.SetRecording(cameraID, true)
	log.Printf("[record] started %s → %s", cam.Name, outFile)
	return nil
}

func (m *Manager) StopRecording(cameraID string) error {
	m.mu.Lock()
	rec, ok := m.recorders[cameraID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("not recording")
	}
	delete(m.recorders, cameraID)
	m.mu.Unlock()
	if rec.cmd.Process != nil {
		_ = rec.cmd.Process.Signal(os.Interrupt)
		time.Sleep(2 * time.Second)
		_ = rec.cmd.Process.Kill()
	}
	_ = m.store.SetRecording(cameraID, false)
	log.Printf("[record] stopped %s", cameraID)
	return nil
}

func (m *Manager) ExportFragment(cameraID string, start, end time.Time) (string, error) {
	if !end.After(start) {
		return "", fmt.Errorf("invalid time range")
	}
	dir := filepath.Join(m.cfg.RecordingsDir, cameraID)
	day := start.Format("2006-01-02")
	matches, _ := filepath.Glob(filepath.Join(dir, day+"*.mp4"))
	if len(matches) == 0 {
		return "", fmt.Errorf("no recording found for this day")
	}
	src := matches[0]
	outPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_%s_%s.mp4",
		cameraID, start.Format("150405"), end.Format("150405")))
	base := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	ss := start.Sub(base).Seconds()
	to := end.Sub(base).Seconds()
	args := []string{"-y",
		"-ss", fmt.Sprintf("%.3f", ss),
		"-to", fmt.Sprintf("%.3f", to),
		"-i", src,
		"-c", "copy",
		"-avoid_negative_ts", "make_zero",
		outPath,
	}
	cmd := exec.Command("ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg export failed: %v\n%s", err, string(out))
	}
	return outPath, nil
}

func (m *Manager) ListRecordings(cameraID string) ([]RecordingInfo, error) {
	dir := filepath.Join(m.cfg.RecordingsDir, cameraID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []RecordingInfo{}, nil
		}
		return nil, err
	}
	var out []RecordingInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".mp4") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		day := ""
		if len(name) >= 10 {
			day = name[:10]
		}
		out = append(out, RecordingInfo{
			Filename: name,
			Path:     filepath.Join(dir, name),
			Size:     info.Size(),
			ModTime:  info.ModTime(),
			Day:      day,
		})
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].ModTime.After(out[i].ModTime) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func buildRTSPURL(raw, user, pass string) string {
	if user == "" {
		return raw
	}
	if strings.Contains(raw, "@") {
		return raw
	}
	const p = "rtsp://"
	if strings.HasPrefix(strings.ToLower(raw), p) {
		return p + user + ":" + pass + "@" + raw[len(p):]
	}
	return raw
}

func (m *Manager) PublishToMediaMTX(cam *models.Camera) {
	src := buildRTSPURL(cam.RTSPUrl, cam.Username, cam.Password)
	name := cam.ID
	body := fmt.Sprintf(
		`{"source":%q,"sourceOnDemand":true,"rtspTransport":"tcp","sourceOnDemandStartTimeout":"15s","sourceOnDemandCloseAfter":"30s"}`,
		src,
	)
	endpoints := []struct {
		method string
		url    string
	}{
		{http.MethodPost, fmt.Sprintf("%s/v3/config/paths/add/%s", m.cfg.MediaMTXAPI, name)},
		{http.MethodPost, fmt.Sprintf("%s/v3/config/paths/replace/%s", m.cfg.MediaMTXAPI, name)},
		{http.MethodPatch, fmt.Sprintf("%s/v3/config/paths/patch/%s", m.cfg.MediaMTXAPI, name)},
	}
	client := &http.Client{Timeout: 10 * time.Second}
	for _, ep := range endpoints {
		req, err := http.NewRequest(ep.method, ep.url, strings.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[mediamtx] %s %s: %v", ep.method, ep.url, err)
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		log.Printf("[mediamtx] %s path %s -> %s %s", ep.method, name, resp.Status, string(b))
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return
		}
	}
	log.Printf("[mediamtx] failed to publish path %s", name)
}
