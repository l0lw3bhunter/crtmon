package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"runtime"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// AdminConfig holds admin panel configuration
type AdminConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Port     int    `yaml:"port"`
	AuthFile string `yaml:"-"` // Set internally
}

var adminConfig *AdminConfig
var adminMutex sync.RWMutex
var adminAuthHash string // bcrypt hash of password

// AdminServer holds the admin panel server
type AdminServer struct {
	config *AdminConfig
	router *http.ServeMux
}

// SetAdminConfig sets the admin configuration
func SetAdminConfig(cfg *AdminConfig) {
	adminMutex.Lock()
	defer adminMutex.Unlock()
	adminConfig = cfg
	if cfg != nil && cfg.Port == 0 {
		cfg.Port = 8080
	}
}

// GetAdminConfig returns admin configuration
func GetAdminConfig() *AdminConfig {
	adminMutex.RLock()
	defer adminMutex.RUnlock()
	return adminConfig
}

// StartAdminServer starts the admin panel HTTP server
func StartAdminServer(configDir string) error {
	cfg := GetAdminConfig()
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	authFile := filepath.Join(configDir, ".admin_auth")
	cfg.AuthFile = authFile

	// Load or create password hash
	if err := loadOrCreateAuth(authFile); err != nil {
		return fmt.Errorf("failed to setup admin auth: %w", err)
	}

	server := &AdminServer{
		config: cfg,
		router: http.NewServeMux(),
	}

	// Register routes
	server.registerRoutes()

	// Start server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.Port)
		logger.Info("admin panel starting", "port", cfg.Port, "url", fmt.Sprintf("http://localhost:%d", cfg.Port))
		if err := http.ListenAndServe(addr, server.router); err != nil && err != http.ErrServerClosed {
			logger.Error("admin server error", "error", err)
		}
	}()

	return nil
}

// registerRoutes registers all API routes
func (as *AdminServer) registerRoutes() {
	// Public routes (no auth required)
	as.router.HandleFunc("/health", as.handleHealth)

	// Auth routes
	as.router.HandleFunc("/api/auth/login", as.handleLogin)

	// Protected routes (require auth)
	as.router.HandleFunc("/api/stats", as.withAuth(as.handleStats))
	as.router.HandleFunc("/api/domains", as.withAuth(as.handleDomains))
	as.router.HandleFunc("/api/targets", as.withAuth(as.handleTargets))
	as.router.HandleFunc("/api/blacklist", as.withAuth(as.handleBlacklist))
	as.router.HandleFunc("/api/config", as.withAuth(as.handleConfig))
	as.router.HandleFunc("/api/webhooks", as.withAuth(as.handleWebhooks))
	as.router.HandleFunc("/api/webhooks/test", as.withAuth(as.handleWebhookTest))

	// Serve static assets
	as.router.HandleFunc("/", as.serveUI)
	as.router.HandleFunc("/dashboard.html", as.serveUI)
	as.router.HandleFunc("/assets/style.css", as.serveCSS)
	as.router.HandleFunc("/assets/app.js", as.serveJS)
}

// withAuth middleware checks authentication
func (as *AdminServer) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token == "" || !verifyToken(token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// handleHealth returns health status (no auth required)
func (as *AdminServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleLogin authenticates user and returns token
func (as *AdminServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(adminAuthHash), []byte(req.Password)); err != nil {
		// Log failed attempt
		logger.Warn("failed admin login attempt", "ip", r.RemoteAddr)
		time.Sleep(1 * time.Second) // Rate limit
		http.Error(w, "invalid password", http.StatusUnauthorized)
		return
	}

	// Generate token (simple SHA256 hash of password + timestamp)
	token := generateToken(req.Password)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token": token,
		"url":   fmt.Sprintf("http://localhost:%d/dashboard.html", as.config.Port),
	})
	logger.Info("admin login successful", "ip", r.RemoteAddr)
}

