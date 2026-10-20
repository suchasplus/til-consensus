package model

// CLISettings 只保存应用级公开选择；认证及全局 CLI 配置始终由 CLI 管理。
type CLISettings struct {
	Version   int                  `json:"version"`
	Providers []CLIProviderSetting `json:"providers"`
}
type CLIProviderSetting struct {
	Enabled   *bool  `json:"enabled,omitempty"`
	ID        string `json:"id"`
	Model     string `json:"model"`
	Reasoning string `json:"reasoning"`
}
