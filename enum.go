package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EnumConfig holds enumeration configuration
type EnumConfig struct {
	EnableEnum         bool   `yaml:"enable_enum"`
	FeroxbusterPath    string `yaml:"feroxbuster_path"`
	PurednsPath        string `yaml:"puredns_path"`
	DirWordlist        string `yaml:"dir_wordlist"`
	DNSWordlist        string `yaml:"dns_wordlist"`
	ResolversFile      string `yaml:"resolvers_file"`
	RateLimit          int    `yaml:"rate_limit"`
	RateLimitTrusted   int    `yaml:"rate_limit_trusted"`
	ScanTimeout        int    `yaml:"scan_timeout"`
	NotifyOnComplete   bool   `yaml:"notify_on_complete"`
}

var enumConfig *EnumConfig
var enumMutex sync.Mutex

// SetEnumConfig sets the enumeration configuration
func SetEnumConfig(cfg *EnumConfig) {
	enumMutex.Lock()
	defer enumMutex.Unlock()
	enumConfig = cfg
}

// IsWildcardDomain checks if domain matches wildcard pattern
func IsWildcardDomain(domain string) bool {
	return strings.HasPrefix(domain, "*.")
}

// ExtractBaseDomain extracts base domain from wildcard
func ExtractBaseDomain(domain string) string {
	if IsWildcardDomain(domain) {
		return strings.TrimPrefix(domain, "*.")
	}
	return domain
}

// RunFeroxbuster runs feroxbuster on a subdomain for directory enumeration
func RunFeroxbuster(domain string, target string) (string, error) {
	enumMutex.Lock()
	if enumConfig == nil || !enumConfig.EnableEnum || enumConfig.FeroxbusterPath == "" {
		enumMutex.Unlock()
		return "", fmt.Errorf("feroxbuster not configured")
	}
	cfg := *enumConfig
	enumMutex.Unlock()

	// Construct the feroxbuster command
	url := fmt.Sprintf("https://%s/", domain)
	outputFile := fmt.Sprintf("%s.ferox.txt", strings.ReplaceAll(domain, ".", "_"))

	// Track scan start
	st := GetStatsTracker()
	st.IncrementActiveFeroxScans()
	defer func() {
		// Track scan completion (success determined by error return)
		st.DecrementActiveFeroxScans(true)
	}()

	args := []string{
		"--url", url,
		"--wordlist", cfg.DirWordlist,
		"-A",
		"--rate-limit", fmt.Sprintf("%d", cfg.RateLimit),
		"-o", outputFile,
		"-g",
		"-x", "html,js,xml,json,config,env,txt",
		"-E",
		"--scan-limit", "5",
		"-s", "200,301,302,400",
	}

	// Start the scan in a screen session
	screenName := fmt.Sprintf("enum_%s", strings.ReplaceAll(domain, ".", "_"))
	screenCmd := exec.Command("screen", "-S", screenName, "-d", "-m", cfg.FeroxbusterPath)
	screenCmd.Args = append(screenCmd.Args, args...)

	if err := screenCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to start feroxbuster in screen: %w", err)
	}

	logger.Info("started feroxbuster scan", "domain", domain, "screen", screenName, "output", outputFile)

	// Read output file asynchronously and send to Discord when complete
	go asyncReadAndSendScanResults(target, domain, outputFile, "feroxbuster", cfg.ScanTimeout)

	return outputFile, nil
}

