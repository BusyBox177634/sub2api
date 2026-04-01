package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

func parseUsageLogRequestMessages(log *UsageLog, raw string) []UsageLogMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" || !gjson.Valid(raw) {
		return []UsageLogMessage{}
	}

	result := gjson.Parse(raw)
	messages := make([]UsageLogMessage, 0, 8)

	appendMessagesFromSystem(&messages, result.Get("system"), "request")
	appendMessagesFromSystem(&messages, result.Get("systemInstruction.parts"), "request")

	appendMessagesFromArray(&messages, result.Get("messages"), "request", false)
	appendMessagesFromArray(&messages, result.Get("contents"), "request", true)

	input := result.Get("input")
	if input.Exists() {
		switch {
		case input.IsArray():
			appendMessagesFromArray(&messages, input, "request", false)
		case input.IsObject():
			appendSingleMessage(&messages, input, "request", "user")
		case input.Type == gjson.String:
			appendMessageText(&messages, "user", "request", input.String())
		}
	}

	return compactUsageLogMessages(messages)
}

func parseUsageLogResponseMessages(log *UsageLog, raw string) []UsageLogMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" || !gjson.Valid(raw) {
		return []UsageLogMessage{}
	}

	result := gjson.Parse(raw)
	messages := make([]UsageLogMessage, 0, 8)

	appendMessagesFromArray(&messages, result.Get("messages"), "response", false)
	appendMessagesFromArray(&messages, result.Get("output"), "response", false)
	appendMessagesFromArray(&messages, result.Get("response.output"), "response", false)

	content := result.Get("content")
	if content.Exists() {
		role := strings.TrimSpace(result.Get("role").String())
		if role == "" {
			role = "assistant"
		}
		appendMessageText(&messages, role, "response", extractContentText(content))
	}

	choices := result.Get("choices")
	if choices.Exists() && choices.IsArray() {
		choices.ForEach(func(_, choice gjson.Result) bool {
			message := choice.Get("message")
			if message.Exists() {
				appendSingleMessage(&messages, message, "response", "assistant")
			}
			return true
		})
	}

	if text := extractGeminiCandidateText(result); text != "" {
		appendMessageText(&messages, "assistant", "response", text)
	}

	if len(messages) == 0 {
		if text := strings.TrimSpace(result.Get("text").String()); text != "" {
			appendMessageText(&messages, "assistant", "response", text)
		}
	}

	return compactUsageLogMessages(messages)
}

func appendMessagesFromSystem(messages *[]UsageLogMessage, value gjson.Result, source string) {
	if !value.Exists() {
		return
	}
	appendMessageText(messages, "system", source, extractContentText(value))
}

func appendMessagesFromArray(messages *[]UsageLogMessage, value gjson.Result, source string, geminiRole bool) {
	if !value.Exists() || !value.IsArray() {
		return
	}
	value.ForEach(func(_, item gjson.Result) bool {
		role := strings.TrimSpace(item.Get("role").String())
		if geminiRole {
			switch role {
			case "model":
				role = "assistant"
			case "", "user":
				role = "user"
			}
		}
		if role == "" {
			switch strings.TrimSpace(item.Get("type").String()) {
			case "message":
				role = strings.TrimSpace(item.Get("role").String())
			case "function_call", "tool_call":
				role = "tool"
			default:
				if source == "response" {
					role = "assistant"
				} else {
					role = "user"
				}
			}
		}
		if source == "request" && shouldSkipUsageLogRequestItem(item, role) {
			return true
		}
		appendSingleMessage(messages, item, source, role)
		return true
	})
}

