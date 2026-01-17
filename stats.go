package main

import (
	"sync"
	"time"
)

// StatsTracker tracks various metrics for monitoring
type StatsTracker struct {
	mu                    sync.RWMutex
	activeCTLogs          int
	disconnectedCTLogs    int
	activeFeroxScans      int
	activePurednsScans    int
	completedScans        int
	failedScans           int
	discoveryTimeline     []DiscoveryPoint
	targetActivity        map[string]int
}

// DiscoveryPoint represents domain discoveries in a time window
type DiscoveryPoint struct {
	Timestamp time.Time
	Count     int
}

var statsTracker *StatsTracker

func init() {
	statsTracker = &StatsTracker{
		targetActivity:    make(map[string]int),
		discoveryTimeline: make([]DiscoveryPoint, 0),
	}
	
	// Start hourly aggregation
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			statsTracker.aggregateDiscoveryRate()
		}
	}()
}

// GetStatsTracker returns the global stats tracker
func GetStatsTracker() *StatsTracker {
	return statsTracker
}

// RecordCTLogStatus updates CT log health stats
func (st *StatsTracker) RecordCTLogStatus(active, disconnected int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.activeCTLogs = active
	st.disconnectedCTLogs = disconnected
}

// IncrementActiveFeroxScans increments feroxbuster scan counter
func (st *StatsTracker) IncrementActiveFeroxScans() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.activeFeroxScans++
}

// DecrementActiveFeroxScans decrements feroxbuster scan counter
func (st *StatsTracker) DecrementActiveFeroxScans(success bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.activeFeroxScans--
	if success {
		st.completedScans++
	} else {
		st.failedScans++
	}
}

// IncrementActivePurednsScans increments puredns scan counter
func (st *StatsTracker) IncrementActivePurednsScans() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.activePurednsScans++
}

// DecrementActivePurednsScans decrements puredns scan counter
func (st *StatsTracker) DecrementActivePurednsScans(success bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.activePurednsScans--
	if success {
		st.completedScans++
	} else {
		st.failedScans++
	}
}

// RecordDiscovery records a domain discovery for rate tracking
func (st *StatsTracker) RecordDiscovery(target string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.targetActivity[target]++
}

// aggregateDiscoveryRate aggregates hourly discovery data
func (st *StatsTracker) aggregateDiscoveryRate() {
	dt := GetDomainTracker()
	allDomains := dt.GetAllDomains()
	
	now := time.Now()
	hourAgo := now.Add(-1 * time.Hour)
	count := 0
	
	for _, entry := range allDomains {
		if entry.FirstSeen.After(hourAgo) && entry.FirstSeen.Before(now) {
			count++
		}
	}
	
	st.mu.Lock()
	defer st.mu.Unlock()
	
	st.discoveryTimeline = append(st.discoveryTimeline, DiscoveryPoint{
		Timestamp: now,
		Count:     count,
	})
	
	// Keep only last 24 hours
	cutoff := now.Add(-24 * time.Hour)
	filtered := []DiscoveryPoint{}
	for _, point := range st.discoveryTimeline {
		if point.Timestamp.After(cutoff) {
			filtered = append(filtered, point)
		}
	}
	st.discoveryTimeline = filtered
}

// GetDiscoveryRate returns domains per hour over last 24h
func (st *StatsTracker) GetDiscoveryRate() []DiscoveryPoint {
	st.mu.RLock()
	defer st.mu.RUnlock()
	
	// If we don't have enough data, calculate current hour rate
	if len(st.discoveryTimeline) == 0 {
		dt := GetDomainTracker()
		allDomains := dt.GetAllDomains()
		hourAgo := time.Now().Add(-1 * time.Hour)
		count := 0
		for _, entry := range allDomains {
			if entry.FirstSeen.After(hourAgo) {
				count++
			}
		}
		return []DiscoveryPoint{{Timestamp: time.Now(), Count: count}}
	}
	
	result := make([]DiscoveryPoint, len(st.discoveryTimeline))
	copy(result, st.discoveryTimeline)
	return result
}

// GetTopTargets returns targets sorted by subdomain count
func (st *StatsTracker) GetTopTargets() map[string]int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	
	result := make(map[string]int)
	for k, v := range st.targetActivity {
		result[k] = v
	}
	return result
}

// GetCTLogHealth returns CT log connection stats
func (st *StatsTracker) GetCTLogHealth() (active, disconnected int) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.activeCTLogs, st.disconnectedCTLogs
}

// GetScanQueue returns active scan counts
func (st *StatsTracker) GetScanQueue() (ferox, puredns int) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.activeFeroxScans, st.activePurednsScans
}

// GetEnumSuccessRate returns scan completion stats
func (st *StatsTracker) GetEnumSuccessRate() (completed, failed int, rate float64) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	total := st.completedScans + st.failedScans
	if total == 0 {
		return 0, 0, 0.0
	}
	rate = float64(st.completedScans) / float64(total) * 100
	return st.completedScans, st.failedScans, rate
}
