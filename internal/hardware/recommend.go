package hardware

func RecommendModel(probe ProbeResult) string {
	switch {
	case probe.TotalMemoryMB < 8000:
		return "qwen3-1.5b"
	case probe.TotalMemoryMB < 16000:
		return "gemma-2b"
	default:
		return "llama-8b"
	}
}
