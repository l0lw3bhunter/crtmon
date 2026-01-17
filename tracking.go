package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DomainTracker tracks domain hits and deduplication
type DomainTracker struct {
	mu       sync.RWMutex
	domains  map[string]*DomainEntry
	filePath string
}

// DomainEntry holds tracking information for a domain
type DomainEntry struct {
	Domain              string              `json:"domain"`
	HitCount            int                 `json:"hit_count"`
	FirstSeen           time.Time           `json:"first_seen"`
	LastSeen            time.Time           `json:"last_seen"`
	LastNotified        time.Time           `json:"last_notified"`
	Resolved            bool                `json:"resolved"`
	Blacklisted         bool                `json:"blacklisted"`
	BlacklistedDate     time.Time           `json:"blacklisted_date"`
	DailyHits           map[string]int      `json:"daily_hits"`            // Date (YYYY-MM-DD) -> hit count
	HighHitDays         int                 `json:"high_hit_days_consecutive"` // Consecutive days with >10 hits
	LastHighHitDate     string              `json:"last_high_hit_date"`
	HttpStatusCode      int                 `json:"http_status_code"`
	ResponseSize        int                 `json:"response_size"`
	ResponseLineCount   int                 `json:"response_line_count"`
	ResponseWordCount   int                 `json:"response_word_count"`
	// Risk indicators
	RiskLabels          []string            `json:"risk_labels"`           // Tags: "wildcard", "status-anomaly", "issuer-change", "high-frequency"
	RiskScore           int                 `json:"risk_score"`            // 0-100 composite score
	CertIssuer          string              `json:"cert_issuer"`           // Last seen certificate issuer
	PreviousIssuer      string              `json:"previous_issuer"`       // Track issuer changes
	StatusCodeHistory   []int               `json:"status_code_history"`   // Recent status codes
	IsDuplicate         bool                `json:"is_duplicate"`          // Marked as duplicate/noise
}

var tracker *DomainTracker

// InitDomainTracker initializes the domain tracker
func InitDomainTracker(configDir string) error {
	filePath := filepath.Join(configDir, "domain_tracking.json")

	t := &DomainTracker{
		domains:  make(map[string]*DomainEntry),
		filePath: filePath,
	}

	// Load existing tracking data
	if err := t.load(); err != nil && !os.IsNotExist(err) {
		logger.Error("failed to load domain tracking data", "error", err)
		// Continue with empty tracker rather than failing
	}

	tracker = t
	logger.Info("domain tracker initialized", "path", filePath)
	return nil
}

// GetDomainTracker returns the global domain tracker
func GetDomainTracker() *DomainTracker {
	if tracker == nil {
		// Initialize with default path if not already initialized
		configDir, _ := getConfigDir()
		InitDomainTracker(configDir)
	}
	return tracker
}

// ShouldNotifyDomain checks if a domain should be notified
// Returns true if:
// - Domain hasn't been notified before, OR
// - Domain is not blacklisted, AND
// - Last notification was more than 7 days ago, AND
// - Only notifies once per day regardless
func (dt *DomainTracker) ShouldNotifyDomain(domain string) bool {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))

	dt.mu.Lock()
	defer dt.mu.Unlock()

	entry, exists := dt.domains[d]
	if !exists {
		// New domain
		today := time.Now().Format("2006-01-02")
		dt.domains[d] = &DomainEntry{
			Domain:       d,
			HitCount:     1,
			FirstSeen:    time.Now(),
			LastSeen:     time.Now(),
			LastNotified: time.Now(),
			Resolved:     false,
			DailyHits:    map[string]int{today: 1},
		}
		return true
	}

	// Check if blacklisted
	   if entry.Blacklisted {
		   entry.HitCount++
		   entry.LastSeen = time.Now()
		   // Skip updateDailyHits() - domain won't be un-blacklisted
		   return false
	}

	// Check 7-day cooldown
	timeSinceNotified := time.Since(entry.LastNotified)
	today := time.Now().Format("2006-01-02")
	lastNotifiedDate := entry.LastNotified.Format("2006-01-02")

	// Only notify once per day, and check 7-day cooldown
	if timeSinceNotified >= 7*24*time.Hour && today != lastNotifiedDate {
		entry.HitCount++
		entry.LastSeen = time.Now()
		entry.LastNotified = time.Now()
		dt.updateDailyHits(entry)
		return true
	}

	// Still in notification cooldown period or already notified today
	entry.HitCount++
	entry.LastSeen = time.Now()
	dt.updateDailyHits(entry)
	return false
}

