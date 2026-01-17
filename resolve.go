package main

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"
)

var (
	resolveCache = make(map[string]cacheEntry)
	resolveMutex sync.RWMutex
)

type cacheEntry struct {
	resolves  bool
	timestamp time.Time
}

// ResolveDomain checks if a domain resolves via DNS
func ResolveDomain(domain string) bool {
	// Normalize domain
	d := strings.ToLower(strings.TrimSuffix(domain, "."))

	// Check cache first (valid for 1 hour)
	resolveMutex.RLock()
	if entry, exists := resolveCache[d]; exists {
		if time.Since(entry.timestamp) < time.Hour {
			resolveMutex.RUnlock()
			return entry.resolves
		}
	}
	resolveMutex.RUnlock()

	// Perform DNS lookup with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, network, address)
		},
	}

	_, err := resolver.LookupHost(ctx, d)
	resolves := err == nil

	// Cache result
	resolveMutex.Lock()
	resolveCache[d] = cacheEntry{
		resolves:  resolves,
		timestamp: time.Now(),
	}
	resolveMutex.Unlock()

	return resolves
}

// ClearResolveCache clears the DNS resolution cache
func ClearResolveCache() {
	resolveMutex.Lock()
	defer resolveMutex.Unlock()
	resolveCache = make(map[string]cacheEntry)
}

// ClearResolveCacheEntry clears a single entry from the cache
func ClearResolveCacheEntry(domain string) {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))
	resolveMutex.Lock()
	defer resolveMutex.Unlock()
	delete(resolveCache, d)
}

// GetResolveCacheSize returns the current cache size
func GetResolveCacheSize() int {
	resolveMutex.RLock()
	defer resolveMutex.RUnlock()
	return len(resolveCache)
}
