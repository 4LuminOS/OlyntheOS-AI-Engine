package hardware

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
)

type ProbeResult struct {
	TotalMemoryMB    int    `json:"total_memory_mb"`
	AvailableMemoryMB int    `json:"available_memory_mb"`
	HasGPU           bool   `json:"has_gpu"`
	GPU              string `json:"gpu"`
}

func ProbeSystem() ProbeResult {
	result := ProbeResult{TotalMemoryMB: runtime.GOMAXPROCS(0) * 1024}
	file, err := os.Open("/proc/meminfo")
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 2 {
				continue
			}
			value, err := strconv.Atoi(fields[1])
			if err != nil {
				continue
			}
			switch strings.TrimSuffix(fields[0], ":") {
			case "MemTotal":
				result.TotalMemoryMB = value / 1024
			case "MemAvailable":
				result.AvailableMemoryMB = value / 1024
			}
		}
	}
	if result.AvailableMemoryMB == 0 {
		result.AvailableMemoryMB = result.TotalMemoryMB / 2
	}
	return result
}