// RecordDomainResolution records whether a domain resolved successfully
func (dt *DomainTracker) RecordDomainResolution(domain string, resolved bool) {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))

	dt.mu.Lock()
	defer dt.mu.Unlock()

	entry, exists := dt.domains[d]
	if !exists {
		entry = &DomainEntry{
			Domain:    d,
			FirstSeen: time.Now(),
			LastSeen:  time.Now(),
		}
		dt.domains[d] = entry
	}

	entry.Resolved = resolved
	dt.save()
}

// GetDomainHitCount returns the hit count for a domain
func (dt *DomainTracker) GetDomainHitCount(domain string) int {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))

	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if entry, exists := dt.domains[d]; exists {
		return entry.HitCount
	}
	return 0
}

// GetDomainInfo returns the full domain entry
func (dt *DomainTracker) GetDomainInfo(domain string) *DomainEntry {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))

	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if entry, exists := dt.domains[d]; exists {
		// Return a copy to avoid external modifications
		copy := *entry
		return &copy
	}
	return nil
}

// GetAllDomains returns all tracked domains
func (dt *DomainTracker) GetAllDomains() map[string]*DomainEntry {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	result := make(map[string]*DomainEntry)
	for k, v := range dt.domains {
		copy := *v
		result[k] = &copy
	}
	return result
}

// ClearOldEntries removes domain entries that haven't been seen in 30 days
func (dt *DomainTracker) ClearOldEntries(maxAge time.Duration) int {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	removed := 0
	now := time.Now()

	for domain, entry := range dt.domains {
		if now.Sub(entry.LastSeen) > maxAge {
			delete(dt.domains, domain)
			removed++
		}
	}

	if removed > 0 {
		dt.save()
		logger.Info("cleared old domain entries", "count", removed)
	}

	return removed
}

// load loads the tracking data from disk
func (dt *DomainTracker) load() error {
	data, err := os.ReadFile(dt.filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &dt.domains)
}

// save saves the tracking data to disk
func (dt *DomainTracker) save() error {
	data, err := json.Marshal(dt.domains)
	if err != nil {
		return err
	}

	// Write to temp file first, then rename for atomicity
	tempPath := dt.filePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tempPath, dt.filePath)
}

// ForceNotifyDomain allows forcing a notification even within the 7-day window
func (dt *DomainTracker) ForceNotifyDomain(domain string) {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))

	dt.mu.Lock()
	defer dt.mu.Unlock()

	entry, exists := dt.domains[d]
	if !exists {
		today := time.Now().Format("2006-01-02")
		entry = &DomainEntry{
			Domain:       d,
			HitCount:     1,
			FirstSeen:    time.Now(),
			LastSeen:     time.Now(),
			LastNotified: time.Now(),
			DailyHits:    map[string]int{today: 1},
		}
		dt.domains[d] = entry
	} else {
		entry.HitCount++
		entry.LastSeen = time.Now()
		entry.LastNotified = time.Now()
		dt.updateDailyHits(entry)
	}

	dt.save()
}

// updateDailyHits updates the daily hit count for a domain
func (dt *DomainTracker) updateDailyHits(entry *DomainEntry) {
	if entry.DailyHits == nil {
		entry.DailyHits = make(map[string]int)
	}

	today := time.Now().Format("2006-01-02")
	entry.DailyHits[today]++

	// Check for blacklist trigger (>10 hits per day for 3+ consecutive days)
	if entry.DailyHits[today] > 10 {
		if entry.LastHighHitDate != today {
			// New high-hit day
			yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
			if entry.LastHighHitDate == yesterday {
				// Consecutive high-hit day
				entry.HighHitDays++
			} else {
				// Reset counter
				entry.HighHitDays = 1
			}
			entry.LastHighHitDate = today

			// Blacklist if 3+ consecutive days
			if entry.HighHitDays >= 3 {
				entry.Blacklisted = true
				entry.BlacklistedDate = time.Now()
				logger.Info("domain blacklisted", "domain", entry.Domain, "reason", "10+ hits for 3 consecutive days")
			}
		}
	}

	dt.save()
}

// IsBlacklisted checks if a domain is blacklisted
func (dt *DomainTracker) IsBlacklisted(domain string) bool {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))

	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if entry, exists := dt.domains[d]; exists {
		return entry.Blacklisted
	}
	return false
}

// GetBlacklistedDomains returns all currently blacklisted domains
func (dt *DomainTracker) GetBlacklistedDomains() []*DomainEntry {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	var blacklisted []*DomainEntry
	for _, entry := range dt.domains {
		if entry.Blacklisted {
			copy := *entry
			blacklisted = append(blacklisted, &copy)
		}
	}
	return blacklisted
}

