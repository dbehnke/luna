package video

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"luna/internal/id"
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
	SampleRate int     `json:"sample_rate"`
	Channels   int     `json:"channels"`
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
				if sr, ok := stream["sample_rate"].(string); ok {
					if r, err := strconv.Atoi(sr); err == nil {
						probe.SampleRate = r
					}
				}
				if ch, ok := stream["channels"].(float64); ok {
					probe.Channels = int(ch)
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
	defer func() { _ = os.Remove(tmpOutput) }()

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

func (p *Processor) TranscodeAudio(itemID string, probeResult *ProbeResult) error {
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

	outputPath := filepath.Join(derivedDir, "master.m4a")

	tmpOutput := outputPath + ".tmp"
	defer func() { _ = os.Remove(tmpOutput) }()

	args := AudioTranscodeArgs(originalFile, tmpOutput)

	cmd := exec.Command("ffmpeg", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg audio transcode failed: %w, stderr: %s", err, stderr.String())
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

	updated := false
	for i := range assetsMeta.Assets {
		if assetsMeta.Assets[i].Kind == "master_m4a" {
			assetsMeta.Assets[i] = meta.Asset{
				Kind:        "master_m4a",
				StoragePath: "derived/master.m4a",
				Codecs:      "aac",
				Bitrate:     "128k",
			}
			updated = true
			break
		}
	}
	if !updated {
		assetsMeta.Assets = append(assetsMeta.Assets, meta.Asset{
			Kind:        "master_m4a",
			StoragePath: "derived/master.m4a",
			Codecs:      "aac",
			Bitrate:     "128k",
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

		tmpOutput := outputPath + ".tmp.webp"
		defer func() { _ = os.Remove(tmpOutput) }()

		cmd := exec.Command("ffmpeg",
			"-y",
			"-ss", fmt.Sprintf("%.2f", timestamp),
			"-i", originalFile,
			"-vframes", "1",
			"-vf", "scale=480:-2",
			"-f", "webp",
			"-c:v", "libwebp",
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
		DurationMs:      int64(probe.Duration * 1000),
		Width:           probe.Width,
		Height:          probe.Height,
		Rotation:        probe.Rotation,
		VideoCodec:      probe.VideoCodec,
		AudioCodec:      probe.AudioCodec,
		Bitrate:         probe.Bitrate,
		AudioSampleRate: probe.SampleRate,
		AudioChannels:   probe.Channels,
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
			Width:      assetsMeta.SourceInfo.Width,
			Height:     assetsMeta.SourceInfo.Height,
			Rotation:   assetsMeta.SourceInfo.Rotation,
			AudioCodec: assetsMeta.SourceInfo.AudioCodec,
			SampleRate: assetsMeta.SourceInfo.AudioSampleRate,
			Channels:   assetsMeta.SourceInfo.AudioChannels,
		}
	}

	// Check if this is an audio item (no video codec, but has audio codec)
	if assetsMeta != nil && assetsMeta.SourceInfo != nil && assetsMeta.SourceInfo.VideoCodec == "" && assetsMeta.SourceInfo.AudioCodec != "" {
		return p.TranscodeAudio(itemID, &probeResult)
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
	defer func() { _ = os.Remove(tmpOutput) }()

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

func (p *Processor) ProcessHLS(itemID string) error {
	inputPath := filepath.Join(p.mediaRoot, "items", itemID, "derived", "master.mp4")
	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("master file not found: %w", err)
	}

	assetsMeta, err := meta.ReadAssetsMetaByID(p.mediaRoot, itemID)
	if err != nil {
		return fmt.Errorf("read assets meta: %w", err)
	}

	if assetsMeta == nil || assetsMeta.SourceInfo == nil {
		return fmt.Errorf("no source info found, transcode may not have completed")
	}

	sourceWidth := assetsMeta.SourceInfo.Width
	sourceHeight := assetsMeta.SourceInfo.Height

	hlsDir := filepath.Join(p.mediaRoot, "items", itemID, "derived", "hls")
	tmpDir := hlsDir + "_tmp_" + id.NewULID()

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("create hls tmp dir: %w", err)
	}

	variants := DefaultHLSVariants()
	args := HLSArgs(inputPath, tmpDir, variants, sourceWidth, sourceHeight)

	cmd := exec.Command("ffmpeg", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("ffmpeg hls failed: %w, stderr: %s", err, stderr.String())
	}

	indexPath := filepath.Join(tmpDir, "index.m3u8")
	if _, err := os.Stat(indexPath); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("hls index.m3u8 not created: %w", err)
	}

	if err := os.Rename(tmpDir, hlsDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("rename hls dir: %w", err)
	}

	hlsVariants := []meta.HLSVariant{}
	for _, v := range variants {
		if v.Height <= sourceHeight {
			hlsVariants = append(hlsVariants, meta.HLSVariant{
				Height:    v.Height,
				Bandwidth: v.Bandwidth,
			})
		}
	}

	assetsMeta, err = meta.ReadAssetsMetaByID(p.mediaRoot, itemID)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read assets meta: %w", err)
	}
	if assetsMeta == nil {
		assetsMeta = &meta.AssetsMeta{Schema: meta.SchemaVersion}
	}

	hlsFound := false
	for i := range assetsMeta.Assets {
		if assetsMeta.Assets[i].Kind == "hls" {
			assetsMeta.Assets[i] = meta.Asset{
				Kind:        "hls",
				StoragePath: "derived/hls/index.m3u8",
				Variants:    hlsVariants,
			}
			hlsFound = true
			break
		}
	}
	if !hlsFound {
		assetsMeta.Assets = append(assetsMeta.Assets, meta.Asset{
			Kind:        "hls",
			StoragePath: "derived/hls/index.m3u8",
			Variants:    hlsVariants,
		})
	}

	if err := meta.WriteAssetsMetaAtomic(p.mediaRoot, itemID, assetsMeta); err != nil {
		return fmt.Errorf("write assets meta: %w", err)
	}

	return nil
}

func (p *Processor) ProcessPhoto(itemID string) error {
	inputPath := filepath.Join(p.mediaRoot, "items", itemID, "original")
	entries, err := os.ReadDir(inputPath)
	if err != nil {
		return fmt.Errorf("read original dir: %w", err)
	}

	var originalEntry string
	var originalFile string
	for _, e := range entries {
		if !e.IsDir() {
			originalEntry = e.Name()
			originalFile = filepath.Join(inputPath, e.Name())
			break
		}
	}

	if originalFile == "" {
		return fmt.Errorf("no original file found")
	}

	photosDir := filepath.Join(p.mediaRoot, "items", itemID, "photos")
	if err := os.MkdirAll(photosDir, 0755); err != nil {
		return fmt.Errorf("create photos dir: %w", err)
	}

	displayPath := filepath.Join(photosDir, "display.webp")
	thumbPath := filepath.Join(photosDir, "thumb.webp")

	writeResized := func(outputPath string, size int) error {
		tmpOutput := outputPath + ".tmp.webp"
		defer func() { _ = os.Remove(tmpOutput) }()

		cmd := exec.Command("ffmpeg",
			"-y",
			"-i", originalFile,
			"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", size, size),
			"-vframes", "1",
			"-f", "webp",
			"-c:v", "libwebp",
			tmpOutput,
		)

		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("ffmpeg photo resize failed: %w, stderr: %s", err, stderr.String())
		}

		if err := os.Rename(tmpOutput, outputPath); err != nil {
			return fmt.Errorf("rename photo output: %w", err)
		}
		return nil
	}

	if err := writeResized(displayPath, 2048); err != nil {
		return err
	}
	if err := writeResized(thumbPath, 480); err != nil {
		return err
	}

	displayW, displayH := probeImageDimensions(displayPath)
	thumbW, thumbH := probeImageDimensions(thumbPath)

	assetsMeta, err := meta.ReadAssetsMetaByID(p.mediaRoot, itemID)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read assets meta: %w", err)
	}
	if assetsMeta == nil {
		assetsMeta = &meta.AssetsMeta{Schema: meta.SchemaVersion}
	}

	upsertPhoto := func(kind, storagePath string, width, height int) {
		for i := range assetsMeta.Photos {
			if assetsMeta.Photos[i].Kind == kind {
				assetsMeta.Photos[i] = meta.Photo{
					Kind:        kind,
					StoragePath: storagePath,
					Width:       width,
					Height:      height,
				}
				return
			}
		}
		assetsMeta.Photos = append(assetsMeta.Photos, meta.Photo{
			Kind:        kind,
			StoragePath: storagePath,
			Width:       width,
			Height:      height,
		})
	}

	upsertPhoto("original", "original/"+originalEntry, 0, 0)
	upsertPhoto("display", "photos/display.webp", displayW, displayH)
	upsertPhoto("thumb", "photos/thumb.webp", thumbW, thumbH)

	if err := meta.WriteAssetsMetaAtomic(p.mediaRoot, itemID, assetsMeta); err != nil {
		return fmt.Errorf("write assets meta: %w", err)
	}

	return nil
}

func probeImageDimensions(path string) (int, int) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		path,
	)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return 0, 0
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return 0, 0
	}

	streams, ok := result["streams"].([]interface{})
	if !ok {
		return 0, 0
	}

	for _, s := range streams {
		stream, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		w, wok := stream["width"].(float64)
		h, hok := stream["height"].(float64)
		if wok && hok {
			return int(w), int(h)
		}
	}

	return 0, 0
}