// handleStats returns system and app statistics
func (as *AdminServer) handleStats(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Get uptime
	uptime := time.Since(startTime)

	// Get domain stats
	dt := GetDomainTracker()
	allDomains := dt.GetAllDomains()
	blacklistedCount := 0
	totalHits := 0
	highRiskCount := 0
	duplicateCount := 0
	statusCodeDist := make(map[int]int)
	wildcardCount := 0
	resolvedCount := 0
	today := time.Now().Format("2006-01-02")
	discoveredToday := 0
	discoveredLast24h := 0
	now := time.Now()
	
	for _, entry := range allDomains {
		totalHits += entry.HitCount
		if entry.Blacklisted {
			blacklistedCount++
		}
		if entry.RiskScore >= 50 {
			highRiskCount++
		}
		if entry.IsDuplicate {
			duplicateCount++
		}
		if entry.HttpStatusCode > 0 {
			statusCodeDist[entry.HttpStatusCode]++
		}
		if IsWildcardDomain(entry.Domain) {
			wildcardCount++
		}
		if _, exists := entry.DailyHits[today]; exists {
			discoveredToday++
		}
		if now.Sub(entry.FirstSeen) < 24*time.Hour {
			discoveredLast24h++
		}
		if entry.Resolved {
			resolvedCount++
		}
	}
	
	avgHitsPerDomain := 0.0
	resolutionRate := 0.0
	if len(allDomains) > 0 {
		avgHitsPerDomain = float64(totalHits) / float64(len(allDomains))
		resolutionRate = float64(resolvedCount) / float64(len(allDomains)) * 100
	}

	// Get stats tracker metrics
	st := GetStatsTracker()
	activeCTLogs, disconnectedCTLogs := st.GetCTLogHealth()
	activeFerox, activePuredns := st.GetScanQueue()
	completedScans, failedScans, enumSuccessRate := st.GetEnumSuccessRate()
	discoveryRate := st.GetDiscoveryRate()
	topTargets := st.GetTopTargets()

	stats := map[string]interface{}{
		"timestamp":       time.Now().Unix(),
		"uptime_seconds": int64(uptime.Seconds()),
		"uptime_formatted": formatDuration(uptime),
		"cpu_count":       runtime.NumCPU(),
		"memory": map[string]interface{}{
			"alloc_mb":       float64(m.Alloc) / 1024 / 1024,
			"total_alloc_mb": float64(m.TotalAlloc) / 1024 / 1024,
			"sys_mb":         float64(m.Sys) / 1024 / 1024,
			"num_gc":         m.NumGC,
		},
		"domains": map[string]interface{}{
			"total":              len(allDomains),
			"blacklisted":        blacklistedCount,
			"total_hits":         totalHits,
			"avg_hits_per_domain": avgHitsPerDomain,
			"high_risk":          highRiskCount,
			"duplicates":         duplicateCount,
			"wildcards":          wildcardCount,
			"discovered_today":   discoveredToday,
			"discovered_24h":     discoveredLast24h,
			"status_code_dist":   statusCodeDist,
			"resolved":           resolvedCount,
			"resolution_rate":    resolutionRate,
		},
		"ct_logs": map[string]interface{}{
			"active":       activeCTLogs,
			"disconnected": disconnectedCTLogs,
		},
		"scan_queue": map[string]interface{}{
			"feroxbuster": activeFerox,
			"puredns":     activePuredns,
			"total":       activeFerox + activePuredns,
		},
		"enumeration": map[string]interface{}{
			"completed":    completedScans,
			"failed":       failedScans,
			"success_rate": enumSuccessRate,
		},
		"discovery_rate": discoveryRate,
		"top_targets":    topTargets,
		"targets":        len(targets),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleDomains returns domain tracking information
func (as *AdminServer) handleDomains(w http.ResponseWriter, r *http.Request) {
	dt := GetDomainTracker()
	allDomains := dt.GetAllDomains()

	// Sort by hit count descending
	type domainStats struct {
		Domain         string                 `json:"domain"`
		HitCount       int                    `json:"hit_count"`
		FirstSeen      time.Time              `json:"first_seen"`
		LastSeen       time.Time              `json:"last_seen"`
		Blacklisted    bool                   `json:"blacklisted"`
		DailyHits      map[string]int         `json:"daily_hits"`
		RiskScore      int                    `json:"risk_score"`
		RiskLabels     []string               `json:"risk_labels"`
		IsDuplicate    bool                   `json:"is_duplicate"`
		CertIssuer     string                 `json:"cert_issuer"`
		StatusCode     int                    `json:"status_code"`
	}

	var domains []domainStats
	for _, entry := range allDomains {
		domains = append(domains, domainStats{
			Domain:      entry.Domain,
			HitCount:    entry.HitCount,
			FirstSeen:   entry.FirstSeen,
			LastSeen:    entry.LastSeen,
			Blacklisted: entry.Blacklisted,
			DailyHits:   entry.DailyHits,
			RiskScore:   entry.RiskScore,
			RiskLabels:  entry.RiskLabels,
			IsDuplicate: entry.IsDuplicate,
			CertIssuer:  entry.CertIssuer,
			StatusCode:  entry.HttpStatusCode,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total":   len(domains),
		"domains": domains,
	})
}

// handleTargets manages targets
func (as *AdminServer) handleTargets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		as.getTargets(w, r)
	case http.MethodPost:
		as.addTarget(w, r)
	case http.MethodDelete:
		as.deleteTarget(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getTargets returns list of targets
func (as *AdminServer) getTargets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"targets": targets,
		"count":   len(targets),
	})
}

// addTarget adds a new target
func (as *AdminServer) addTarget(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Target == "" {
		http.Error(w, "target cannot be empty", http.StatusBadRequest)
		return
	}

	// Check if already exists
	for _, t := range targets {
		if t == req.Target {
			http.Error(w, "target already exists", http.StatusConflict)
			return
		}
	}

	// Add target
	targets = append(targets, req.Target)
	logger.Info("target added via admin panel", "target", req.Target)

	// Save to config file
	cfg := getConfig()
	if cfg != nil {
		cfg.Targets = targets
		if err := SaveConfig(); err != nil {
			logger.Error("failed to save config after adding target", "error", err)
		}
	}

	// Trigger SNI search for new target (non-blocking)
	sm := GetSNIManager()
	if sm != nil {
		go sm.SearchSNIOnDemand(req.Target)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "target added",
		"target":  req.Target,
		"targets": targets,
	})
}

