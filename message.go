package main

import (
	"fmt"
	"strings"
	"time"
)

func buildDiscordPayload(target string, domains []string) map[string]interface{} {
	domainList := strings.Builder{}
	dt := GetDomainTracker()

	for _, domain := range domains {
		hitCount := dt.GetDomainHitCount(domain)
		if hitCount > 1 {
			domainList.WriteString(fmt.Sprintf("%s  [hit: %d]\n", domain, hitCount))
		} else {
			domainList.WriteString(domain + "\n")
		}
	}

	return map[string]interface{}{
		"tts": false,
		"embeds": []map[string]interface{}{
			{
				"title":       fmt.Sprintf("%s  [%d]", target, len(domains)),
				"description": fmt.Sprintf("```\n%s\n```", strings.TrimSuffix(domainList.String(), "\n")),
				"color":       2829617,
				// "author": map[string]string{
				// 	"name": "1hehaq/ceye",
				// 	"url":  "https://github.com/1hehaq/ceye",
				// },
				"timestamp": time.Now().Format(time.RFC3339),
			},
		},
	}
}

func buildTelegramMessage(target string, domains []string) string {
	domainList := strings.Builder{}
	dt := GetDomainTracker()

	for _, domain := range domains {
		hitCount := dt.GetDomainHitCount(domain)
		if hitCount > 1 {
			domainList.WriteString(fmt.Sprintf("%s  [hit: %d]\n", domain, hitCount))
		} else {
			domainList.WriteString(domain + "\n")
		}
	}

	return fmt.Sprintf("*%s* [%d]\n```%s```", target, len(domains), strings.TrimSuffix(domainList.String(), "\n"))
}

// sendNewDomainToWebhook sends a new domain to the new domains webhook
func sendNewDomainToWebhook(domain string, rootDomain string) {
	dt := GetDomainTracker()
	entry := dt.GetDomainInfo(domain)

	statusCode := 0
	responseSize := 0
	lineCount := 0
	wordCount := 0

	if entry != nil {
		statusCode = entry.HttpStatusCode
		responseSize = entry.ResponseSize
		lineCount = entry.ResponseLineCount
		wordCount = entry.ResponseWordCount
	}

	// Try to send to new domains webhook
	if err := SendNewDomainNotification(domain, rootDomain, statusCode, responseSize, lineCount, wordCount); err != nil {
		logger.Debug("failed to send to new domains webhook", "domain", domain, "error", err)
	}
}

// extractRootDomain extracts the root domain from a full domain
func extractRootDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "." + parts[len(parts)-1]
	}
	return domain
}
