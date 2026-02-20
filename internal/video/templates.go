// Package video provides video processing capabilities for Luna.
//
// WARNING: These ffmpeg templates are the single source of truth.
// Do not inline ffmpeg arguments elsewhere in the codebase.
// All encoding decisions must be made through these template functions.
//
// This ensures:
// - Consistent encoding behavior across all video processing
// - Safe and reviewable future changes to encoding settings
// - Reuse of encoding logic between master videos and shorts/clips
package video

import (
	"fmt"
	"strings"
)

// MasterTranscodeArgs builds ffmpeg arguments for transcoding a video to master MP4.
//
// Requirements:
// - H.264 video codec + AAC audio codec
// - Faststart enabled (moov atom at beginning for better streaming)
// - Max resolution: 1080p (downscale only, never upscale)
// - Rotation metadata baked into pixels
// - yuv420p pixel format for compatibility
// - Uses medium preset with CRF 23
func MasterTranscodeArgs(inPath string, outPath string, probe ProbeResult) []string {
	args := []string{
		"-y",
		"-i", inPath,
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
	}

	width := probe.Width
	height := probe.Height
	rotation := probe.Rotation

	// Swap dimensions if rotated 90/270
	if rotation == 90 || rotation == 270 {
		width, height = height, width
	}

	// Scale down to max 1080p (never upscale)
	if width > 1920 || height > 1080 {
		if width > height {
			args = append(args, "-vf", "scale=-2:1080")
		} else {
			args = append(args, "-vf", "scale=1080:-2")
		}
	}

	// Handle rotation by baking it into pixels
	if rotation != 0 {
		rotateFilter := ""
		switch rotation {
		case 90:
			rotateFilter = "transpose=1"
		case 180:
			rotateFilter = "transpose=2,transpose=2"
		case 270:
			rotateFilter = "transpose=2"
		}
		if rotateFilter != "" {
			// Check if we already have a -vf filter
			hasVF := false
			for _, arg := range args {
				if arg == "-vf" {
					hasVF = true
					break
				}
			}
			if hasVF {
				// Prepend rotation to existing filter
				for i, arg := range args {
					if arg == "-vf" {
						args[i+1] = rotateFilter + "," + args[i+1]
						break
					}
				}
			} else {
				args = append(args, "-vf", rotateFilter)
			}
		}
	}

	// Ensure yuv420p for maximum compatibility
	args = append(args, "-pix_fmt", "yuv420p")

	args = append(args, outPath)

	return args
}

// ClipTranscodeArgs builds ffmpeg arguments for creating a short clip.
//
// Requirements:
// - Input must be derived/master.mp4 (not original)
// - Trim to exact start/end times
// - Center-crop to 9:16 aspect ratio
// - Scale to exactly 1080x1920
// - H.264 video codec + AAC audio codec
// - Faststart enabled
// - Uses medium preset with CRF 23
//
// Validation of clip duration (max 90s) is handled at the API/job level.
func ClipTranscodeArgs(inPath string, outPath string, probe ProbeResult, startMs int64, endMs int64) []string {
	startSec := float64(startMs) / 1000.0
	durationMs := endMs - startMs
	durationSec := float64(durationMs) / 1000.0

	args := []string{
		"-y",
		"-ss", fmt.Sprintf("%.3f", startSec),
		"-t", fmt.Sprintf("%.3f", durationSec),
		"-i", inPath,
		"-vf", fmt.Sprintf("crop=ih*(9/16):ih*(9/16):(iw-iw*(9/16))/2:0,scale=%d:%d", ClipWidth, ClipHeight),
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
		"-pix_fmt", "yuv420p",
		outPath,
	}

	return args
}

// HasFlag checks if the given arguments slice contains a specific flag.
// This is useful for testing and verification.
func HasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

// ContainsString checks if args contain a specific string value.
// Useful for testing that certain values are present.
func ContainsString(args []string, s string) bool {
	for _, arg := range args {
		if arg == s {
			return true
		}
	}
	return false
}

// ContainsSubstring checks if any argument contains the given substring.
func ContainsSubstring(args []string, substr string) bool {
	for _, arg := range args {
		if strings.Contains(arg, substr) {
			return true
		}
	}
	return false
}

