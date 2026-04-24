package service

import "context"

// IsUsageMessageRetentionEnabled reports whether usage message detail should be exposed.
// The setting is not user-configurable yet, so we keep it enabled by default.
func (s *SettingService) IsUsageMessageRetentionEnabled(_ context.Context) bool {
	return true
}
