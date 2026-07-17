package service

// UsageLogDetailWiring is a Wire marker that guarantees usage detail dependencies
// are attached before the HTTP handler graph is exposed.
type UsageLogDetailWiring struct{}

// ProvideUsageLogDetailWiring attaches the shared detail repository to the
// gateway writers and the detail reader. Usage detail exposure remains enabled
// through SettingService.IsUsageMessageRetentionEnabled.
func ProvideUsageLogDetailWiring(
	gatewayService *GatewayService,
	openAIGatewayService *OpenAIGatewayService,
	usageService *UsageService,
	repo UsageLogDetailRepository,
	settingService *SettingService,
) *UsageLogDetailWiring {
	gatewayService.SetUsageLogDetailRepo(repo)
	openAIGatewayService.SetUsageLogDetailRepo(repo)
	usageService.SetUsageLogDetailRepo(repo)
	usageService.SetSettingService(settingService)
	return &UsageLogDetailWiring{}
}
