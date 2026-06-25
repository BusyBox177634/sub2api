package service

import (
	"context"
	"regexp"
	"strings"
)

var quickOpsMonitorSuffixPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{4,64}$`)

var quickOpsMonitorReservedSuffixes = map[string]struct{}{
	"admin":              {},
	"affiliate":          {},
	"api":                {},
	"auth":               {},
	"available-channels": {},
	"chat":               {},
	"custom":             {},
	"dashboard":          {},
	"email-verify":       {},
	"forgot-password":    {},
	"groups":             {},
	"health":             {},
	"home":               {},
	"key-usage":          {},
	"keys":               {},
	"legal":              {},
	"login":              {},
	"monitor":            {},
	"orders":             {},
	"payment":            {},
	"profile":            {},
	"purchase":           {},
	"redeem":             {},
	"register":           {},
	"reset-password":     {},
	"setup":              {},
	"subscriptions":      {},
	"usage":              {},
	"usage-brief":        {},
	"wechat":             {},
	"ws":                 {},
}

type QuickOpsMonitorRuntime struct {
	Enabled bool   `json:"enabled"`
	Suffix  string `json:"suffix,omitempty"`
}

func NormalizeQuickOpsMonitorSuffix(raw string) string {
	return strings.Trim(strings.TrimSpace(raw), "/")
}

func ValidateQuickOpsMonitorSuffix(raw string) (string, bool) {
	suffix := NormalizeQuickOpsMonitorSuffix(raw)
	if suffix == "" {
		return "", true
	}
	if !quickOpsMonitorSuffixPattern.MatchString(suffix) {
		return suffix, false
	}
	if _, reserved := quickOpsMonitorReservedSuffixes[strings.ToLower(suffix)]; reserved {
		return suffix, false
	}
	return suffix, true
}

func (s *SettingService) GetQuickOpsMonitorRuntime(ctx context.Context) QuickOpsMonitorRuntime {
	vals, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyQuickOpsMonitorEnabled,
		SettingKeyQuickOpsMonitorSuffix,
	})
	if err != nil {
		return QuickOpsMonitorRuntime{}
	}
	suffix, ok := ValidateQuickOpsMonitorSuffix(vals[SettingKeyQuickOpsMonitorSuffix])
	return QuickOpsMonitorRuntime{
		Enabled: vals[SettingKeyQuickOpsMonitorEnabled] == "true" && ok && suffix != "",
		Suffix:  suffix,
	}
}

func (s *SettingService) IsQuickOpsMonitorSuffixAllowed(ctx context.Context, suffix string) bool {
	runtime := s.GetQuickOpsMonitorRuntime(ctx)
	return runtime.Enabled && runtime.Suffix == NormalizeQuickOpsMonitorSuffix(suffix)
}
