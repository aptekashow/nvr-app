package stream

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ListRecordingDays returns unique days (YYYY-MM-DD) that have recordings.
func (m *Manager) ListRecordingDays(cameraID string) ([]string, error) {
	recs, err := m.ListRecordings(cameraID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var days []string
	for _, r := range recs {
		if r.Day == "" || seen[r.Day] {
			continue
		}
		seen[r.Day] = true
		days = append(days, r.Day)
	}
	return days, nil
}

type TimelineInfo struct {
	Day      string    `json:"day"`
	Filename string    `json:"filename"`
	Duration float64   `json:"duration"`
	Thumbs   []float64 `json:"thumbs"`
}

func (m *Manager) findDayFile(cameraID, day string) (string, error) {
	dir := filepath.Join(m.cfg.RecordingsDir, cameraID)
	matches, _ := filepath.Glob(filepath.Join(dir, day+"*.mp4"))
	if len(matches) == 0 {
		return "", fmt.Errorf("no recording for day %s", day)
	}
	exact := filepath.Join(dir, day+".mp4")
	for _, mth := range matches {
		if mth == exact {
			return exact, nil
		}
	}
	return matches[0], nil
}

func probeDuration(path string) (float64, error) {
	cmd := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	var d float64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &d); err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	return d, nil
}

func (m *Manager) GetTimeline(cameraID, day string) (*TimelineInfo, error) {
	file, err := m.findDayFile(cameraID, day)
	if err != nil {
		return nil, err
	}
	dur, err := probeDuration(file)
	if err != nil {
		dur = 3600
	}
	step := dur / 24
	if step < 30 {
		step = 30
	}
	if step > 300 {
		step = 300
	}
	var thumbs []float64
	for t := 0.0; t < dur; t += step {
		thumbs = append(thumbs, t)
	}
	if len(thumbs) == 0 {
		thumbs = []float64{0}
	}
	return &TimelineInfo{
		Day:      day,
		Filename: filepath.Base(file),
		Duration: dur,
		Thumbs:   thumbs,
	}, nil
}

func (m *Manager) ExtractThumb(cameraID, day string, offsetSec float64) (string, error) {
	src, err := m.findDayFile(cameraID, day)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(m.cfg.PreviewsDir, "timeline", cameraID, day)
	_ = os.MkdirAll(dir, 0755)
	key := int(offsetSec)
	out := filepath.Join(dir, fmt.Sprintf("%d.jpg", key))
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil
	}
	cmd := exec.Command("ffmpeg", "-y",
		"-ss", fmt.Sprintf("%.2f", offsetSec),
		"-i", src,
		"-frames:v", "1",
		"-q:v", "6",
		"-vf", "scale=160:-1",
		out,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("thumb extract failed: %w", err)
	}
	return out, nil
}

func (m *Manager) ExportFragmentByOffset(cameraID, day string, startSec, endSec float64) (string, error) {
	if endSec <= startSec {
		return "", fmt.Errorf("invalid range")
	}
	src, err := m.findDayFile(cameraID, day)
	if err != nil {
		return "", err
	}
	outPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_%s_%.0f_%.0f.mp4", cameraID, day, startSec, endSec))
	args := []string{
		"-y",
		"-ss", fmt.Sprintf("%.3f", startSec),
		"-to", fmt.Sprintf("%.3f", endSec),
		"-i", src,
		"-c", "copy",
		"-avoid_negative_ts", "make_zero",
		outPath,
	}
	cmd := exec.Command("ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("export failed: %v\n%s", err, string(out))
	}
	return outPath, nil
}