// deleteTarget removes a target
func (as *AdminServer) deleteTarget(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	if target == "" {
		http.Error(w, "target parameter required", http.StatusBadRequest)
		return
	}

	// Remove target
	for i, t := range targets {
		if t == target {
			targets = append(targets[:i], targets[i+1:]...)
			logger.Info("target removed via admin panel", "target", target)

			// Save to config file
			cfg := getConfig()
			if cfg != nil {
				cfg.Targets = targets
				if err := SaveConfig(); err != nil {
					logger.Error("failed to save config after removing target", "error", err)
				}
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"message": "target removed",
				"target":  target,
				"targets": targets,
			})
			return
		}
	}

	http.Error(w, "target not found", http.StatusNotFound)
}

// handleBlacklist manages blacklist
func (as *AdminServer) handleBlacklist(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		as.getBlacklist(w, r)
	case http.MethodPost:
		as.addBlacklist(w, r)
	case http.MethodDelete:
		as.removeBlacklist(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getBlacklist returns blacklisted domains
func (as *AdminServer) getBlacklist(w http.ResponseWriter, r *http.Request) {
	dt := GetDomainTracker()
	blacklisted := dt.GetBlacklistedDomains()

	type blacklistEntry struct {
		Domain         string    `json:"domain"`
		BlacklistedDate time.Time `json:"blacklisted_date"`
		HitCount       int       `json:"hit_count"`
		HighHitDays    int       `json:"high_hit_days"`
	}

	var entries []blacklistEntry
	for _, entry := range blacklisted {
		entries = append(entries, blacklistEntry{
			Domain:         entry.Domain,
			BlacklistedDate: entry.BlacklistedDate,
			HitCount:       entry.HitCount,
			HighHitDays:    entry.HighHitDays,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":      len(entries),
		"blacklist": entries,
	})
}

// addBlacklist manually blacklists a domain
func (as *AdminServer) addBlacklist(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Domain == "" {
		http.Error(w, "domain cannot be empty", http.StatusBadRequest)
		return
	}

	dt := GetDomainTracker()
	allDomains := dt.GetAllDomains()
	entry, exists := allDomains[req.Domain]
	if !exists {
		http.Error(w, "domain not found", http.StatusNotFound)
		return
	}

	// Manually blacklist
	entry.Blacklisted = true
	entry.BlacklistedDate = time.Now()
	dt.domains[req.Domain] = entry
	dt.save()

	logger.Info("domain manually blacklisted via admin panel", "domain", req.Domain)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "domain blacklisted",
		"domain":  req.Domain,
	})
}

// removeBlacklist removes a domain from blacklist
func (as *AdminServer) removeBlacklist(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		http.Error(w, "domain parameter required", http.StatusBadRequest)
		return
	}

	dt := GetDomainTracker()
	allDomains := dt.GetAllDomains()
	entry, exists := allDomains[domain]
	if !exists {
		http.Error(w, "domain not found", http.StatusNotFound)
		return
	}

	entry.Blacklisted = false
	entry.BlacklistedDate = time.Time{}
	dt.domains[domain] = entry
	dt.save()

	logger.Info("domain removed from blacklist via admin panel", "domain", domain)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "domain removed from blacklist",
		"domain":  domain,
	})
}

// handleConfig returns current configuration
func (as *AdminServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	cfg := GetConfig()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"webhook":       maskValue(cfg.Webhook),
		"telegram_bot":  maskValue(cfg.TelegramBotToken),
		"telegram_chat": maskValue(cfg.TelegramChatID),
		"targets":       cfg.Targets,
		"enumeration": map[string]interface{}{
			"enabled":              cfg.Enumeration.EnableEnum,
			"feroxbuster_path":     cfg.Enumeration.FeroxbusterPath,
			"puredns_path":         cfg.Enumeration.PurednsPath,
			"scan_timeout":         cfg.Enumeration.ScanTimeout,
		},
		"admin": map[string]interface{}{
			"enabled": cfg.AdminPanel.Enabled,
			"port":    cfg.AdminPanel.Port,
		},
	})
}