func appendSingleMessage(messages *[]UsageLogMessage, item gjson.Result, source string, fallbackRole string) {
	if !item.Exists() {
		return
	}
	role := strings.TrimSpace(item.Get("role").String())
	if role == "" {
		role = fallbackRole
	}
	if role == "" {
		role = "unknown"
	}

	text := extractContentText(item.Get("content"))
	if text == "" {
		text = extractContentText(item.Get("parts"))
	}
	if text == "" {
		text = strings.TrimSpace(item.Get("text").String())
	}
	if text == "" {
		switch strings.TrimSpace(item.Get("type").String()) {
		case "input_text", "output_text", "text":
			text = strings.TrimSpace(item.Get("text").String())
		case "message":
			text = extractContentText(item.Get("content"))
		case "input_image", "image":
			text = "[image]"
		case "tool", "function_call", "tool_call":
			text = strings.TrimSpace(item.Get("name").String())
			if text == "" {
				text = "[tool call]"
			}
			role = "tool"
		}
	}

	appendMessageText(messages, role, source, text)
}

func appendMessageText(messages *[]UsageLogMessage, role string, source string, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	role = strings.TrimSpace(role)
	if role == "" {
		role = "unknown"
	}
	*messages = append(*messages, UsageLogMessage{
		Role:   role,
		Source: source,
		Text:   text,
	})
}

func extractContentText(value gjson.Result) string {
	if !value.Exists() {
		return ""
	}
	switch {
	case value.Type == gjson.String:
		return strings.TrimSpace(value.String())
	case value.IsArray():
		parts := make([]string, 0, len(value.Array()))
		value.ForEach(func(_, item gjson.Result) bool {
			if text := extractContentText(item); text != "" {
				parts = append(parts, text)
			}
			return true
		})
		return strings.TrimSpace(strings.Join(parts, "\n\n"))
	case value.IsObject():
		if text := strings.TrimSpace(value.Get("text").String()); text != "" {
			return text
		}
		if text := strings.TrimSpace(value.Get("delta").String()); text != "" {
			return text
		}
		if text := strings.TrimSpace(value.Get("partial_json").String()); text != "" {
			return text
		}
		if text := strings.TrimSpace(value.Get("arguments").Raw); text != "" && text != "null" {
			return text
		}
		if text := extractContentText(value.Get("content")); text != "" {
			return text
		}
		if text := extractContentText(value.Get("parts")); text != "" {
			return text
		}
		if inlineData := value.Get("inlineData"); inlineData.Exists() {
			if mimeType := strings.TrimSpace(inlineData.Get("mimeType").String()); mimeType != "" {
				return "[" + mimeType + "]"
			}
			return "[inline data]"
		}
		if imageURL := strings.TrimSpace(value.Get("image_url.url").String()); imageURL != "" {
			return "[image]"
		}
		if itemType := strings.TrimSpace(value.Get("type").String()); itemType == "input_image" || itemType == "image" {
			return "[image]"
		}
	}
	return ""
}

func extractGJSONText(value gjson.Result) string {
	return extractContentText(value)
}

func compactUsageLogMessages(messages []UsageLogMessage) []UsageLogMessage {
	if len(messages) == 0 {
		return []UsageLogMessage{}
	}
	compacted := make([]UsageLogMessage, 0, len(messages))
	for _, message := range messages {
		message.Role = strings.TrimSpace(message.Role)
		message.Source = strings.TrimSpace(message.Source)
		message.Text = strings.TrimSpace(message.Text)
		if message.Role == "" {
			message.Role = "unknown"
		}
		if message.Source == "" {
			message.Source = "unknown"
		}
		if message.Text == "" {
			continue
		}
		if len(compacted) > 0 {
			last := &compacted[len(compacted)-1]
			if last.Role == message.Role && last.Source == message.Source {
				last.Text += "\n\n" + message.Text
				continue
			}
		}
		compacted = append(compacted, message)
	}
	return compacted
}

func shouldSkipUsageLogRequestItem(item gjson.Result, role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	itemType := strings.ToLower(strings.TrimSpace(item.Get("type").String()))

	switch role {
	case "assistant":
		return true
	}

	switch itemType {
	case "reasoning", "item_reference":
		return true
	case "output_text":
		return true
	}

	if itemType == "message" && role == "assistant" {
		return true
	}
	return false
}
