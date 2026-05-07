package geo

import (
	"log"
	"os/exec"
	"strings"
	"sync"
)

var ipCache = make(map[string]bool)
var cacheMu sync.RWMutex

// getIPRange extracts the first 3 octets of an IPv4 address
func getIPRange(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) < 3 {
		return ip
	}
	return strings.Join(parts[:3], ".")
}

// IsSpanishIP checks if an IP is from Spain using a /24 range cache
func IsSpanishIP(ipStr string) bool {
	ipRange := getIPRange(ipStr)

	// 1. Check cache
	cacheMu.RLock()
	isES, exists := ipCache[ipRange]
	cacheMu.RUnlock()
	if exists {
		return isES
	}

	// 2. If not in cache, run system command (requirement: apt install geoip-bin geoip-database)
	out, err := exec.Command("geoiplookup", ipStr).Output()
	currentIsES := false
	if err == nil {
		output := string(out)
		// The command usually returns "GeoIP Country Edition: ES, Spain"
		if strings.Contains(output, " ES,") {
			currentIsES = true
		}
	} else {
		log.Printf("GeoIP Error: %v. Is geoip-bin installed? (apt install geoip-bin geoip-database)", err)
		return true // For security, notify if the command fails
	}

	// 3. Store in cache
	cacheMu.Lock()
	ipCache[ipRange] = currentIsES
	cacheMu.Unlock()
	log.Printf("GeoIP Lookup: IP %s -> Range %s.0/24 -> Spain: %v", ipStr, ipRange, currentIsES)
	return currentIsES
}