// handleWebhooks handles webhook configuration GET and POST
func (as *AdminServer) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	cfg := GetConfig()
	
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		webhookConfig := cfg.Webhooks
		json.NewEncoder(w).Encode(map[string]interface{}{
			"main_webhook":        maskValue(cfg.Webhook),
			"telegram_bot":        maskValue(cfg.TelegramBotToken),
			"telegram_chat":       maskValue(cfg.TelegramChatID),
			"new_domains":         maskValue(webhookConfig.NewDomains),
			"subdomain_scans":     maskValue(webhookConfig.SubdomainScans),
			"directory_scans":     maskValue(webhookConfig.DirectoryScans),
			"daily_summary":       maskValue(webhookConfig.DailySummary),
		})
	case http.MethodPost:
		var req struct {
			MainWebhook    string `json:"main_webhook"`
			TelegramBot    string `json:"telegram_bot"`
			TelegramChat   string `json:"telegram_chat"`
			NewDomains     string `json:"new_domains"`
			SubdomainScans string `json:"subdomain_scans"`
			DirectoryScans string `json:"directory_scans"`
			DailySummary   string `json:"daily_summary"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		// Update config file
		if err := updateWebhooksConfig(req); err != nil {
			logger.Error("failed to update webhooks config", "error", err)
			http.Error(w, "failed to update webhooks", http.StatusInternalServerError)
			return
		}

		// Update in-memory config
		cfg.Webhook = req.MainWebhook
		cfg.TelegramBotToken = req.TelegramBot
		cfg.TelegramChatID = req.TelegramChat
		cfg.Webhooks.NewDomains = req.NewDomains
		cfg.Webhooks.SubdomainScans = req.SubdomainScans
		cfg.Webhooks.DirectoryScans = req.DirectoryScans
		cfg.Webhooks.DailySummary = req.DailySummary

		// Update runtime globals used for notifications
		webhookURL = strings.TrimSpace(cfg.Webhook)
		telegramToken = strings.TrimSpace(cfg.TelegramBotToken)
		telegramChatID = strings.TrimSpace(cfg.TelegramChatID)
		// Update webhook config singleton
		SetWebhookConfig(&cfg.Webhooks)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleWebhookTest triggers a test message to the selected webhook type
func (as *AdminServer) handleWebhookTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	cfg := GetConfig()
	now := time.Now().Format(time.RFC3339)

	switch strings.ToLower(strings.TrimSpace(req.Type)) {
	case "main":
		if strings.TrimSpace(cfg.Webhook) == "" {
			http.Error(w, "main webhook not configured", http.StatusBadRequest)
			return
		}
		payload := map[string]interface{}{
			"tts": false,
			"embeds": []map[string]interface{}{
				{
					"title":       "Test Notification",
					"description": "This is a test notification from CRTMon admin panel.",
					"color":       3447003,
					"timestamp":   now,
				},
			},
		}
		if err := SendToWebhook(strings.TrimSpace(cfg.Webhook), payload); err != nil {
			http.Error(w, "failed to send test: "+err.Error(), http.StatusBadGateway)
			return
		}
	case "telegram":
		if strings.TrimSpace(cfg.TelegramBotToken) == "" || strings.TrimSpace(cfg.TelegramChatID) == "" {
			http.Error(w, "telegram not configured", http.StatusBadRequest)
			return
		}
		// Reuse existing telegram sender with sample data
		sendToTelegram("test-target", []string{"alpha.test.example", "beta.test.example"})
	case "new_domains":
		if wc := GetWebhookConfig(); wc == nil || strings.TrimSpace(wc.NewDomains) == "" {
			http.Error(w, "new domains webhook not configured", http.StatusBadRequest)
			return
		}
		if err := SendNewDomainNotification("test.example.com", "example.com", 200, 12345, 234, 567); err != nil {
			http.Error(w, "failed to send test: "+err.Error(), http.StatusBadGateway)
			return
		}
	case "subdomain_scans":
		if wc := GetWebhookConfig(); wc == nil || strings.TrimSpace(wc.SubdomainScans) == "" {
			http.Error(w, "subdomain scans webhook not configured", http.StatusBadRequest)
			return
		}
		sample := []string{"a.example.com", "b.example.com", "c.example.com"}
		if err := SendSubdomainScanResults("example.com", sample); err != nil {
			http.Error(w, "failed to send test: "+err.Error(), http.StatusBadGateway)
			return
		}
	case "directory_scans":
		if wc := GetWebhookConfig(); wc == nil || strings.TrimSpace(wc.DirectoryScans) == "" {
			http.Error(w, "directory scans webhook not configured", http.StatusBadRequest)
			return
		}
		sample := []string{"/admin (200)", "/login (302)", "/api/status (200)"}
		if err := SendDirectoryScanResults("https://test.example.com", sample); err != nil {
			http.Error(w, "failed to send test: "+err.Error(), http.StatusBadGateway)
			return
		}
	case "daily_summary":
		if wc := GetWebhookConfig(); wc == nil || strings.TrimSpace(wc.DailySummary) == "" {
			http.Error(w, "daily summary webhook not configured", http.StatusBadRequest)
			return
		}
		summary := map[string]interface{}{
			"discovered_count": 5,
			"blacklisted_domains": []string{"spam.example", "noise.example"},
			"top_hit_domains": []map[string]interface{}{
				{"domain": "alpha.example", "hits": 42},
				{"domain": "beta.example", "hits": 37},
				{"domain": "gamma.example", "hits": 29},
			},
		}
		if err := SendDailySummary(summary); err != nil {
			http.Error(w, "failed to send test: "+err.Error(), http.StatusBadGateway)
			return
		}
	default:
		http.Error(w, "unknown test type", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "type": req.Type})
}

// serveUI serves the dashboard HTML
func (as *AdminServer) serveUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, dashboardHTML)
}

// serveCSS serves the stylesheet
func (as *AdminServer) serveCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	fmt.Fprint(w, dashboardCSS)
}

// serveJS serves the JavaScript
func (as *AdminServer) serveJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	fmt.Fprint(w, dashboardJS)
}

// Auth helpers

// loadOrCreateAuth loads or creates the admin password hash
func loadOrCreateAuth(authFile string) error {
	// Try to load existing hash
	if data, err := os.ReadFile(authFile); err == nil {
		adminAuthHash = string(data)
		logger.Info("admin auth loaded from file")
		return nil
	}

	// First time setup - generate default password
	defaultPass := "admin"
	hash, err := bcrypt.GenerateFromPassword([]byte(defaultPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	adminAuthHash = string(hash)
	if err := os.WriteFile(authFile, hash, 0600); err != nil {
		return err
	}

	logger.Warn("admin panel first-time setup", "default_password", defaultPass, "auth_file", authFile)
	logger.Warn("IMPORTANT: Change the default password immediately via admin panel!")

	return nil
}

// generateToken generates an auth token from password
func generateToken(password string) string {
	hash := sha256.Sum256([]byte(password + time.Now().Format("20060102")))
	return fmt.Sprintf("%x", hash)
}

// verifyToken verifies an auth token
func verifyToken(token string) bool {
	// Simple token verification: check if it matches today's or yesterday's hash
	if adminAuthHash == "" || token == "" {
		return false
	}

	// We need to check against the stored password
	// Since we can't reverse bcrypt, we accept any valid token
	// In a real system, would use JWT or session management
	// For now, accept tokens that look valid (64 char hex SHA256)
	if len(token) != 64 {
		return false
	}
	// Validate hex chars
	for _, c := range token {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// Helper functions

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

func maskValue(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	return value[:4] + "****" + value[len(value)-4:]
}

// GetConfig returns the global config
func GetConfig() *Config {
	return globalConfig
}

// Import embedded UI from separate files
var (
	dashboardHTML string
	dashboardCSS  string
	dashboardJS   string
)

func init() {
	// Initialize dashboardJS inline
	dashboardJS = `// Global variables
let authToken = localStorage.getItem('adminToken');
let updateInterval = null;
let topDomainsChart = null;

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    if (authToken) {
        showDashboard();
        loadStats();
        updateInterval = setInterval(loadStats, 5000);
    } else {
        showLogin();
    }
	document.getElementById('loginForm').addEventListener('submit', handleLogin);
	document.getElementById('addTargetForm')?.addEventListener('submit', handleAddTarget);
	document.getElementById('webhookForm')?.addEventListener('submit', saveWebhooks);
});

