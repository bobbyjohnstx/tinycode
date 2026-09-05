package provider

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// DetectGPUMemory returns available GPU memory in bytes.
// On macOS with Apple Silicon, returns 75% of unified memory.
// On Linux with NVIDIA GPUs, parses nvidia-smi output.
// Returns 0 with an error if detection fails.
func DetectGPUMemory() (int64, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0, fmt.Errorf("sysctl hw.memsize: %w", err)
		}
		return parseSysctlMemsize(string(out))
	case "linux":
		out, err := exec.Command("nvidia-smi", "--query-gpu=memory.total", "--format=csv,noheader,nounits").Output()
		if err != nil {
			return 0, fmt.Errorf("nvidia-smi: %w", err)
		}
		return parseNvidiaSMIOutput(string(out))
	default:
		return 0, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

var numericRe = regexp.MustCompile(`\d+`)

// parseSysctlMemsize parses the output of `sysctl -n hw.memsize` and returns
// 75% of total memory as the GPU budget for Apple Silicon unified memory.
func parseSysctlMemsize(output string) (int64, error) {
	s := strings.TrimSpace(output)
	if s == "" {
		return 0, fmt.Errorf("empty sysctl output")
	}
	totalBytes, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing sysctl memsize %q: %w", s, err)
	}
	if totalBytes <= 0 {
		return 0, fmt.Errorf("invalid memory size: %d", totalBytes)
	}
	return totalBytes * 3 / 4, nil
}

// parseNvidiaSMIOutput parses nvidia-smi memory output (MiB per GPU, one per line)
// and returns the total memory in bytes across all GPUs.
func parseNvidiaSMIOutput(output string) (int64, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var totalMiB int64
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		match := numericRe.FindString(line)
		if match == "" {
			continue
		}
		mib, err := strconv.ParseInt(match, 10, 64)
		if err != nil {
			continue
		}
		totalMiB += mib
	}
	if totalMiB == 0 {
		return 0, fmt.Errorf("no GPU memory found in nvidia-smi output")
	}
	return totalMiB * 1024 * 1024, nil
}
