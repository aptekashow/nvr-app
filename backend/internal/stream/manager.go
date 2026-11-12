package stream

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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
	cfg           *config.Config
	store         *store.MemoryStore
	mu            sync.Mutex
	recorders     map[string]*recorder
	online        map[string]bool
	lastFrame     map[string]time.Time
	previewRunning map[string]bool
	previewSlots  chan struct{}
}

type recorder struct {
	cmd      *exec.Cmd
	file     string
	done     chan struct{}
	stopping bool
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
		cfg:            cfg,
		store:          st,
		recorders:      make(map[string]*recorder),
		online:         make(map[string]bool),
		lastFrame:      make(map[string]time.Time),
		previewRunning: make(map[string]bool),
		previewSlots:   make(chan struct{}, 2),
	}
}

func (m *Manager) mediaMTXStreamURL(cameraID string) string {
	base := strings.TrimRight(m.cfg.MediaMTXRTSP, "/")
	return base + "/" + url.PathEscape(cameraID)
}

// Start registers all camera paths before any preview or recording reader is
// started. MediaMTX owns the single upstream RTSP connection per camera.
func (m *Manager) Start() {
	for _, cam := range m.store.ListCameras() {
		m.PublishToMediaMTX(cam)
	}
	m.updateAllPreviews()
	ticker := time.NewTicker(time.Duration(m.cfg.PreviewInterval) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		m.updateAllPreviews()
	}
}

func (m *Manager) updateAllPreviews() {
	for _, cam := range m.store.ListCameras() {
		if !cam.Enabled {
			continue
		}
		// Do not create an unbounded queue of goroutines when there are many
		// cameras. Skip this cycle when both preview workers are busy.
		select {
		case m.previewSlots <- struct{}{}:
		default:
			continue
		}
		m.mu.Lock()
		if m.previewRunning[cam.ID] {
			m.mu.Unlock()
			<-m.previewSlots
			continue
		}
		m.previewRunning[cam.ID] = true
		m.mu.Unlock()

		go func(camera *models.Camera) {
			defer func() {
				<-m.previewSlots
				m.mu.Lock()
				delete(m.previewRunning, camera.ID)
				m.mu.Unlock()
			}()
			m.capturePreview(camera)
		}(cam)
	}
}

// Preview capture reads the MediaMTX relay, never the physical camera. This
// avoids opening competing RTSP sessions and limits thumbnail work to 2 jobs.
func (m *Manager) capturePreview(cam *models.Camera) {
	outPath := filepath.Join(m.cfg.PreviewsDir, cam.ID+".jpg")
	tmpPath := filepath.Join(m.cfg.PreviewsDir, cam.ID+".tmp.jpg")

	// MediaMTX can need up to 15 seconds to start an on-demand RTSP source.
	// The old 8-second deadline killed FFmpeg before that startup window ended.
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-rtsp_transport", "tcp", "-rw_timeout", "20000000",
		"-i", m.mediaMTXStreamURL(cam.ID),
		"-an", "-frames:v", "1", "-vf", "scale=640:-2", "-q:v", "7",
		tmpPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(tmpPath)
		m.mu.Lock()
		// Keep the last good frame visible; only mark the camera offline when
		// there has never been a frame or the last successful frame is stale.
		staleAfter := time.Duration(m.cfg.PreviewInterval*3) * time.Second
		if m.lastFrame[cam.ID].IsZero() || time.Since(m.lastFrame[cam.ID]) > staleAfter {
			m.online[cam.ID] = false
		}
		m.mu.Unlock()
		log.Printf("[preview] camera %s failed (timeout=%t): %v: %s",
			cam.ID, ctx.Err() != nil, err, strings.TrimSpace(string(output)))
		return
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		_ = os.Remove(tmpPath)
		log.Printf("[preview] camera %s cannot publish JPEG: %v", cam.ID, err)
		return
	}
	m.mu.Lock()
	m.online[cam.ID] = true
	m.lastFrame[cam.ID] = time.Now()
	m.mu.Unlock()
}

func (m *Manager) GetStatus(cam *models.Camera) models.CameraStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	preview := ""
	if m.online[cam.ID] {
		preview = "/api/previews/" + cam.ID + ".jpg"
	}
	return models.CameraStatus{
		ID:          cam.ID,
		Name:        cam.Name,
		Online:      m.online[cam.ID],
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
	if _, exists := m.recorders[cameraID]; exists {
		m.mu.Unlock()
		return fmt.Errorf("already recording")
	}

	dir := filepath.Join(m.cfg.RecordingsDir, cameraID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("create recording directory: %w", err)
	}
	outFile := filepath.Join(dir, time.Now().Format("2006-01-02_15-04-05.000")+".mp4")

	// Read from MediaMTX instead of opening the physical camera a second time.
	// Both video and audio are stream-copied: no codec conversion is performed.
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-n",
		"-rtsp_transport", "tcp", "-rw_timeout", "15000000",
		"-i", m.mediaMTXStreamURL(cameraID),
		"-map", "0:v:0", "-map", "0:a?",
		"-c", "copy",
		"-movflags", "+frag_keyframe+empty_moov+default_base_moof",
		"-flush_packets", "1",
		"-f", "mp4", outFile,
	}
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("failed to start stream-copy recorder: %w", err)
	}

	rec := &recorder{cmd: cmd, file: outFile, done: make(chan struct{})}
	m.recorders[cameraID] = rec
	if err := m.store.SetRecording(cameraID, true); err != nil {
		delete(m.recorders, cameraID)
		m.mu.Unlock()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("update recording status: %w", err)
	}
	m.mu.Unlock()

	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		stopping := rec.stopping
		if current := m.recorders[cameraID]; current == rec {
			delete(m.recorders, cameraID)
		}
		m.mu.Unlock()
		if err != nil && !stopping {
			log.Printf("[record] camera %s recorder stopped: %v", cameraID, err)
		}
		_ = m.store.SetRecording(cameraID, false)
		close(rec.done)
	}()

	log.Printf("[record] started stream-copy %s -> %s", cam.Name, outFile)
	return nil
}

func (m *Manager) StopRecording(cameraID string) error {
	m.mu.Lock()
	rec, ok := m.recorders[cameraID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("not recording")
	}
	if rec.stopping {
		done := rec.done
		m.mu.Unlock()
		<-done
		return nil
	}
	rec.stopping = true
	m.mu.Unlock()

	// SIGINT lets FFmpeg close the fragmented MP4 cleanly. Kill only if it
	// doesn't exit within the grace period.
	if rec.cmd.Process != nil {
		_ = rec.cmd.Process.Signal(os.Interrupt)
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-rec.done:
		return nil
	case <-timer.C:
		if rec.cmd.Process != nil {
			_ = rec.cmd.Process.Kill()
		}
		<-rec.done
		return nil
	}
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
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "rtsp" && parsed.Scheme != "rtsps") || parsed.Hostname() == "" {
		return raw
	}
	// Credentials embedded in the URL take precedence over separate fields.
	if parsed.User == nil && user != "" {
		parsed.User = url.UserPassword(user, pass)
	}
	return parsed.String()
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

func (m *Manager) RemoveFromMediaMTX(cameraID string) {
	endpoint := fmt.Sprintf("%s/v3/config/paths/delete/%s", m.cfg.MediaMTXAPI, cameraID)
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		log.Printf("[mediamtx] delete path %s: %v", cameraID, err)
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[mediamtx] delete path %s: %v", cameraID, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[mediamtx] delete path %s -> %s %s", cameraID, resp.Status, string(body))
	}
}
