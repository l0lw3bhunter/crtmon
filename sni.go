package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// SNIManager handles SNI IP range file downloads and domain searching
type SNIManager struct {
	sniFilePath      string
	lastUpdateFile   string
	previousResultsFile string
	mu               sync.RWMutex
}

var sniManager *SNIManager

// InitSNIManager initializes the SNI manager
func InitSNIManager(sniPath string) {
	sniManager = &SNIManager{
		sniFilePath:      sniPath,
		lastUpdateFile:   sniPath + ".lastupdate",
		previousResultsFile: sniPath + ".previous",
	}
	
	// Start monthly refresh scheduler
	go sniManager.startMonthlyScheduler()
	logger.Info("SNI manager initialized", "path", sniPath)
}

// GetSNIManager returns the SNI manager instance
func GetSNIManager() *SNIManager {
	return sniManager
}

// SNI source URLs
var sniSources = []string{
	"https://kaeferjaeger.gay/sni-ip-ranges/amazon/ipv4_merged_sni.txt",
	"https://kaeferjaeger.gay/sni-ip-ranges/google/ipv4_merged_sni.txt",
	"https://kaeferjaeger.gay/sni-ip-ranges/digitalocean/ipv4_merged_sni.txt",
	"https://kaeferjaeger.gay/sni-ip-ranges/microsoft/ipv4_merged_sni.txt",
	"https://kaeferjaeger.gay/sni-ip-ranges/oracle/ipv4_merged_sni.txt",
}

// startMonthlyScheduler starts the monthly SNI file refresh scheduler
func (sm *SNIManager) startMonthlyScheduler() {
	// Check if we should update on startup
	shouldUpdate := sm.shouldUpdateSNIFile()
	if shouldUpdate {
		logger.Info("SNI file update due - refreshing on startup")
		sm.RefreshSNIFiles()
	}
	
	// Schedule monthly updates
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	
	for range ticker.C {
		if sm.shouldUpdateSNIFile() {
			logger.Info("SNI file monthly refresh triggered")
			sm.RefreshSNIFiles()
		}
	}
}

// shouldUpdateSNIFile checks if SNI file should be updated
func (sm *SNIManager) shouldUpdateSNIFile() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	lastUpdate, err := os.Stat(sm.lastUpdateFile)
	if err != nil {
		// File doesn't exist or error reading - should update
		return true
	}
	
	// Check if last update was more than 30 days ago
	return time.Since(lastUpdate.ModTime()) > 30*24*time.Hour
}

// RefreshSNIFiles downloads all SNI files and refreshes sni.txt
func (sm *SNIManager) RefreshSNIFiles() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	logger.Info("starting SNI file refresh")
	
	// Create temp file for new data
	tmpFile := sm.sniFilePath + ".tmp"
	out, err := os.Create(tmpFile)
	if err != nil {
		logger.Error("failed to create temp SNI file", "error", err)
		return err
	}
	defer out.Close()
	
	// Download each source and append to file
	for _, source := range sniSources {
		logger.Info("downloading SNI file", "source", source)
		if err := sm.downloadAndAppend(source, out); err != nil {
			logger.Error("failed to download SNI file", "source", source, "error", err)
			continue // Continue with other sources
		}
	}
	
	out.Close()
	
	// Backup old file and replace with new
	if _, err := os.Stat(sm.sniFilePath); err == nil {
		// Old file exists, back it up
		backupFile := sm.sniFilePath + ".backup"
		os.Rename(sm.sniFilePath, backupFile)
	}
	
	if err := os.Rename(tmpFile, sm.sniFilePath); err != nil {
		logger.Error("failed to replace SNI file", "error", err)
		return err
	}
	
	// Update last update timestamp
	if err := os.WriteFile(sm.lastUpdateFile, []byte(time.Now().Format(time.RFC3339)), 0644); err != nil {
		logger.Error("failed to write last update file", "error", err)
	}
	
	logger.Info("SNI file refresh completed", "path", sm.sniFilePath)
	
	// Re-check all targets for new domains
	sm.recheckAllTargets()
	
	return nil
}

