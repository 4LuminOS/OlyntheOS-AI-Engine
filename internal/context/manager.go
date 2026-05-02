package context

import "strings"

type Manager struct {
	MaxTokens int
	Messages  []string
}

func NewManager(maxTokens int) *Manager {
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	return &Manager{MaxTokens: maxTokens}
}

func (m *Manager) Add(text string) {
	m.Messages = append(m.Messages, text)
	m.Trim()
}

func (m *Manager) Count() int {
	count := 0
	for _, message := range m.Messages {
		count += len(strings.Fields(message))
	}
	return count
}

func (m *Manager) Trim() {
	for m.Count() > m.MaxTokens && len(m.Messages) > 0 {
		m.Messages = m.Messages[1:]
	}
}
