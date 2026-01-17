package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	batchDelay    = 5 * time.Second
	maxBatchSize  = 25
	rateLimitWait = 2 * time.Second
	maxRetries    = 3
)

type notificationBuffer struct {
	mu      sync.Mutex
	pending map[string][]string
	timers  map[string]*time.Timer
}

var notifier = &notificationBuffer{
	pending: make(map[string][]string),
	timers:  make(map[string]*time.Timer),
}

func sendToDiscord(domain, target string) {
	notifier.add(target, domain)
}

func (n *notificationBuffer) add(target, domain string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.pending[target] = append(n.pending[target], domain)

	if len(n.pending[target]) >= maxBatchSize {
		domains := n.pending[target]
		delete(n.pending, target)
		if timer, exists := n.timers[target]; exists {
			timer.Stop()
			delete(n.timers, target)
		}
		go n.send(target, domains)
		return
	}

	if _, exists := n.timers[target]; !exists {
		n.timers[target] = time.AfterFunc(batchDelay, func() {
			n.flush(target)
		})
	}
}

func (n *notificationBuffer) flush(target string) {
	n.mu.Lock()
	domains, exists := n.pending[target]
	if !exists || len(domains) == 0 {
		n.mu.Unlock()
		return
	}
	delete(n.pending, target)
	delete(n.timers, target)
	n.mu.Unlock()

	n.send(target, domains)
}

func (n *notificationBuffer) send(target string, domains []string) {
	if notifyDiscord && webhookURL != "" {
		n.sendDiscord(target, domains)
	}

	if notifyTelegram {
		sendToTelegram(target, domains)
	}

	// Trigger enumeration if enabled
	enumMutex.Lock()
	enumEnabled := enumConfig != nil && enumConfig.EnableEnum
	enumMutex.Unlock()

	if enumEnabled {
		for _, domain := range domains {
			go triggerEnumeration(domain, target)
		}
	}
}

func (n *notificationBuffer) sendDiscord(target string, domains []string) {
	payload := buildDiscordPayload(target, domains)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Error("failed to marshal discord payload", "error", err)
		return
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			logger.Error("failed to send discord notification", "error", err)
			return
		}

		   switch resp.StatusCode {
		   case http.StatusOK, http.StatusNoContent:
			   resp.Body.Close()

			   // Send to new domains webhook if configured
			   if GetWebhookConfig() != nil && GetWebhookConfig().NewDomains != "" {
				   for _, domain := range domains {
					   sendNewDomainToWebhook(domain, extractRootDomain(domain))
				   }
			   }

			   // Trigger enumeration after successful send (if enabled)
			   enumMutex.Lock()
			   enumEnabled := enumConfig != nil && enumConfig.EnableEnum
			   enumMutex.Unlock()

			   if enumEnabled {
				   for _, domain := range domains {
					   go triggerEnumeration(domain, target)
				   }
			   }
			   return
		case http.StatusTooManyRequests:
			resp.Body.Close()
			logger.Warn("discord rate limited, waiting", "attempt", attempt+1)
			time.Sleep(rateLimitWait * time.Duration(attempt+1))
			continue
		default:
			resp.Body.Close()
			logger.Warn("discord webhook error", "status", resp.StatusCode)
			return
		}
	}

	logger.Error("failed to send discord after retries", "target", target)
}

func sendToTelegram(target string, domains []string) {
	if telegramToken == "" || telegramChatID == "" {
		return
	}

	text := buildTelegramMessage(target, domains)

	payload := map[string]interface{}{
		"chat_id":                  telegramChatID,
		"text":                     text,
		"parse_mode":               "Markdown",
		"disable_web_page_preview": true,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Error("failed to marshal telegram payload", "error", err)
		return
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", telegramToken)

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			logger.Error("failed to send telegram notification", "error", err)
			return
		}

		if resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			logger.Warn("telegram rate limited, waiting", "attempt", attempt+1)
			time.Sleep(rateLimitWait * time.Duration(attempt+1))
			continue
		}

		resp.Body.Close()
		logger.Warn("telegram send error", "status", resp.StatusCode)
		return
	}

	logger.Error("failed to send telegram after retries", "target", target)
}

// triggerEnumeration starts enumeration based on domain type
func triggerEnumeration(domain, target string) {
	if IsWildcardDomain(domain) {
		// Wildcard domain - use puredns
		baseDomain := ExtractBaseDomain(domain)
		logger.Info("wildcard detected, starting puredns", "domain", baseDomain)
		_, err := RunPuredns(baseDomain, target)
		if err != nil {
			logger.Error("failed to start puredns", "domain", baseDomain, "error", err)
		}
	} else {
		// Regular subdomain - use feroxbuster
		logger.Info("starting feroxbuster", "domain", domain)
		_, err := RunFeroxbuster(domain, target)
		if err != nil {
			logger.Error("failed to start feroxbuster", "domain", domain, "error", err)
		}
	}
}
// sendSNIDiscoveryNotification sends a Discord notification for SNI discoveries
func sendSNIDiscoveryNotification(target string, newDomains []string) {
	if !notifyDiscord || webhookURL == "" {
		return
	}

	// Group by wildcard vs regular domains
	wildcards := []string{}
	regular := []string{}

	for _, domain := range newDomains {
		if strings.HasPrefix(domain, "*.") {
			wildcards = append(wildcards, domain)
		} else {
			regular = append(regular, domain)
		}
	}

	description := fmt.Sprintf("**Target**: %s\n\n", target)

	if len(wildcards) > 0 {
		description += fmt.Sprintf("**Wildcards** (%d):\n```\n%s\n```\n\n", 
			len(wildcards), strings.Join(wildcards, "\n"))
	}

	if len(regular) > 0 {
		description += fmt.Sprintf("**Regular Domains** (%d):\n```\n%s\n```\n\n",
			len(regular), strings.Join(regular, "\n"))
	}

	description += fmt.Sprintf("*Total: %d new domains found*", len(newDomains))

	payload := map[string]interface{}{
		"tts": false,
		"embeds": []map[string]interface{}{
			{
				"title":       "🔍 SNI File Updated - New Domains Discovered",
				"description": description,
				"color":       9764863, // Purple
				"timestamp":   time.Now().Format(time.RFC3339),
			},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Error("failed to marshal SNI notification", "error", err)
		return
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Error("failed to send SNI notification", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		logger.Error("discord returned error for SNI notification", "status", resp.StatusCode)
	}
}