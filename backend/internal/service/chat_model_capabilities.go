package service

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

const chatModelCapabilitiesFile = "model_prices_and_context_window.json"

var (
	chatWebSearchSupportOnce sync.Once
	chatWebSearchSupportSet  map[string]struct{}
)

type chatModelCapabilityEntry struct {
	SupportsWebSearch bool `json:"supports_web_search"`
}

func loadChatWebSearchSupportSet() map[string]struct{} {
	chatWebSearchSupportOnce.Do(func() {
		chatWebSearchSupportSet = make(map[string]struct{})

		candidates := []string{
			"resources/model-pricing/" + chatModelCapabilitiesFile,
			"backend/resources/model-pricing/" + chatModelCapabilitiesFile,
			"/app/resources/model-pricing/" + chatModelCapabilitiesFile,
		}

		for _, candidate := range candidates {
			payload, err := os.ReadFile(candidate)
			if err != nil {
				continue
			}

			var entries map[string]chatModelCapabilityEntry
			if err := json.Unmarshal(payload, &entries); err != nil {
				continue
			}

			for modelID, entry := range entries {
				if !entry.SupportsWebSearch {
					continue
				}
				normalized := strings.ToLower(strings.TrimSpace(modelID))
				if normalized == "" {
					continue
				}
				chatWebSearchSupportSet[normalized] = struct{}{}
			}
			return
		}
	})

	return chatWebSearchSupportSet
}