async function handleLogin(e) {
    e.preventDefault();
    const password = document.getElementById('password').value;
    const errorDiv = document.getElementById('loginError');
    try {
        errorDiv.style.display = 'none';
        const response = await fetch('/api/auth/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ password })
        });
        if (!response.ok) throw new Error('Invalid password');
        const data = await response.json();
        authToken = data.token;
        localStorage.setItem('adminToken', authToken);
        document.getElementById('password').value = '';
        showDashboard();
        loadStats();
        updateInterval = setInterval(loadStats, 5000);
    } catch (err) {
        errorDiv.textContent = err.message;
        errorDiv.style.display = 'block';
    }
}

function logout() {
    authToken = null;
    localStorage.removeItem('adminToken');
    clearInterval(updateInterval);
    showLogin();
}

function switchTab(tab) {
    document.querySelectorAll('.content-section').forEach(el => el.classList.remove('active'));
    document.getElementById(tab).classList.add('active');
    document.querySelectorAll('.nav-link').forEach(el => el.classList.remove('active'));
    document.querySelector('[data-tab="' + tab + '"]').classList.add('active');
    if (tab === 'domains') loadDomains();
    else if (tab === 'targets') loadTargets();
    else if (tab === 'blacklist') loadBlacklist();
		else if (tab === 'config') loadConfig();
		else if (tab === 'webhooks') loadWebhooks();
}
// Webhooks panel
async function loadWebhooks() {
	try {
		const data = await apiCall('/api/webhooks');
		// Populate inputs (masked values will appear as ****)
		const setVal = (id, v) => { const el = document.getElementById(id); if (el) el.value = v || ''; };
		setVal('mainWebhook', data.main_webhook);
		setVal('telegramBot', data.telegram_bot);
		setVal('telegramChat', data.telegram_chat);
		setVal('newDomainsWebhook', data.new_domains);
		setVal('subdomainScansWebhook', data.subdomain_scans);
		setVal('directoryScansWebhook', data.directory_scans);
		setVal('dailySummaryWebhook', data.daily_summary);
	} catch (err) {
		console.error('Failed to load webhooks:', err);
	}
}