// downloadAndAppend downloads a SNI file and appends to output
func (sm *SNIManager) downloadAndAppend(source string, out *os.File) error {
	resp, err := http.Get(source)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	
	// Copy response to output file
	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to write: %w", err)
	}
	
	// Ensure newline between sources
	out.WriteString("\n")
	
	return nil
}

// SearchSNIForDomain searches sni.txt for a domain and extracts related domains
func (sm *SNIManager) SearchSNIForDomain(domain string) ([]string, error) {
	sm.mu.RLock()
	sniPath := sm.sniFilePath
	sm.mu.RUnlock()
	
	// Check if SNI file exists
	if _, err := os.Stat(sniPath); err != nil {
		logger.Warn("SNI file does not exist yet", "path", sniPath)
		return []string{}, nil
	}
	
	// Run the grep/awk/sed chain to extract domains
	// Command: cat sni.txt | grep -F ".{domain}" | awk -F' -- ''{print $2}' | tr '\n' | tr '[' | sed 's/ //' | sed 's/\]//' | grep -F ".{domain}" | sort -u
	
	results := make(map[string]bool) // Use map for dedup
	
	file, err := os.Open(sniPath)
	if err != nil {
		logger.Error("failed to open SNI file", "error", err)
		return []string{}, err
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		
		// Check if line contains domain
		if !strings.Contains(line, "."+domain) && !strings.Contains(line, domain) {
			continue
		}
		
		// Parse line format: IP -- domain1, domain2, domain3, etc.
		parts := strings.Split(line, " -- ")
		if len(parts) < 2 {
			continue
		}
		
		// Extract domains from second part
		domainsStr := parts[1]
		
		// Clean up brackets and whitespace
		domainsStr = strings.ReplaceAll(domainsStr, "[", "")
		domainsStr = strings.ReplaceAll(domainsStr, "]", "")
		domainsStr = strings.ReplaceAll(domainsStr, " ", "")
		
		// Split by comma
		domains := strings.Split(domainsStr, ",")
		
		for _, d := range domains {
			d = strings.TrimSpace(d)
			
			// Only include if it contains our target domain
			if strings.Contains(d, "."+domain) || d == domain {
				results[d] = true
			}
		}
	}
	
	// Convert map to sorted slice
	var resultSlice []string
	for domain := range results {
		resultSlice = append(resultSlice, domain)
	}
	sort.Strings(resultSlice)
	
	logger.Info("SNI search completed", "target_domain", domain, "found_count", len(resultSlice))
	return resultSlice, nil
}

// recheckAllTargets re-searches all current targets in SNI file after update
func (sm *SNIManager) recheckAllTargets() {
	dt := GetDomainTracker()
	if dt == nil {
		return
	}
	
	// Get all targets from config
	cfg := getConfig()
	if cfg == nil || len(cfg.Targets) == 0 {
		return
	}
	
	logger.Info("rechecking all targets after SNI update")
	
	newDiscoveries := make(map[string][]string)
	
	for _, target := range cfg.Targets {
		logger.Info("rechecking SNI for target", "target", target)
		
		// Search SNI for this target
		domains, err := sm.SearchSNIForDomain(target)
		if err != nil || len(domains) == 0 {
			continue
		}
		
		// Track which domains are new (not previously seen)
		previousDomains := sm.getPreviousResults(target)
		newDomains := []string{}
		
		for _, domain := range domains {
			isNew := true
			for _, prev := range previousDomains {
				if prev == domain {
					isNew = false
					break
				}
			}
			
			if isNew {
				newDomains = append(newDomains, domain)
				logger.Info("found new domain in SNI", "target", target, "domain", domain)
			}
		}
		
		if len(newDomains) > 0 {
			newDiscoveries[target] = newDomains
		}
	}
	
	// Save current results as previous for next update
	sm.savePreviousResults(newDiscoveries)
	
	// Send Discord notification and trigger enumeration
	if len(newDiscoveries) > 0 {
		sm.notifyAndEnumerateNewDomains(newDiscoveries)
	}
}