// GetDomainsDiscoveredToday returns domains discovered today
func (dt *DomainTracker) GetDomainsDiscoveredToday() []*DomainEntry {
	t := time.Now().Format("2006-01-02")
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	var discovered []*DomainEntry
	for _, entry := range dt.domains {
		if _, exists := entry.DailyHits[t]; exists && entry.DailyHits[t] > 0 {
			copy := *entry
			discovered = append(discovered, &copy)
		}
	}
	return discovered
}

// RecordDomainMetadata records HTTP response metadata for a domain
func (dt *DomainTracker) RecordDomainMetadata(domain string, statusCode, responseSize, lineCount, wordCount int) {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))

	dt.mu.Lock()
	defer dt.mu.Unlock()

	if entry, exists := dt.domains[d]; exists {
		// Track status code history
		if entry.StatusCodeHistory == nil {
			entry.StatusCodeHistory = []int{}
		}
		// Keep only last 10 status codes
		entry.StatusCodeHistory = append(entry.StatusCodeHistory, statusCode)
		if len(entry.StatusCodeHistory) > 10 {
			entry.StatusCodeHistory = entry.StatusCodeHistory[len(entry.StatusCodeHistory)-10:]
		}
		
		entry.HttpStatusCode = statusCode
		entry.ResponseSize = responseSize
		entry.ResponseLineCount = lineCount
		entry.ResponseWordCount = wordCount
		
		// Calculate risk after metadata update
		dt.calculateRisk(entry)
		dt.save()
	}
}

// RecordDomainIssuer records certificate issuer information
func (dt *DomainTracker) RecordDomainIssuer(domain string, issuer string) {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))

	dt.mu.Lock()
	defer dt.mu.Unlock()

	if entry, exists := dt.domains[d]; exists {
		if entry.CertIssuer != "" && entry.CertIssuer != issuer {
			// Issuer changed
			entry.PreviousIssuer = entry.CertIssuer
			dt.addRiskLabel(entry, "issuer-change")
		}
		entry.CertIssuer = issuer
		dt.calculateRisk(entry)
		dt.save()
	}
}

// calculateRisk computes risk score and labels for a domain
func (dt *DomainTracker) calculateRisk(entry *DomainEntry) {
	if entry.RiskLabels == nil {
		entry.RiskLabels = []string{}
	}
	
	score := 0
	
	// Check for wildcard
	if IsWildcardDomain(entry.Domain) {
		dt.addRiskLabel(entry, "wildcard")
		score += 30
	}
	
	// Check for status anomalies (multiple different status codes)
	if len(entry.StatusCodeHistory) >= 3 {
		uniqueStatuses := make(map[int]bool)
		for _, status := range entry.StatusCodeHistory {
			uniqueStatuses[status] = true
		}
		if len(uniqueStatuses) >= 3 {
			dt.addRiskLabel(entry, "status-anomaly")
			score += 20
		}
	}
	
	// Check for high frequency (>50 hits or high daily average)
	if entry.HitCount > 50 || entry.HighHitDays >= 2 {
		dt.addRiskLabel(entry, "high-frequency")
		score += 25
	}
	
	// Issuer change is already tracked in RecordDomainIssuer
	if entry.PreviousIssuer != "" {
		score += 15
	}
	
	// Suspicious response patterns (very small or very large responses)
	if entry.ResponseSize > 0 {
		if entry.ResponseSize < 100 || entry.ResponseSize > 1000000 {
			dt.addRiskLabel(entry, "response-anomaly")
			score += 10
		}
	}
	
	// Cap score at 100
	if score > 100 {
		score = 100
	}
	
	entry.RiskScore = score
	
	// Auto-mark as duplicate if very high frequency and minimal content
	if entry.HitCount > 100 && entry.ResponseSize < 500 {
		entry.IsDuplicate = true
	}
}

// addRiskLabel adds a label if not already present
func (dt *DomainTracker) addRiskLabel(entry *DomainEntry, label string) {
	for _, existing := range entry.RiskLabels {
		if existing == label {
			return
		}
	}
	entry.RiskLabels = append(entry.RiskLabels, label)
}

// GetHighRiskDomains returns domains with risk score above threshold
func (dt *DomainTracker) GetHighRiskDomains(minScore int) []*DomainEntry {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	var highRisk []*DomainEntry
	for _, entry := range dt.domains {
		if entry.RiskScore >= minScore && !entry.IsDuplicate && !entry.Blacklisted {
			copy := *entry
			highRisk = append(highRisk, &copy)
		}
	}
	return highRisk
}

// GetDomainsByRiskLabel returns domains with a specific risk label
func (dt *DomainTracker) GetDomainsByRiskLabel(label string) []*DomainEntry {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	var matches []*DomainEntry
	for _, entry := range dt.domains {
		for _, l := range entry.RiskLabels {
			if l == label {
				copy := *entry
				matches = append(matches, &copy)
				break
			}
		}
	}
	return matches
}