// RunPuredns runs puredns for DNS bruteforce on wildcard domains
func RunPuredns(baseDomain string, target string) (string, error) {
	enumMutex.Lock()
	if enumConfig == nil || !enumConfig.EnableEnum || enumConfig.PurednsPath == "" {
		enumMutex.Unlock()
		return "", fmt.Errorf("puredns not configured")
	}
	cfg := *enumConfig
	enumMutex.Unlock()

	// Track scan start
	st := GetStatsTracker()
	st.IncrementActivePurednsScans()

	outputFile := fmt.Sprintf("%s.puredns.txt", strings.ReplaceAll(baseDomain, ".", "_"))

	args := []string{
		"bruteforce",
		cfg.DNSWordlist,
		baseDomain,
		"--resolvers", cfg.ResolversFile,
		"--rate-limit", fmt.Sprintf("%d", cfg.RateLimit),
		"--rate-limit-trusted", fmt.Sprintf("%d", cfg.RateLimitTrusted),
		"--write", outputFile,
	}

	// Start the scan in a screen session
	screenName := fmt.Sprintf("dns_%s", strings.ReplaceAll(baseDomain, ".", "_"))
	screenCmd := exec.Command("screen", "-S", screenName, "-d", "-m", cfg.PurednsPath)
	screenCmd.Args = append(screenCmd.Args, args...)

	if err := screenCmd.Run(); err != nil {
		st.DecrementActivePurednsScans(false)
		return "", fmt.Errorf("failed to start puredns in screen: %w", err)
	}

	logger.Info("started puredns scan", "domain", baseDomain, "screen", screenName, "output", outputFile)

	// Read output file asynchronously and send to Discord when complete
	go asyncReadAndSendScanResults(target, baseDomain, outputFile, "puredns", cfg.ScanTimeout)

	return outputFile, nil
}

// asyncReadAndSendScanResults monitors a scan output file and sends results to Discord
func asyncReadAndSendScanResults(target, domain, outputFile string, scanType string, timeoutSeconds int) {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 1 * time.Hour // Default 1 hour timeout
	}

	st := GetStatsTracker()
	success := false
	defer func() {
		// Track scan completion based on type
		if scanType == "puredns" {
			st.DecrementActivePurednsScans(success)
		} else if scanType == "feroxbuster" {
			st.DecrementActiveFeroxScans(success)
		}
	}()

	deadline := time.Now().Add(timeout)
	var lastSize int64

	for {
		if time.Now().After(deadline) {
			logger.Warn("scan timeout reached", "domain", domain, "type", scanType, "file", outputFile)
			sendScanResultsToDiscord(target, domain, outputFile, scanType, true)
			success = false
			return
		}

		fileInfo, err := os.Stat(outputFile)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		// Check if file size hasn't changed for 30 seconds (scan likely complete)
		if fileInfo.Size() == lastSize && lastSize > 0 {
			time.Sleep(30 * time.Second)
			fileInfo2, _ := os.Stat(outputFile)
			if fileInfo2.Size() == lastSize {
				logger.Info("scan completed", "domain", domain, "type", scanType, "file", outputFile)
				sendScanResultsToDiscord(target, domain, outputFile, scanType, false)
				success = true
				return
			}
		}

		lastSize = fileInfo.Size()
		time.Sleep(10 * time.Second)
	}
}

// sendScanResultsToDiscord reads the scan output and sends it to Discord
func sendScanResultsToDiscord(target, domain, outputFile, scanType string, timedOut bool) {
	file, err := os.Open(outputFile)
	if err != nil {
		logger.Error("failed to open scan output file", "file", outputFile, "error", err)
		return
	}
	defer file.Close()

	   var results []string
	   scanner := bufio.NewScanner(file)
	   for scanner.Scan() {
		   line := strings.TrimSpace(scanner.Text())
		   if line != "" {
			   results = append(results, line)
		   }
	   }

	   if len(results) == 0 && !timedOut {
		   logger.Info("no results from scan", "domain", domain, "type", scanType)
		   return
	   }

	   // --- HTTP Metadata Extraction ---
	   // Try to extract status code, response size, line count, word count from results
	   statusCode := 0
	   responseSize := 0
	   lineCount := len(results)
	   wordCount := 0
	   for _, line := range results {
		   wordCount += len(strings.Fields(line))
		   // Try to extract status code from line (e.g., "200 OK /path")
		   if statusCode == 0 {
			   fields := strings.Fields(line)
			   if len(fields) > 0 {
				   if code, err := strconv.Atoi(fields[0]); err == nil && code >= 100 && code < 600 {
					   statusCode = code
				   }
			   }
		   }
		   responseSize += len(line)
	   }
	   GetDomainTracker().RecordDomainMetadata(domain, statusCode, responseSize, lineCount, wordCount)

	   status := "Completed"
	   if timedOut {
		   status = "Timeout (results so far)"
	   }

	   sendScanResultsMessage(target, domain, scanType, status, results)
}

