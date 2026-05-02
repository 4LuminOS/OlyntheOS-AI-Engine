package inference

// EncodePrompt converts input text into model tokens using the loaded model's vocabulary.
func EncodePrompt(model *Model, text string, addSpecial bool) ([]int32, error) {
	return model.Tokenize(text, addSpecial)
}

// DecodeTokens converts model tokens back into a string using the loaded model's vocabulary.
func DecodeTokens(model *Model, tokens []int32) (string, error) {
	return model.Detokenize(tokens)
}