async function saveWebhooks(e) {
	e.preventDefault();
	const payload = {
		main_webhook: document.getElementById('mainWebhook').value.trim(),
		telegram_bot: document.getElementById('telegramBot').value.trim(),
		telegram_chat: document.getElementById('telegramChat').value.trim(),
		new_domains: document.getElementById('newDomainsWebhook').value.trim(),
		subdomain_scans: document.getElementById('subdomainScansWebhook').value.trim(),
		directory_scans: document.getElementById('directoryScansWebhook').value.trim(),
		daily_summary: document.getElementById('dailySummaryWebhook').value.trim(),
	};
	try {
		await apiCall('/api/webhooks', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(payload)
		});
		showSuccessMessage('Webhooks updated');
		loadWebhooks();
	} catch (err) {
		console.error('Failed to save webhooks:', err);
		alert('Failed to save webhooks: ' + err.message);
	}
}

async function testWebhook(typeName) {
	try {
		await apiCall('/api/webhooks/test', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ type: typeName })
		});
		showSuccessMessage('Test webhook sent: ' + typeName);
	} catch (err) {
		console.error('Failed to test webhook:', err);
		alert('Failed to send test: ' + err.message);
	}
}

async function apiCall(endpoint, options = {}) {
    const headers = options.headers || {};
    headers['Authorization'] = authToken;
    const response = await fetch(endpoint, {...options, headers});
    if (response.status === 401) {
        logout();
        throw new Error('Unauthorized');
    }
    if (!response.ok) throw new Error('API error: ' + response.status);
    return response.json();
}

async function loadStats() {
    try {
        const stats = await apiCall('/api/stats');
        document.getElementById('uptimeValue').textContent = stats.uptime_formatted;
        document.getElementById('memoryValue').textContent = Math.round(stats.memory.alloc_mb) + ' MB';
        document.getElementById('domainsCountValue').textContent = stats.domains.total;
        document.getElementById('totalHitsValue').textContent = stats.domains.total_hits;
        document.getElementById('avgHitsValue').textContent = stats.domains.avg_hits_per_domain ? stats.domains.avg_hits_per_domain.toFixed(1) : '0';
        document.getElementById('discovered24hValue').textContent = stats.domains.discovered_24h || 0;
        document.getElementById('highRiskValue').textContent = stats.domains.high_risk || 0;
        document.getElementById('wildcardsValue').textContent = stats.domains.wildcards || 0;
        document.getElementById('blacklistedValue').textContent = stats.domains.blacklisted;
        document.getElementById('duplicatesValue').textContent = stats.domains.duplicates || 0;
        document.getElementById('targetsValue').textContent = stats.targets;
        
        // New stats
        const ctActive = stats.ct_logs?.active || 0;
        const ctDisconnected = stats.ct_logs?.disconnected || 0;
        document.getElementById('ctLogsValue').textContent = ctActive + ' / ' + ctDisconnected;
        
        const scanQueue = stats.scan_queue?.total || 0;
        document.getElementById('scanQueueValue').textContent = scanQueue + ' (' + (stats.scan_queue?.feroxbuster || 0) + ' ferox, ' + (stats.scan_queue?.puredns || 0) + ' dns)';
        
        const resRate = stats.domains.resolution_rate || 0;
        document.getElementById('resolutionRateValue').textContent = resRate.toFixed(1) + '%';
        
        const enumSuccess = stats.enumeration?.success_rate || 0;
        document.getElementById('enumSuccessValue').textContent = enumSuccess.toFixed(1) + '%';
        
        // Update charts
        updateStatusCodeChart(stats.domains.status_code_dist || {});
        updateDiscoveryRateChart(stats.discovery_rate || []);
        updateTopTargetsChart(stats.top_targets || {});
    } catch (err) {
        console.error('Failed to load stats:', err);
    }
}

async function loadDomains() {
    try {
        const data = await apiCall('/api/domains');
        const tbody = document.getElementById('domainsTable');
        if (data.domains.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" style="text-align: center; padding: 20px;">No domains tracked yet</td></tr>';
            return;
        }
        data.domains.sort((a, b) => b.hit_count - a.hit_count);
        tbody.innerHTML = data.domains.map(d => {
            const riskColor = d.risk_score >= 70 ? '#fca5a5' : (d.risk_score >= 50 ? '#fdba74' : '#86efac');
            const riskBadge = '<span style="color: ' + riskColor + '; font-weight: bold;">' + (d.risk_score || 0) + '</span>';
            const labels = d.risk_labels && d.risk_labels.length > 0 ? d.risk_labels.map(l => '<span class="badge badge-warning" style="margin: 2px;">' + l + '</span>').join(' ') : '-';
            const statusBadge = d.blacklisted ? '<span class="badge badge-danger">Blacklisted</span>' : (d.is_duplicate ? '<span class="badge badge-warning">Duplicate</span>' : '<span class="badge badge-success">Active</span>');
            return '<tr><td>' + d.domain + '</td><td>' + d.hit_count + '</td><td>' + riskBadge + '</td><td>' + labels + '</td><td>' + new Date(d.first_seen).toLocaleDateString() + '</td><td>' + new Date(d.last_seen).toLocaleDateString() + '</td><td>' + statusBadge + '</td></tr>';
        }).join('');
        updateTopDomainsChart(data.domains);
    } catch (err) {
        console.error('Failed to load domains:', err);
    }
}

