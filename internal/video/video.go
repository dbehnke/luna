package video

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"luna/internal/meta"
)

type Processor struct {
	mediaRoot string
}

func NewProcessor(mediaRoot string) *Processor {
	return &Processor{mediaRoot: mediaRoot}
}

func (p *Processor) CheckFFmpeg() (bool, string) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return false, "ffmpeg not found"
	}
	return true, path
}

func (p *Processor) CheckFFprobe() (bool, string) {
	path, err := exec.LookPath("ffprobe")
	if err != nil {
		return false, "ffprobe not found"
	}
	return true, path
}

type ProbeResult struct {
	Duration   float64 `json:"duration"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Rotation   int     `json:"rotation"`
	VideoCodec string  `json:"video_codec"`
	AudioCodec string  `json:"audio_codec"`
	Bitrate    string  `json:"bitrate"`
}

func (p *Processor) Probe(itemID string) (*ProbeResult, error) {
	inputPath := filepath.Join(p.mediaRoot, "items", itemID, "original")
	entries, err := os.ReadDir(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read original dir: %w", err)
	}

	var originalFile string
	for _, e := range entries {
		if !e.IsDir() {
			originalFile = filepath.Join(inputPath, e.Name())
			break
		}
	}

	if originalFile == "" {
		return nil, fmt.Errorf("no original file found")
	}

	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		originalFile,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w, stderr: %s", err, stderr.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	probe := &ProbeResult{}

	if format, ok := result["format"].(map[string]interface{}); ok {
		if dur, ok := format["duration"].(string); ok {
			if d, err := strconv.ParseFloat(dur, 64); err == nil {
				probe.Duration = d
			}
		}
		if br, ok := format["bit_rate"].(string); ok {
			probe.Bitrate = br
		}
	}

	if streams, ok := result["streams"].([]interface{}); ok {
		for _, s := range streams {
			stream, ok := s.(map[string]interface{})
			if !ok {
				continue
			}

			codecType, _ := stream["codec_type"].(string)
			if codecType == "video" {
				if w, ok := stream["width"].(float64); ok {
					probe.Width = int(w)
				}
				if h, ok := stream["height"].(float64); ok {
					probe.Height = int(h)
				}
				if vc, ok := stream["codec_name"].(string); ok {
					probe.VideoCodec = vc
				}

				if rot, ok := stream["rotation"].(float64); ok {
					probe.Rotation = int(rot)
				} else if tags, ok := stream["tags"].(map[string]interface{}); ok {
					if rot, ok := tags["rotate"].(string); ok {
						if r, err := strconv.Atoi(rot); err == nil {
							probe.Rotation = r
						}
					}
				}
			}
			if codecType == "audio" {
				if ac, ok := stream["codec_name"].(string); ok {
					probe.AudioCodec = ac
				}
			}
		}
	}

	return probe, nil
}

func (p *Processor) Transcode(itemID string, probeResult *ProbeResult) error {
	inputPath := filepath.Join(p.mediaRoot, "items", itemID, "original")
	entries, err := os.ReadDir(inputPath)
	if err != nil {
		return fmt.Errorf("read original dir: %w", err)
	}

	var originalFile string
	for _, e := range entries {
		if !e.IsDir() {
			originalFile = filepath.Join(inputPath, e.Name())
			break
		}
	}

	if originalFile == "" {
		return fmt.Errorf("no original file found")
	}

	derivedDir := filepath.Join(p.mediaRoot, "items", itemID, "derived")
	if err := os.MkdirAll(derivedDir, 0755); err != nil {
		return fmt.Errorf("create derived dir: %w", err)
	}

	outputPath := filepath.Join(derivedDir, "master.mp4")

	tmpOutput := outputPath + ".tmp"
	defer os.Remove(tmpOutput)

	args := MasterTranscodeArgs(originalFile, tmpOutput, *probeResult)

	cmd := exec.Command("ffmpeg", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg transcode failed: %w, stderr: %s", err, stderr.String())
	}

	if err := os.Rename(tmpOutput, outputPath); err != nil {
		return fmt.Errorf("rename output: %w", err)
	}

	assetsMeta, err := meta.ReadAssetsMetaByID(p.mediaRoot, itemID)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read assets meta: %w", err)
	}
	if assetsMeta == nil {
		assetsMeta = &meta.AssetsMeta{Schema: meta.SchemaVersion}
	}

	width := probeResult.Width
	height := probeResult.Height
	rotation := probeResult.Rotation

	if rotation == 90 || rotation == 270 {
		width, height = height, width
	}

	updated := false
	for i := range assetsMeta.Assets {
		if assetsMeta.Assets[i].Kind == "master_mp4" {
			assetsMeta.Assets[i] = meta.Asset{
				Kind:        "master_mp4",
				StoragePath: "derived/master.mp4",
				Width:       width,
				Height:      height,
				Codecs:      "h264/aac",
			}
			updated = true
			break
		}
	}
	if !updated {
		assetsMeta.Assets = append(assetsMeta.Assets, meta.Asset{
			Kind:        "master_mp4",
			StoragePath: "derived/master.mp4",
			Width:       width,
			Height:      height,
			Codecs:      "h264/aac",
		})
	}

	if err := meta.WriteAssetsMetaAtomic(p.mediaRoot, itemID, assetsMeta); err != nil {
		return fmt.Errorf("write assets meta: %w", err)
	}

	return nil
}

func (p *Processor) GenerateThumbnails(itemID string, duration float64) error {
	if duration <= 0 {
		return fmt.Errorf("invalid duration: %f", duration)
	}

	thumbsDir := filepath.Join(p.mediaRoot, "items", itemID, "thumbs")
	if err := os.MkdirAll(thumbsDir, 0755); err != nil {
		return fmt.Errorf("create thumbs dir: %w", err)
	}

	inputPath := filepath.Join(p.mediaRoot, "items", itemID, "original")
	entries, err := os.ReadDir(inputPath)
	if err != nil {
		return fmt.Errorf("read original dir: %w", err)
	}

	var originalFile string
	for _, e := range entries {
		if !e.IsDir() {
			originalFile = filepath.Join(inputPath, e.Name())
			break
		}
	}

	if originalFile == "" {
		return fmt.Errorf("no original file found")
	}

	percentages := []float64{0.1, 0.5, 0.9}

	assetsMeta, err := meta.ReadAssetsMetaByID(p.mediaRoot, itemID)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read assets meta: %w", err)
	}
	if assetsMeta == nil {
		assetsMeta = &meta.AssetsMeta{Schema: meta.SchemaVersion}
	}

	for i, pct := range percentages {
		timestamp := duration * pct
		outputPath := filepath.Join(thumbsDir, fmt.Sprintf("t_%04d.webp", i+1))

		tmpOutput := outputPath + ".tmp"
		defer os.Remove(tmpOutput)

		cmd := exec.Command("ffmpeg",
			"-y",
			"-ss", fmt.Sprintf("%.2f", timestamp),
			"-i", originalFile,
			"-vframes", "1",
			"-vf", "scale=480:-2",
			"-lossless", "1",
			tmpOutput,
		)

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("ffmpeg thumb failed: %w, stderr: %s", err, stderr.String())
		}

		if err := os.Rename(tmpOutput, outputPath); err != nil {
			return fmt.Errorf("rename thumb: %w", err)
		}

		assetsMeta.Thumbnails = append(assetsMeta.Thumbnails, meta.Thumbnail{
			StoragePath: fmt.Sprintf("thumbs/t_%04d.webp", i+1),
			Timestamp:   fmt.Sprintf("%.0f%%", pct*100),
		})
	}

	if err := meta.WriteAssetsMetaAtomic(p.mediaRoot, itemID, assetsMeta); err != nil {
		return fmt.Errorf("write assets meta: %w", err)
	}

	return nil
}

func (p *Processor) ProcessProbe(itemID string) error {
	probe, err := p.Probe(itemID)
	if err != nil {
		return fmt.Errorf("probe failed: %w", err)
	}

	assetsMeta, err := meta.ReadAssetsMetaByID(p.mediaRoot, itemID)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read assets meta: %w", err)
	}
	if assetsMeta == nil {
		assetsMeta = &meta.AssetsMeta{Schema: meta.SchemaVersion}
	}

	assetsMeta.SourceInfo = &meta.SourceInfo{
		DurationMs: int64(probe.Duration * 1000),
		Width:      probe.Width,
		Height:     probe.Height,
		Rotation:   probe.Rotation,
		VideoCodec: probe.VideoCodec,
		AudioCodec: probe.AudioCodec,
		Bitrate:    probe.Bitrate,
	}

	if err := meta.WriteAssetsMetaAtomic(p.mediaRoot, itemID, assetsMeta); err != nil {
		return fmt.Errorf("write assets meta: %w", err)
	}

	return nil
}

func (p *Processor) ProcessTranscode(itemID string) error {
	assetsMeta, err := meta.ReadAssetsMetaByID(p.mediaRoot, itemID)
	if err != nil {
		return fmt.Errorf("read assets meta: %w", err)
	}

	var probeResult ProbeResult
	if assetsMeta != nil && assetsMeta.SourceInfo != nil {
		probeResult = ProbeResult{
			Width:    assetsMeta.SourceInfo.Width,
			Height:   assetsMeta.SourceInfo.Height,
			Rotation: assetsMeta.SourceInfo.Rotation,
		}
	}

	return p.Transcode(itemID, &probeResult)
}

func (p *Processor) ProcessThumbs(itemID string) error {
	assetsMeta, err := meta.ReadAssetsMetaByID(p.mediaRoot, itemID)
	if err != nil {
		return fmt.Errorf("read assets meta: %w", err)
	}

	if assetsMeta == nil || assetsMeta.SourceInfo == nil {
		return fmt.Errorf("no source info found, probe may not have completed")
	}

	durationMs := assetsMeta.SourceInfo.DurationMs
	if durationMs <= 0 {
		return fmt.Errorf("invalid duration from source info")
	}

	duration := float64(durationMs) / 1000.0
	return p.GenerateThumbnails(itemID, duration)
}

const (
	ClipWidth  = 1080
	ClipHeight = 1920
	CropMode   = "9:16_center"
)

func (p *Processor) ProcessClip(itemID, clipID string, startMs, endMs int64) error {
	durationMs := endMs - startMs
	if durationMs <= 0 {
		return fmt.Errorf("invalid clip duration: start=%d end=%d", startMs, endMs)
	}
	if durationMs > 90000 {
		return fmt.Errorf("clip exceeds max duration: %dms > 90000ms", durationMs)
	}

	inputPath := filepath.Join(p.mediaRoot, "items", itemID, "derived", "master.mp4")
	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("master file not found: %w", err)
	}

	derivedDir := filepath.Join(p.mediaRoot, "items", itemID, "derived")
	if err := os.MkdirAll(derivedDir, 0755); err != nil {
		return fmt.Errorf("create derived dir: %w", err)
	}

	outputPath := filepath.Join(derivedDir, fmt.Sprintf("short_%s.mp4", clipID))

	tmpOutput := outputPath + ".tmp"
	defer os.Remove(tmpOutput)

	args := ClipTranscodeArgs(inputPath, tmpOutput, ProbeResult{}, startMs, endMs)

	cmd := exec.Command("ffmpeg", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg clip failed: %w, stderr: %s", err, stderr.String())
	}

	if err := os.Rename(tmpOutput, outputPath); err != nil {
		return fmt.Errorf("rename output: %w", err)
	}

	assetsMeta, err := meta.ReadAssetsMetaByID(p.mediaRoot, itemID)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read assets meta: %w", err)
	}
	if assetsMeta == nil {
		assetsMeta = &meta.AssetsMeta{Schema: meta.SchemaVersion}
	}

	assetsMeta.Assets = append(assetsMeta.Assets, meta.Asset{
		Kind:        "short_clip",
		StoragePath: fmt.Sprintf("derived/short_%s.mp4", clipID),
		ClipID:      clipID,
		StartMs:     startMs,
		EndMs:       endMs,
		CropMode:    CropMode,
		Width:       ClipWidth,
		Height:      ClipHeight,
	})

	if err := meta.WriteAssetsMetaAtomic(p.mediaRoot, itemID, assetsMeta); err != nil {
		return fmt.Errorf("write assets meta: %w", err)
	}

	return nil
}
