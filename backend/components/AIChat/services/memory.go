package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

type MemoryUpdate struct {
	Summary        string   `json:"summary"`
	Courses        []string `json:"courses"`
	Goals          []string `json:"goals"`
	Strengths      []string `json:"strengths"`
	Misconceptions []string `json:"misconceptions"`
	Preferences    []string `json:"preferences"`
}

// EstimateTokens is deliberately conservative and dependency-free. Exact usage
// is returned by the provider, but this estimate is sufficient for enforcing a
// stable pre-request context budget.
func EstimateTokens(text string) int {
	runes := utf8.RuneCountInString(text)
	if runes == 0 {
		return 0
	}
	return (runes + 2) / 3
}

func GenerateMemory(existingSummary string, profile StudentProfile, messages []ChatMessage) (MemoryUpdate, error) {
	transcript := make([]string, 0, len(messages))
	for _, message := range messages {
		if content := strings.TrimSpace(message.Content); content != "" {
			transcript = append(transcript, message.Role+": "+content)
		}
	}
	profileJSON, _ := json.Marshal(profile)
	prompt := fmt.Sprintf(`Update an academic chatbot's durable memory from the new transcript.
Return JSON only with exactly these fields: summary, courses, goals, strengths, misconceptions, preferences.
The arrays must contain short strings. Preserve still-relevant existing facts, remove duplicates, and do not invent facts.
The summary must capture topics covered, conclusions, unresolved questions, and the next useful learning step.

Existing summary:
%s

Existing student profile:
%s

New transcript:
%s`, existingSummary, profileJSON, strings.Join(transcript, "\n"))

	raw, err := GetChatGPTResponse([]Message{
		{Role: "system", Content: "You maintain concise, factual memory for an educational assistant."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return MemoryUpdate{}, err
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	var update MemoryUpdate
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &update); err != nil {
		return MemoryUpdate{}, fmt.Errorf("invalid memory response: %w", err)
	}
	if strings.TrimSpace(update.Summary) == "" {
		return MemoryUpdate{}, fmt.Errorf("memory response contained an empty summary")
	}
	return update, nil
}