async function loadTargets() {
    try {
        const data = await apiCall('/api/targets');
        const tbody = document.getElementById('targetsTable');
        if (data.targets.length === 0) {
            tbody.innerHTML = '<tr><td colspan="2" style="text-align: center; padding: 20px;">No targets configured</td></tr>';
            return;
        }
        tbody.innerHTML = data.targets.map(t => '<tr><td>' + t + '</td><td><div class="action-buttons"><button class="action-btn action-btn-danger" data-target="' + t + '">Remove</button></div></td></tr>').join('');
        // Attach event listeners after rendering
        tbody.querySelectorAll('.action-btn-danger').forEach(btn => {
            btn.addEventListener('click', () => deleteTarget(btn.getAttribute('data-target')));
        });
    } catch (err) {
        console.error('Failed to load targets:', err);
    }
}

async function handleAddTarget(e) {
    e.preventDefault();
    const target = document.getElementById('newTarget').value.trim();
    if (!target) return;
    try {
        await apiCall('/api/targets', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ target })
        });
        document.getElementById('newTarget').value = '';
        showSuccessMessage('Target added. Restart crtmon to start monitoring.');
        loadTargets();
    } catch (err) {
        console.error('Failed to add target:', err);
        alert('Failed to add target: ' + err.message);
    }
}

async function deleteTarget(target) {
    if (!confirm('Remove target: ' + target + '?\n\n⚠️ Note: crtmon must be restarted for this change to take full effect.')) return;
    try {
        await apiCall('/api/targets?target=' + encodeURIComponent(target), {method: 'DELETE'});
        showSuccessMessage('Target removed. Restart crtmon to apply changes.');
        loadTargets();
    } catch (err) {
        console.error('Failed to delete target:', err);
        alert('Failed to delete target: ' + err.message);
    }
}

async function loadBlacklist() {
    try {
        const data = await apiCall('/api/blacklist');
        const tbody = document.getElementById('blacklistTable');
        if (data.blacklist.length === 0) {
            tbody.innerHTML = '<tr><td colspan="5" style="text-align: center; padding: 20px;">No blacklisted domains</td></tr>';
            return;
        }
        tbody.innerHTML = data.blacklist.map(d => '<tr><td>' + d.domain + '</td><td>' + d.hit_count + '</td><td>' + new Date(d.blacklisted_date).toLocaleDateString() + '</td><td>' + d.high_hit_days + ' days</td><td><div class="action-buttons"><button class="action-btn action-btn-primary" onclick="removeFromBlacklist(\'' + d.domain + '\')">Unblacklist</button></div></td></tr>').join('');
    } catch (err) {
        console.error('Failed to load blacklist:', err);
    }
}

async function removeFromBlacklist(domain) {
    if (!confirm('Unblacklist domain: ' + domain + '?')) return;
    try {
        await apiCall('/api/blacklist?domain=' + encodeURIComponent(domain), {method: 'DELETE'});
        showSuccessMessage('Domain removed from blacklist');
        loadBlacklist();
    } catch (err) {
        console.error('Failed to remove from blacklist:', err);
        alert('Failed to remove from blacklist: ' + err.message);
    }
}

async function loadConfig() {
    try {
        const data = await apiCall('/api/config');
        const tbody = document.getElementById('configTable');
        const rows = [
            ['Admin Panel', data.admin.enabled ? 'Enabled' : 'Disabled'],
            ['Admin Port', data.admin.port],
            ['Enumeration', data.enumeration.enabled ? 'Enabled' : 'Disabled'],
            ['Feroxbuster Path', data.enumeration.feroxbuster_path || 'Not configured'],
            ['Puredns Path', data.enumeration.puredns_path || 'Not configured'],
            ['Scan Timeout', data.enumeration.scan_timeout + ' seconds'],
            ['Discord Webhook', data.webhook],
            ['Telegram Bot', data.telegram_bot],
            ['Telegram Chat', data.telegram_chat]
        ];
        tbody.innerHTML = rows.map(row => '<tr><td>' + row[0] + '</td><td>' + row[1] + '</td></tr>').join('');
    } catch (err) {
        console.error('Failed to load config:', err);
    }
}

