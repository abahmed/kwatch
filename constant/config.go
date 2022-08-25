package constant

type AlertProvider struct {
}
type Config struct {
	MaxRecentLogLines            int                          `mapstructure:"maxRecentLogLines"`
	IgnoreFailedGracefulShutdown bool                         `mapstructure:"ignoreFailedGracefulShutdown"`
	DisableUpdateCheck           bool                         `mapstructure:"DisableUpdateCheck"`
	Namespaces                   []string                     `mapstructure:"namespaces"`
	Reasons                      []string                     `mapstructure:"reasons"`
	IgnoreContainerNames         []string                     `mapstructure:"ignoreContainerNames"`
	Alert                        map[string]map[string]string `mapstructure:"alert"`
}