// getPreviousResults retrieves previously found domains for a target
func (sm *SNIManager) getPreviousResults(target string) []string {
	sm.mu.RLock()
	previousFile := sm.previousResultsFile
	sm.mu.RUnlock()
	
	data, err := os.ReadFile(previousFile)
	if err != nil {
		return []string{}
	}
	
	// Simple format: target|domain1,domain2,domain3\n
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) == 2 && parts[0] == target {
			domains := strings.Split(parts[1], ",")
			var result []string
			for _, d := range domains {
				if d = strings.TrimSpace(d); d != "" {
					result = append(result, d)
				}
			}
			return result
		}
	}
	
	return []string{}
}

// savePreviousResults saves the current found domains as previous for next update
func (sm *SNIManager) savePreviousResults(newDiscoveries map[string][]string) {
	sm.mu.Lock()
	previousFile := sm.previousResultsFile
	sm.mu.Unlock()
	
	// Also load all existing domains (from before this update)
	existingResults := make(map[string][]string)
	data, err := os.ReadFile(previousFile)
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			parts := strings.Split(line, "|")
			if len(parts) == 2 {
				target := parts[0]
				domains := strings.Split(parts[1], ",")
				var result []string
				for _, d := range domains {
					if d = strings.TrimSpace(d); d != "" {
						result = append(result, d)
					}
				}
				existingResults[target] = result
			}
		}
	}
	
	// Merge new discoveries with existing
	for target, newDomains := range newDiscoveries {
		existingResults[target] = append(existingResults[target], newDomains...)
		
		// Deduplicate
		seen := make(map[string]bool)
		var unique []string
		for _, d := range existingResults[target] {
			if !seen[d] {
				unique = append(unique, d)
				seen[d] = true
			}
		}
		sort.Strings(unique)
		existingResults[target] = unique
	}
	
	// Write to file
	var lines []string
	for target, domains := range existingResults {
		if len(domains) > 0 {
			lines = append(lines, fmt.Sprintf("%s|%s", target, strings.Join(domains, ",")))
		}
	}
	sort.Strings(lines)
	
	content := strings.Join(lines, "\n")
	if err := os.WriteFile(previousFile, []byte(content), 0644); err != nil {
		logger.Error("failed to save previous results", "error", err)
	}
}

// notifyAndEnumerateNewDomains sends Discord notification and triggers enumeration
func (sm *SNIManager) notifyAndEnumerateNewDomains(newDiscoveries map[string][]string) {
	for target, newDomains := range newDiscoveries {
		logger.Info("SNI discovery notification", "target", target, "count", len(newDomains))
		
		// Send Discord notification
		sendSNIDiscoveryNotification(target, newDomains)
		
		// Trigger enumeration for each domain
		for _, domain := range newDomains {
			go sm.enumerateSNIDomain(target, domain)
		}
	}
}

// enumerateSNIDomain determines if domain should go to puredns (wildcard) or feroxbuster
func (sm *SNIManager) enumerateSNIDomain(target, domain string) {
	logger.Info("enumerating SNI domain", "target", target, "domain", domain)
	
	isWildcard := strings.HasPrefix(domain, "*.")
	
	if isWildcard {
		// Wildcard - use puredns
		baseDomain := ExtractBaseDomain(domain)
		if _, err := RunPuredns(baseDomain, target); err != nil {
			logger.Error("failed to start puredns for SNI domain", "domain", baseDomain, "error", err)
		}
	} else {
		// Non-wildcard - use feroxbuster
		if _, err := RunFeroxbuster(domain, target); err != nil {
			logger.Error("failed to start feroxbuster for SNI domain", "domain", domain, "error", err)
		}
	}
}

// SearchSNIOnDemand performs an immediate SNI search for a domain (for when target is added)
func (sm *SNIManager) SearchSNIOnDemand(target string) {
	go func() {
		time.Sleep(1 * time.Second) // Brief delay for SNI file to be available
		
		domains, err := sm.SearchSNIForDomain(target)
		if err != nil || len(domains) == 0 {
			logger.Info("no SNI results for new target", "target", target)
			return
		}
		
		logger.Info("found SNI domains for new target", "target", target, "count", len(domains))
		
		// Save these as previous results since these are the initial findings
		sm.savePreviousResults(map[string][]string{target: domains})
		
		// Trigger enumeration
		for _, domain := range domains {
			go sm.enumerateSNIDomain(target, domain)
		}
	}()
}
