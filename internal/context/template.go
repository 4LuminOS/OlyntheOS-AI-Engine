package context

import "strings"

type Message struct {
	Role    string
	Content string
}

func Render(templateName, system string, messages []Message) string {
	var builder strings.Builder
	switch strings.ToLower(templateName) {
	case "qwen3", "qwen":
		builder.WriteString("<|im_start|>system\n")
		builder.WriteString(system)
		builder.WriteString("<|im_end|>\n")
	default:
		builder.WriteString("System: ")
		builder.WriteString(system)
		builder.WriteString("\n")
	}
	for _, message := range messages {
		role := message.Role
		if role == "" {
			role = "message"
		}
		builder.WriteString(strings.ToUpper(role[:1]))
		builder.WriteString(role[1:])
		builder.WriteString(": ")
		builder.WriteString(message.Content)
		builder.WriteString("\n")
	}
	return builder.String()
}
