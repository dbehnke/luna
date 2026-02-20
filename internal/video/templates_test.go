package video

import (
	"strings"
	"testing"
)

func TestMasterTranscodeArgs_HasFaststart(t *testing.T) {
	args := MasterTranscodeArgs("input.mp4", "output.mp4", ProbeResult{})

	found := false
	for _, arg := range args {
		if arg == "-movflags" {
			// Check next arg contains faststart
			continue
		}
		if strings.Contains(arg, "faststart") {
			found = true
			break
		}
	}

	if !found {
		t.Error("MasterTranscodeArgs should include faststart flag")
	}
}

func TestMasterTranscodeArgs_HasScaleTo1080p(t *testing.T) {
	// Test with wide resolution (should scale height to 1080)
	probe := ProbeResult{Width: 1920, Height: 1440}
	args := MasterTranscodeArgs("input.mp4", "output.mp4", probe)

	found := false
	for i, arg := range args {
		if arg == "-vf" {
			vf := args[i+1]
			if strings.Contains(vf, "scale=-2:1080") || strings.Contains(vf, "scale=1080") {
				found = true
			}
		}
	}

	if !found {
		t.Error("MasterTranscodeArgs should scale to 1080p for wide video")
	}
}

func TestMasterTranscodeArgs_NoScaleForSmall(t *testing.T) {
	// Test with small resolution (should not add scale)
	probe := ProbeResult{Width: 640, Height: 480}
	args := MasterTranscodeArgs("input.mp4", "output.mp4", probe)

	hasScale := false
	for _, arg := range args {
		if arg == "-vf" {
			hasScale = true
		}
	}

	if hasScale {
		t.Error("MasterTranscodeArgs should not add scale filter for small video")
	}
}

func TestMasterTranscodeArgs_HasYUV420p(t *testing.T) {
	args := MasterTranscodeArgs("input.mp4", "output.mp4", ProbeResult{})

	found := false
	for i, arg := range args {
		if arg == "-pix_fmt" && args[i+1] == "yuv420p" {
			found = true
			break
		}
	}

	if !found {
		t.Error("MasterTranscodeArgs should include yuv420p pixel format")
	}
}

func TestMasterTranscodeArgs_HasH264(t *testing.T) {
	args := MasterTranscodeArgs("input.mp4", "output.mp4", ProbeResult{})

	found := false
	for i, arg := range args {
		if arg == "-c:v" && args[i+1] == "libx264" {
			found = true
			break
		}
	}

	if !found {
		t.Error("MasterTranscodeArgs should use libx264")
	}
}

func TestMasterTranscodeArgs_HasAAC(t *testing.T) {
	args := MasterTranscodeArgs("input.mp4", "output.mp4", ProbeResult{})

	found := false
	for i, arg := range args {
		if arg == "-c:a" && args[i+1] == "aac" {
			found = true
			break
		}
	}

	if !found {
		t.Error("MasterTranscodeArgs should use aac audio")
	}
}

func TestClipTranscodeArgs_HasFaststart(t *testing.T) {
	args := ClipTranscodeArgs("input.mp4", "output.mp4", ProbeResult{}, 0, 10000)

	found := false
	for _, arg := range args {
		if strings.Contains(arg, "faststart") {
			found = true
			break
		}
	}

	if !found {
		t.Error("ClipTranscodeArgs should include faststart flag")
	}
}

func TestClipTranscodeArgs_HasScaleTo1080x1920(t *testing.T) {
	args := ClipTranscodeArgs("input.mp4", "output.mp4", ProbeResult{}, 0, 10000)

	found := false
	for i, arg := range args {
		if arg == "-vf" {
			vf := args[i+1]
			if strings.Contains(vf, "scale=1080:1920") {
				found = true
			}
		}
	}

	if !found {
		t.Error("ClipTranscodeArgs should scale to 1080x1920")
	}
}

func TestClipTranscodeArgs_HasCropTo9_16(t *testing.T) {
	args := ClipTranscodeArgs("input.mp4", "output.mp4", ProbeResult{}, 0, 10000)

	found := false
	for i, arg := range args {
		if arg == "-vf" {
			vf := args[i+1]
			if strings.Contains(vf, "crop=ih*(9/16)") {
				found = true
			}
		}
	}

	if !found {
		t.Error("ClipTranscodeArgs should include 9:16 center crop")
	}
}

func TestClipTranscodeArgs_HasH264(t *testing.T) {
	args := ClipTranscodeArgs("input.mp4", "output.mp4", ProbeResult{}, 0, 10000)

	found := false
	for i, arg := range args {
		if arg == "-c:v" && args[i+1] == "libx264" {
			found = true
			break
		}
	}

	if !found {
		t.Error("ClipTranscodeArgs should use libx264")
	}
}

func TestClipTranscodeArgs_HasYUV420p(t *testing.T) {
	args := ClipTranscodeArgs("input.mp4", "output.mp4", ProbeResult{}, 0, 10000)

	found := false
	for i, arg := range args {
		if arg == "-pix_fmt" && args[i+1] == "yuv420p" {
			found = true
			break
		}
	}

	if !found {
		t.Error("ClipTranscodeArgs should include yuv420p pixel format")
	}
}

func TestClipTranscodeArgs_HasTrim(t *testing.T) {
	args := ClipTranscodeArgs("input.mp4", "output.mp4", ProbeResult{}, 5000, 15000)

	hasSS := false
	hasT := false

	for i, arg := range args {
		if arg == "-ss" {
			hasSS = true
			// Verify it starts at 5 seconds
			if args[i+1] != "5.000" {
				t.Errorf("Expected -ss 5.000, got %s", args[i+1])
			}
		}
		if arg == "-t" {
			hasT = true
			// Verify duration is 10 seconds (15-5)
			if args[i+1] != "10.000" {
				t.Errorf("Expected -t 10.000, got %s", args[i+1])
			}
		}
	}

	if !hasSS {
		t.Error("ClipTranscodeArgs should include -ss flag for start time")
	}
	if !hasT {
		t.Error("ClipTranscodeArgs should include -t flag for duration")
	}
}

func TestClipTranscodeArgs_UsesInputAsMaster(t *testing.T) {
	args := ClipTranscodeArgs("/path/to/master.mp4", "output.mp4", ProbeResult{}, 0, 10000)

	inputIndex := -1
	for i, arg := range args {
		if arg == "-i" {
			inputIndex = i
			break
		}
	}

	if inputIndex == -1 {
		t.Fatal("ClipTranscodeArgs should include -i flag")
	}

	// The input path should be the first argument after -i
	if args[inputIndex+1] != "/path/to/master.mp4" {
		t.Error("ClipTranscodeArgs should use the provided input path (expected to be master.mp4)")
	}
}

func TestHasFlag(t *testing.T) {
	args := []string{"-y", "-i", "input.mp4", "-vf", "scale=100:100"}

	if !HasFlag(args, "-y") {
		t.Error("HasFlag should find -y")
	}
	if !HasFlag(args, "-i") {
		t.Error("HasFlag should find -i")
	}
	if HasFlag(args, "-c:v") {
		t.Error("HasFlag should not find -c:v")
	}
}

func TestContainsSubstring(t *testing.T) {
	args := []string{"-vf", "scale=1080:1920,crop=ih*(9/16)"}

	if !ContainsSubstring(args, "1080:1920") {
		t.Error("ContainsSubstring should find scale dimensions")
	}
	if !ContainsSubstring(args, "crop") {
		t.Error("ContainsSubstring should find crop")
	}
	if ContainsSubstring(args, "faststart") {
		t.Error("ContainsSubstring should not find faststart")
	}
}
