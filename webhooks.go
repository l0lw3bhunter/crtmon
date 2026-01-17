package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// WebhookConfig holds all webhook URLs for different message types
type WebhookConfig struct {
	NewDomains       string `yaml:"new_domains_webhook"`
	SubdomainScans   string `yaml:"subdomain_scans_webhook"`
	DirectoryScans   string `yaml:"directory_scans_webhook"`
	DailySummary     string `yaml:"daily_summary_webhook"`
}

var webhookConfig *WebhookConfig
var webhookMutex sync.Mutex

// SetWebhookConfig sets the webhook configuration
func SetWebhookConfig(cfg *WebhookConfig) {
	webhookMutex.Lock()
	defer webhookMutex.Unlock()
	webhookConfig = cfg
}

// GetWebhookConfig returns the webhook configuration
func GetWebhookConfig() *WebhookConfig {
	webhookMutex.Lock()
	defer webhookMutex.Unlock()
	return webhookConfig
}

// SendToWebhook sends a payload to a specific webhook
func SendToWebhook(webhookURL string, payload map[string]interface{}) error {
	if webhookURL == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Error("failed to marshal webhook payload", "error", err)
		return err
	}

	for attempt := 0; attempt < 3; attempt++ {
		resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			logger.Error("failed to send webhook", "attempt", attempt+1, "error", err)
			if attempt < 2 {
				time.Sleep(time.Second * time.Duration(attempt+1))
			}
			continue
		}

		switch resp.StatusCode {
		case http.StatusOK, http.StatusNoContent:
			resp.Body.Close()
			return nil
		case http.StatusTooManyRequests:
			resp.Body.Close()
			if attempt < 2 {
				time.Sleep(2 * time.Second)
			}
		default:
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			logger.Error("webhook returned error", "status", resp.StatusCode, "body", string(bodyBytes))
			return fmt.Errorf("webhook returned status %d", resp.StatusCode)
		}
	}

	return fmt.Errorf("failed to send webhook after retries")
}

// SendNewDomainNotification sends a new domain notification
func SendNewDomainNotification(domain string, rootDomain string, statusCode int, responseSize int, lineCount int, wordCount int) error {
	cfg := GetWebhookConfig()
	if cfg == nil || cfg.NewDomains == "" {
		return fmt.Errorf("new domains webhook not configured")
	}

	payload := buildNewDomainPayload(domain, rootDomain, statusCode, responseSize, lineCount, wordCount)
	return SendToWebhook(cfg.NewDomains, payload)
}

// SendSubdomainScanResults sends subdomain enumeration results
func SendSubdomainScanResults(domain string, results []string) error {
	cfg := GetWebhookConfig()
	if cfg == nil || cfg.SubdomainScans == "" {
		return fmt.Errorf("subdomain scans webhook not configured")
	}

	payload := buildSubdomainScanPayload(domain, results)
	return SendToWebhook(cfg.SubdomainScans, payload)
}

// SendDirectoryScanResults sends directory enumeration results
func SendDirectoryScanResults(domain string, results []string) error {
	cfg := GetWebhookConfig()
	if cfg == nil || cfg.DirectoryScans == "" {
		return fmt.Errorf("directory scans webhook not configured")
	}

	payload := buildDirectoryScanPayload(domain, results)
	return SendToWebhook(cfg.DirectoryScans, payload)
}

// SendDailySummary sends the daily summary
func SendDailySummary(summary map[string]interface{}) error {
	cfg := GetWebhookConfig()
	if cfg == nil || cfg.DailySummary == "" {
		return fmt.Errorf("daily summary webhook not configured")
	}

	payload := buildDailySummaryPayload(summary)
	return SendToWebhook(cfg.DailySummary, payload)
}

// buildNewDomainPayload builds a Discord embed for new domain notification
func buildNewDomainPayload(domain string, rootDomain string, statusCode int, responseSize int, lineCount int, wordCount int) map[string]interface{} {
	statusStr := fmt.Sprintf("%d", statusCode)
	if statusCode == 0 {
		statusStr = "Unknown"
	}

	description := fmt.Sprintf("```\nStatus Code: %s\nResponse Size: %d bytes\nLine Count: %d\nWord Count: %d\n```\n`%s`",
		statusStr, responseSize, lineCount, wordCount, domain)

	return map[string]interface{}{
		"tts": false,
		"embeds": []map[string]interface{}{
			{
				"title":       rootDomain,
				"description": description,
				"color":       3447003, // Blue
				"timestamp":   time.Now().Format(time.RFC3339),
			},
		},
	}
}

// buildSubdomainScanPayload builds a Discord embed for subdomain scan results
func buildSubdomainScanPayload(domain string, results []string) map[string]interface{} {
	resultList := strings.Join(results, "\n")
	if len(resultList) > 4000 {
		resultList = resultList[:4000] + "\n... (truncated)"
	}

	return map[string]interface{}{
		"tts": false,
		"embeds": []map[string]interface{}{
			{
				"title":       fmt.Sprintf("Subdomain Scan: %s", domain),
				"description": fmt.Sprintf("```\n%s\n```", resultList),
				"color":       65280, // Green
				"footer": map[string]string{
					"text": "Scan completed",
				},
				"timestamp": time.Now().Format(time.RFC3339),
			},
		},
	}
}

// buildDirectoryScanPayload builds a Discord embed for directory scan results
func buildDirectoryScanPayload(domain string, results []string) map[string]interface{} {
	resultList := strings.Join(results, "\n")
	if len(resultList) > 4000 {
		resultList = resultList[:4000] + "\n... (truncated)"
	}

	return map[string]interface{}{
		"tts": false,
		"embeds": []map[string]interface{}{
			{
				"title":       fmt.Sprintf("Directory Scan: %s", domain),
				"description": fmt.Sprintf("```\n%s\n```", resultList),
				"color":       16776960, // Yellow
				"footer": map[string]string{
					"text": "Scan completed",
				},
				"timestamp": time.Now().Format(time.RFC3339),
			},
		},
	}
}

// buildDailySummaryPayload builds a Discord embed for daily summary
func buildDailySummaryPayload(summary map[string]interface{}) map[string]interface{} {
	description := "**Daily Summary**\n"

	if domainsCount, ok := summary["discovered_count"].(int); ok {
		description += fmt.Sprintf("📊 Domains Discovered: %d\n", domainsCount)
	}

	if blacklisted, ok := summary["blacklisted_domains"].([]string); ok && len(blacklisted) > 0 {
		description += fmt.Sprintf("🚫 Blacklisted: %d\n", len(blacklisted))
		for _, domain := range blacklisted {
			description += fmt.Sprintf("   • %s\n", domain)
		}
	}

	if topDomains, ok := summary["top_hit_domains"].([]map[string]interface{}); ok && len(topDomains) > 0 {
		description += "\n📈 Top Domains by Hit Count:\n"
		for i, dm := range topDomains {
			if i >= 10 {
				break
			}
			if domain, ok := dm["domain"].(string); ok {
				if hits, ok := dm["hits"].(int); ok {
					description += fmt.Sprintf("   %d. %s - %d hits\n", i+1, domain, hits)
				}
			}
		}
	}

	return map[string]interface{}{
		"tts": false,
		"embeds": []map[string]interface{}{
			{
				"title":       "Daily Summary - " + time.Now().Format("2006-01-02"),
				"description": description,
				"color":       12745742, // Purple
				"timestamp":   time.Now().Format(time.RFC3339),
			},
		},
	}
}
