package inference

type GenerationOptions struct {
	MaxTokens   int
	Temperature float64
	TopP        float64
}

func DefaultGenerationOptions() GenerationOptions {
	return GenerationOptions{MaxTokens: 128, Temperature: 0.7, TopP: 0.9}
}