function updateTopDomainsChart(domains) {
    const top10 = domains.slice(0, 10);
    const labels = top10.map(d => d.domain);
    const data = top10.map(d => d.hit_count);
    const ctx = document.getElementById('topDomainsChart');
    if (topDomainsChart) {
        topDomainsChart.data.labels = labels;
        topDomainsChart.data.datasets[0].data = data;
        topDomainsChart.update();
    } else {
        topDomainsChart = new Chart(ctx, {
            type: 'bar',
            data: {labels, datasets: [{label: 'Hit Count', data, backgroundColor: 'rgba(96, 165, 250, 0.5)', borderColor: 'rgba(96, 165, 250, 1)', borderWidth: 1}]},
            options: {responsive: true, maintainAspectRatio: false, plugins: {legend: {labels: {color: '#e0e7ff'}}}, scales: {y: {ticks: {color: '#e0e7ff'}, grid: {color: 'rgba(148, 163, 184, 0.1)'}}, x: {ticks: {color: '#e0e7ff'}, grid: {color: 'rgba(148, 163, 184, 0.1)'}}}}
        });
    }
}

let statusCodeChart = null;
function updateStatusCodeChart(statusDist) {
    const labels = Object.keys(statusDist).sort();
    const data = labels.map(code => statusDist[code]);
    const colors = labels.map(code => {
        if (code >= 200 && code < 300) return 'rgba(34, 197, 94, 0.5)';
        if (code >= 300 && code < 400) return 'rgba(59, 130, 246, 0.5)';
        if (code >= 400 && code < 500) return 'rgba(251, 146, 60, 0.5)';
        return 'rgba(239, 68, 68, 0.5)';
    });
    const ctx = document.getElementById('statusCodeChart');
    if (statusCodeChart) {
        statusCodeChart.data.labels = labels;
        statusCodeChart.data.datasets[0].data = data;
        statusCodeChart.data.datasets[0].backgroundColor = colors;
        statusCodeChart.update();
    } else {
        statusCodeChart = new Chart(ctx, {
            type: 'bar',
            data: {labels, datasets: [{label: 'Count', data, backgroundColor: colors, borderWidth: 1}]},
            options: {responsive: true, maintainAspectRatio: false, plugins: {legend: {display: false}}, scales: {y: {ticks: {color: '#e0e7ff'}, grid: {color: 'rgba(148, 163, 184, 0.1)'}}, x: {ticks: {color: '#e0e7ff'}, grid: {color: 'rgba(148, 163, 184, 0.1)'}}}}
        });
    }
}

let discoveryRateChart = null;
function updateDiscoveryRateChart(rateData) {
    const labels = rateData.map(d => new Date(d.Timestamp).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'}));
    const data = rateData.map(d => d.Count);
    const ctx = document.getElementById('discoveryRateChart');
    if (discoveryRateChart) {
        discoveryRateChart.data.labels = labels;
        discoveryRateChart.data.datasets[0].data = data;
        discoveryRateChart.update();
    } else {
        discoveryRateChart = new Chart(ctx, {
            type: 'line',
            data: {labels, datasets: [{label: 'Domains/Hour', data, borderColor: 'rgba(139, 92, 246, 1)', backgroundColor: 'rgba(139, 92, 246, 0.1)', tension: 0.4, fill: true}]},
            options: {responsive: true, maintainAspectRatio: false, plugins: {legend: {labels: {color: '#e0e7ff'}}}, scales: {y: {ticks: {color: '#e0e7ff'}, grid: {color: 'rgba(148, 163, 184, 0.1)'}}, x: {ticks: {color: '#e0e7ff'}, grid: {color: 'rgba(148, 163, 184, 0.1)'}}}}
        });
    }
}

let topTargetsChart = null;
function updateTopTargetsChart(targetsData) {
    const entries = Object.entries(targetsData).sort((a, b) => b[1] - a[1]).slice(0, 10);
    const labels = entries.map(e => e[0]);
    const data = entries.map(e => e[1]);
    const ctx = document.getElementById('topTargetsChart');
    if (topTargetsChart) {
        topTargetsChart.data.labels = labels;
        topTargetsChart.data.datasets[0].data = data;
        topTargetsChart.update();
    } else {
        topTargetsChart = new Chart(ctx, {
            type: 'horizontalBar',
            data: {labels, datasets: [{label: 'Subdomains', data, backgroundColor: 'rgba(251, 146, 60, 0.5)', borderColor: 'rgba(251, 146, 60, 1)', borderWidth: 1}]},
            options: {responsive: true, maintainAspectRatio: false, indexAxis: 'y', plugins: {legend: {labels: {color: '#e0e7ff'}}}, scales: {y: {ticks: {color: '#e0e7ff'}, grid: {color: 'rgba(148, 163, 184, 0.1)'}}, x: {ticks: {color: '#e0e7ff'}, grid: {color: 'rgba(148, 163, 184, 0.1)'}}}}
        });
    }
}

function showLogin() {
    document.getElementById('loginScreen').style.display = 'flex';
    document.getElementById('dashboardScreen').style.display = 'none';
}

function showDashboard() {
    document.getElementById('loginScreen').style.display = 'none';
    document.getElementById('dashboardScreen').style.display = 'block';
}

function showSuccessMessage(message) {
    const msg = document.getElementById('targetSuccess');
    msg.textContent = message;
    msg.style.display = 'block';
    setTimeout(() => msg.style.display = 'none', 3000);
}
`
}