// HLSVariant represents a single HLS variant stream.
type HLSVariant struct {
	Height    int
	Bandwidth int
}

// DefaultHLSVariants returns the default HLS variant ladder.
func DefaultHLSVariants() []HLSVariant {
	return []HLSVariant{
		{Height: 360, Bandwidth: 800000},
		{Height: 720, Bandwidth: 2500000},
		{Height: 1080, Bandwidth: 5000000},
	}
}

// HLSArgs builds ffmpeg arguments for generating HLS adaptive streaming.
//
// For simplicity, this creates a single variant based on source resolution.
// Multi-variant support can be added later with filter_complex.
func HLSArgs(inPath string, outDir string, variants []HLSVariant, sourceWidth, sourceHeight int) []string {
	if len(variants) == 0 {
		variants = DefaultHLSVariants()
	}

	filteredVariants := []HLSVariant{}
	for _, v := range variants {
		if v.Height <= sourceHeight {
			filteredVariants = append(filteredVariants, v)
		}
	}

	if len(filteredVariants) == 0 {
		filteredVariants = []HLSVariant{{Height: sourceHeight, Bandwidth: 2500000}}
	}

	numVariants := len(filteredVariants)

	if numVariants == 1 {
		v := filteredVariants[0]
		return []string{
			"-y",
			"-i", inPath,
			"-c:v", "libx264",
			"-preset", "medium",
			"-crf", "23",
			"-c:a", "aac",
			"-b:a", "128k",
			"-vf", fmt.Sprintf("scale=-2:%d", v.Height),
			"-f", "hls",
			"-hls_time", "6",
			"-hls_playlist_type", "vod",
			"-hls_segment_filename", fmt.Sprintf("%s/seg_%%03d.ts", outDir),
			"-hls_list_size", "0",
			"-master_pl_name", "index.m3u8",
			fmt.Sprintf("%s/playlist.m3u8", outDir),
		}
	}

	args := []string{"-y", "-i", inPath}

	scaleExprs := make([]string, numVariants)
	varStreamMap := make([]string, numVariants)
	segmentFiles := make([]string, numVariants)
	playlistFiles := make([]string, numVariants)

	for i, v := range filteredVariants {
		scaleExprs[i] = fmt.Sprintf("scale=-2:%d", v.Height)
		varStreamMap[i] = fmt.Sprintf("v:%d,a:%d", i, i)
		segmentFiles[i] = fmt.Sprintf("%s/v%d_%%03d.ts", outDir, i)
		playlistFiles[i] = fmt.Sprintf("%s/v%d.m3u8", outDir, i)
		args = append(args, "-filter_complex", fmt.Sprintf("[0:v]scale=-2:%d[v%d]", v.Height, i))
	}

	for i, v := range filteredVariants {
		args = append(args, "-map", fmt.Sprintf("[v%d]", i))
		args = append(args, "-map", "0:a")
		args = append(args, "-c:v", "libx264")
		args = append(args, "-preset", "medium")
		args = append(args, "-crf", "23")
		args = append(args, "-b:v", fmt.Sprintf("%dk", v.Bandwidth/1000))
		args = append(args, "-c:a", "aac")
		args = append(args, "-b:a", "128k")
		args = append(args, "-f", "hls")
		args = append(args, "-hls_time", "6")
		args = append(args, "-hls_playlist_type", "vod")
		args = append(args, "-hls_segment_filename", segmentFiles[i])
		args = append(args, "-hls_list_size", "0")
		args = append(args, "-var_stream_map", varStreamMap[i])
		if i == 0 {
			args = append(args, "-master_pl_name", "index.m3u8")
		}
		args = append(args, playlistFiles[i])
	}

	return args
}

// AudioTranscodeArgs builds ffmpeg arguments for transcoding audio to master M4A.
//
// Requirements:
// - AAC codec in M4A container
// - Faststart enabled (moov atom at beginning)
// - Normalize to stereo if needed
// - Use 44.1k or 48k sample rate
// - Use VBR or fixed bitrate (128k)
func AudioTranscodeArgs(inPath string, outPath string) []string {
	args := []string{
		"-y",
		"-i", inPath,
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
	}
	args = append(args, outPath)
	return args
}