// sendScanResultsMessage sends the scan results as a Discord message
func sendScanResultsMessage(target, domain, scanType, status string, results []string) {
	if !notifyDiscord || webhookURL == "" {
		return
	}

	// Chunk results if they're too large
	if len(results) == 0 {
		results = []string{"No results found"}
	}

	chunks := chunkResults(results, 30) // Split into chunks of 30 results

	for i, chunk := range chunks {
		chunkInfo := ""
		if len(chunks) > 1 {
			chunkInfo = fmt.Sprintf(" (Part %d/%d)", i+1, len(chunks))
		}

		// Route to appropriate webhook based on scan type
		if scanType == "feroxbuster" {
			if err := SendDirectoryScanResults(domain, chunk); err != nil {
				logger.Debug("failed to send directory scan to webhook", "domain", domain, "error", err)
				// Fall back to main Discord webhook
				payload := buildScanResultsPayload(target, domain, scanType, status+chunkInfo, chunk)
				sendDiscordPayload(payload)
			}
		} else if scanType == "puredns" {
			if err := SendSubdomainScanResults(domain, chunk); err != nil {
				logger.Debug("failed to send subdomain scan to webhook", "domain", domain, "error", err)
				// Fall back to main Discord webhook
				payload := buildScanResultsPayload(target, domain, scanType, status+chunkInfo, chunk)
				sendDiscordPayload(payload)
			}
		} else {
			payload := buildScanResultsPayload(target, domain, scanType, status+chunkInfo, chunk)
			sendDiscordPayload(payload)
		}

		time.Sleep(500 * time.Millisecond) // Rate limit Discord sends
	}
}

// chunkResults splits results into smaller chunks
func chunkResults(results []string, chunkSize int) [][]string {
	if len(results) == 0 {
		return nil
	}

	var chunks [][]string
	for i := 0; i < len(results); i += chunkSize {
		end := i + chunkSize
		if end > len(results) {
			end = len(results)
		}
		chunks = append(chunks, results[i:end])
	}
	return chunks
}

// buildScanResultsPayload builds a Discord embed for scan results
func buildScanResultsPayload(target, domain, scanType, status string, results []string) map[string]interface{} {
	resultList := strings.Join(results, "\n")
	if len(resultList) > 4000 {
		resultList = resultList[:4000] + "\n... (truncated)"
	}

	title := fmt.Sprintf("%s Scan: %s", strings.ToUpper(scanType), domain)
	description := fmt.Sprintf("```\n%s\n```", resultList)
	color := 3447003 // Blue

	if status == "Timeout (results so far)" {
		color = 16776960 // Yellow
	}

	return map[string]interface{}{
		"tts": false,
		"embeds": []map[string]interface{}{
			{
				"title":       title,
				"description": description,
				"color":       color,
				"footer": map[string]string{
					"text": status,
				},
				"timestamp": time.Now().Format(time.RFC3339),
			},
		},
	}
}

// sendDiscordPayload sends a payload to Discord webhook
func sendDiscordPayload(payload map[string]interface{}) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Error("failed to marshal payload", "error", err)
		return err
	}

	for attempt := 0; attempt < 3; attempt++ {
		resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			logger.Error("failed to send discord payload", "attempt", attempt+1, "error", err)
			time.Sleep(time.Second * time.Duration(attempt+1))
			continue
		}

		switch resp.StatusCode {
		case http.StatusOK, http.StatusNoContent:
			resp.Body.Close()
			return nil
		case http.StatusTooManyRequests:
			resp.Body.Close()
			time.Sleep(2 * time.Second)
		default:
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			logger.Error("discord returned error", "status", resp.StatusCode, "body", string(bodyBytes))
			return fmt.Errorf("discord returned status %d", resp.StatusCode)
		}
	}

	return fmt.Errorf("failed to send after retries")
}
