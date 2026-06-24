package service

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

type UsageLogPayloadKind string

const (
	UsageLogPayloadKindRequest  UsageLogPayloadKind = "request"
	UsageLogPayloadKindResponse UsageLogPayloadKind = "response"
)

type compressedUsageLogPayload struct {
	Messages []compressedUsageLogMessage `json:"messages"`
}

type compressedUsageLogMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

var usageLogPseudoTextPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?is)^<system-reminder\b`),
	regexp.MustCompile(`(?is)^<local-command-`),
	regexp.MustCompile(`(?is)^<command-name>`),
	regexp.MustCompile(`(?is)^<command-message>`),
	regexp.MustCompile(`(?is)^<ide_opened_file>`),
	regexp.MustCompile(`(?is)^<ide_selection>`),
	regexp.MustCompile(`(?is)^<ide_diagnostics>`),
	regexp.MustCompile(`(?is)^<environment_context>`),
	regexp.MustCompile(`(?is)^<permissions instructions>`),
	regexp.MustCompile(`(?is)^<skills_instructions>`),
	regexp.MustCompile(`(?is)^<plugins_instructions>`),
	regexp.MustCompile(`(?is)^<collaboration_mode>`),
	regexp.MustCompile(`(?is)^# AGENTS\.md instructions\b`),
	regexp.MustCompile(`(?is)^# Overview\s+Generate 0 to 3 hyperpersonalized suggestions\b`),
	regexp.MustCompile(`(?is)^This session is being continued from a previous conversation\b`),
	regexp.MustCompile(`(?is)^You have oh-my-codex installed\b`),
	regexp.MustCompile(`(?is)^Called the .* tool with the following input:`),
	regexp.MustCompile(`(?is)^Result of calling the .* tool:`),
	regexp.MustCompile(`(?is)^\(Bash completed with no output\)$`),
}

var (
	usageLogImageBlockPattern      = regexp.MustCompile(`(?is)<image\b.*?</image>`)
	usageLogImageOpenTagPattern    = regexp.MustCompile(`(?is)<image\b[^>]*>`)
	usageLogImageCloseTagPattern   = regexp.MustCompile(`(?is)</image>`)
	usageLogRepeatedNewlinePattern = regexp.MustCompile(`\n{4,}`)
)

func CompressUsageLogPayloadJSON(payloadJSON *string, kind UsageLogPayloadKind) *string {
	if payloadJSON == nil {
		return nil
	}
	payload := strings.TrimSpace(*payloadJSON)
	if payload == "" || !gjson.Valid(payload) {
		return nil
	}
	messages := compressUsageLogPayloadMessages(gjson.Parse(payload), kind)
	out := compressedUsageLogPayload{Messages: messages}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	result := string(encoded)
	return &result
}

func compressUsageLogPayloadMessages(result gjson.Result, kind UsageLogPayloadKind) []compressedUsageLogMessage {
	sourceMessages := usageLogProtocolMessageResults(result, kind)
	messages := make([]compressedUsageLogMessage, 0, len(sourceMessages))
	for _, message := range sourceMessages {
		role := strings.ToLower(strings.TrimSpace(message.Get("role").String()))
		if role != "user" && role != "assistant" {
			continue
		}
		texts := extractUsageLogCompressedTexts(message)
		cleaned := make([]string, 0, len(texts))
		for _, text := range texts {
			text = cleanUsageLogCompressedText(text)
			if text == "" || isUsageLogPseudoText(text) {
				continue
			}
			cleaned = append(cleaned, text)
		}
		content := strings.TrimSpace(strings.Join(cleaned, "\n\n"))
		if content == "" {
			continue
		}
		messages = append(messages, compressedUsageLogMessage{Role: role, Content: content})
	}
	return messages
}

