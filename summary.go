package main

import (
	"sort"
	"sync"
	"time"
)

var summaryMutex sync.Mutex
var lastSummarySent time.Time

// StartDailySummaryScheduler starts the daily summary scheduler
func StartDailySummaryScheduler() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			now := time.Now()
			// Send summary at 00:01 every day
			if now.Hour() == 0 && now.Minute() >= 0 && now.Minute() < 5 {
				summaryMutex.Lock()
				if time.Since(lastSummarySent) > 12*time.Hour {
					summaryMutex.Unlock()
					GenerateAndSendDailySummary()
					summaryMutex.Lock()
					lastSummarySent = time.Now()
				}
				summaryMutex.Unlock()
			}
		}
	}()
}

// GenerateAndSendDailySummary generates and sends the daily summary
func GenerateAndSendDailySummary() {
	dt := GetDomainTracker()

	// Get all domains discovered today
	domainsToday := dt.GetDomainsDiscoveredToday()
	discoveredCount := len(domainsToday)

	// Get blacklisted domains
	blacklistedDomains := dt.GetBlacklistedDomains()
	var blacklistedNames []string
	for _, domain := range blacklistedDomains {
		blacklistedNames = append(blacklistedNames, domain.Domain)
	}

	// Get top domains by hit count (organized by root domain)
	topHitsByRoot := getTopDomainsByRoot(domainsToday)

	summary := map[string]interface{}{
		"discovered_count":  discoveredCount,
		"blacklisted_count": len(blacklistedNames),
		"blacklisted_domains": blacklistedNames,
		"top_hit_domains":   topHitsByRoot,
		"timestamp":         time.Now().Unix(),
	}

	if err := SendDailySummary(summary); err != nil {
		logger.Error("failed to send daily summary", "error", err)
	} else {
		logger.Info("daily summary sent", "domains_count", discoveredCount, "blacklisted", len(blacklistedNames))
	}
}

// getTopDomainsByRoot gets top domains organized by root domain
func getTopDomainsByRoot(domains []*DomainEntry) []map[string]interface{} {
	// Group by root domain
	rootDomainMap := make(map[string][]*DomainEntry)

	for _, entry := range domains {
		root := extractRootDomain(entry.Domain)
		rootDomainMap[root] = append(rootDomainMap[root], entry)
	}

	// Sort each root domain's subdomains by hit count
	var result []map[string]interface{}

	for root, subs := range rootDomainMap {
		sort.Slice(subs, func(i, j int) bool {
			return subs[i].HitCount > subs[j].HitCount
		})

		// Get top 5 for this root
		limit := 5
		if len(subs) < limit {
			limit = len(subs)
		}

		rootEntry := map[string]interface{}{
			"domain":     root,
			"total_hits": getTotalHits(subs),
			"subdomains": make([]map[string]interface{}, 0),
		}

		subdomains := rootEntry["subdomains"].([]map[string]interface{})
		for i := 0; i < limit; i++ {
			subdomains = append(subdomains, map[string]interface{}{
				"domain": subs[i].Domain,
				"hits":   subs[i].HitCount,
			})
		}
		rootEntry["subdomains"] = subdomains

		result = append(result, rootEntry)
	}

	// Sort root domains by total hits
	sort.Slice(result, func(i, j int) bool {
		return result[i]["total_hits"].(int) > result[j]["total_hits"].(int)
	})

	return result
}

// getTotalHits gets total hits for a list of domains
func getTotalHits(domains []*DomainEntry) int {
	total := 0
	for _, d := range domains {
		total += d.HitCount
	}
	return total
}
