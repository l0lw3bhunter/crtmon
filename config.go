package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Webhook          string        `yaml:"webhook"`
	TelegramBotToken string        `yaml:"telegram_bot_token"`
	TelegramChatID   string        `yaml:"telegram_chat_id"`
	GitHubToken      string        `yaml:"github_token"`
	GitLabToken      string        `yaml:"gitlab_token"`
	Targets          []string      `yaml:"targets"`
	Enumeration      EnumConfig    `yaml:"enumeration"`
	Webhooks         WebhookConfig `yaml:"webhooks"`
	AdminPanel       AdminConfig   `yaml:"admin_panel"`
}

var customConfigPath string

func setConfigPath(path string) {
	customConfigPath = path
}

func getConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "crtmon"), nil
}

func getConfigPath() (string, error) {
	if customConfigPath != "" {
		return customConfigPath, nil
	}
	dir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "provider.yaml"), nil
}

func createConfigTemplate() error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	template := `# crtmon configuration
# monitor your targets real time via certificate transparency logs

# discord webhook url for notifications
webhook: ""

# telegram bot credentials for notifications (optional)
telegram_bot_token: ""
telegram_chat_id: ""

# target wildcard to monitor
targets:

# Multiple webhook URLs for different message types (optional)
webhooks:
  new_domains_webhook: ""        # New domain discoveries
  subdomain_scans_webhook: ""    # Subdomain enumeration results
  directory_scans_webhook: ""    # Directory enumeration results
  daily_summary_webhook: ""      # Daily summary

# admin panel configuration (optional)
admin_panel:
  enabled: true
  port: 8080

# enumeration settings (optional)
enumeration:
  enable_enum: false
  feroxbuster_path: "/usr/bin/feroxbuster"
  puredns_path: "/usr/bin/puredns"
  dir_wordlist: "/usr/share/wordlists/SecLists-master/Discovery/Web-Content/DirBuster-2007_directory-list-lowercase-2.3-small.txt"
  dns_wordlist: "/usr/share/wordlists/SecLists-master/Discovery/DNS/dns-Jhaddix.txt"
  resolvers_file: "/usr/share/wordlists/resolvers.txt"
  rate_limit: 15
  rate_limit_trusted: 300
  scan_timeout: 3600
  notify_on_complete: true
`

	return os.WriteFile(configPath, []byte(template), 0644)
}

func loadConfig() (*Config, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func configExists() bool {
	configPath, err := getConfigPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(configPath)
	return err == nil
}

func validateConfig(cfg *Config) error {
	if cfg.Webhook == "" || cfg.Webhook == `""` {
		return fmt.Errorf("webhook not configured. please add your discord webhook url to ~/.config/crtmon/provider.yaml")
	}
	if len(cfg.Targets) == 0 {
		return fmt.Errorf("no targets configured. please add target domains to ~/.config/crtmon/provider.yaml")
	}
	return nil
}

func updateWebhook(newWebhook string) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	config.Webhook = newWebhook

	newData, err := yaml.Marshal(&config)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, newData, 0644)
}

// updateWebhooksConfig updates all webhook configurations
func updateWebhooksConfig(webhooks struct {
	MainWebhook    string `json:"main_webhook"`
	TelegramBot    string `json:"telegram_bot"`
	TelegramChat   string `json:"telegram_chat"`
	NewDomains     string `json:"new_domains"`
	SubdomainScans string `json:"subdomain_scans"`
	DirectoryScans string `json:"directory_scans"`
	DailySummary   string `json:"daily_summary"`
}) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	config.Webhook = webhooks.MainWebhook
	config.TelegramBotToken = webhooks.TelegramBot
	config.TelegramChatID = webhooks.TelegramChat
	config.Webhooks.NewDomains = webhooks.NewDomains
	config.Webhooks.SubdomainScans = webhooks.SubdomainScans
	config.Webhooks.DirectoryScans = webhooks.DirectoryScans
	config.Webhooks.DailySummary = webhooks.DailySummary

	newData, err := yaml.Marshal(&config)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, newData, 0644)
}

// getConfig returns the global config
func getConfig() *Config {
	return globalConfig
}

// SaveConfig saves the current global config to file
func SaveConfig() error {
	if globalConfig == nil {
		return fmt.Errorf("no config loaded")
	}

	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(globalConfig)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
