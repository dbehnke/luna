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