func usageLogProtocolMessageResults(result gjson.Result, kind UsageLogPayloadKind) []gjson.Result {
	switch {
	case result.Get("messages").IsArray():
		return filterUsageLogRoleMessages(result.Get("messages").Array())
	case kind == UsageLogPayloadKindRequest && result.Get("input").IsArray():
		return filterUsageLogTypedMessages(result.Get("input").Array())
	case kind == UsageLogPayloadKindRequest && result.Get("input").IsObject():
		return []gjson.Result{result.Get("input")}
	case kind == UsageLogPayloadKindResponse && result.Get("output").IsArray():
		return filterUsageLogTypedMessages(result.Get("output").Array())
	case kind == UsageLogPayloadKindResponse && result.Get("response.output").IsArray():
		return filterUsageLogTypedMessages(result.Get("response.output").Array())
	case kind == UsageLogPayloadKindResponse && result.Get("choices").IsArray():
		return usageLogChoiceMessageResults(result.Get("choices").Array())
	case kind == UsageLogPayloadKindResponse && result.Get("content").Exists():
		return []gjson.Result{result}
	default:
		return nil
	}
}

func filterUsageLogRoleMessages(items []gjson.Result) []gjson.Result {
	out := make([]gjson.Result, 0, len(items))
	for _, item := range items {
		role := strings.ToLower(strings.TrimSpace(item.Get("role").String()))
		if role == "user" || role == "assistant" {
			out = append(out, item)
		}
	}
	return out
}

func filterUsageLogTypedMessages(items []gjson.Result) []gjson.Result {
	out := make([]gjson.Result, 0, len(items))
	for _, item := range items {
		if shouldSkipUsageLogCompressedItem(item) {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(item.Get("role").String()))
		if role == "user" || role == "assistant" {
			out = append(out, item)
		}
	}
	return out
}

func shouldSkipUsageLogCompressedItem(item gjson.Result) bool {
	itemType := strings.ToLower(strings.TrimSpace(item.Get("type").String()))
	switch itemType {
	case "", "message":
		return false
	case "function_call", "function_call_output", "tool_call", "tool_result", "reasoning", "item_reference", "input_image", "image":
		return true
	default:
		return false
	}
}

func usageLogChoiceMessageResults(items []gjson.Result) []gjson.Result {
	out := make([]gjson.Result, 0, len(items))
	for _, item := range items {
		message := item.Get("message")
		if !message.Exists() {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(message.Get("role").String()))
		if role == "user" || role == "assistant" {
			out = append(out, message)
		}
	}
	return out
}

func extractUsageLogCompressedTexts(message gjson.Result) []string {
	if text := strings.TrimSpace(message.Get("text").String()); text != "" {
		return []string{text}
	}
	content := message.Get("content")
	if !content.Exists() {
		return nil
	}
	if content.Type == gjson.String {
		return []string{content.String()}
	}
	if !content.IsArray() {
		return nil
	}
	texts := make([]string, 0, len(content.Array()))
	content.ForEach(func(_, part gjson.Result) bool {
		partType := strings.TrimSpace(part.Get("type").String())
		switch partType {
		case "text", "input_text", "output_text":
			if text := strings.TrimSpace(part.Get("text").String()); text != "" {
				texts = append(texts, text)
			} else if text := strings.TrimSpace(part.Get("content").String()); text != "" {
				texts = append(texts, text)
			}
		}
		return true
	})
	return texts
}

func cleanUsageLogCompressedText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = usageLogImageBlockPattern.ReplaceAllString(value, "")
	value = usageLogImageOpenTagPattern.ReplaceAllString(value, "")
	value = usageLogImageCloseTagPattern.ReplaceAllString(value, "")
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	value = strings.TrimSpace(strings.Join(lines, "\n"))
	value = usageLogRepeatedNewlinePattern.ReplaceAllString(value, "\n\n\n")
	return strings.TrimSpace(value)
}

func isUsageLogPseudoText(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return true
	}
	for _, pattern := range usageLogPseudoTextPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}
