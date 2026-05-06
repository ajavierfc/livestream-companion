package geo

import (
	"log"
	"os/exec"
	"strings"
	"sync"
)

var (
	rangeCache = make(map[string]bool)
	rangeMu    sync.RWMutex
)

// getIPRange extrae los primeros 3 octetos de una IPv4
func getIPRange(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) < 3 {
		return ip
	}
	return strings.Join(parts[:3], ".")
}

// IsSpanishIP chequea si una IP es de España con caché por rango /24
func IsSpanishIP(ipStr string) bool {
	ipRange := getIPRange(ipStr)

	// 1. Check caché
	rangeMu.RLock()
	isES, exists := rangeCache[ipRange]
	rangeMu.RUnlock()

	if exists {
		return isES
	}

	// 2. Si no está en caché, ejecutar comando de sistema
	// Usamos geoiplookup (requiere apt install geoip-bin)
	out, err := exec.Command("geoiplookup", ipStr).Output()
	
	currentIsES := false
	if err == nil {
		output := string(out)
		// El comando suele devolver "GeoIP Country Edition: ES, Spain"
		if strings.Contains(output, " ES,") {
			currentIsES = true
		}
	} else {
		log.Printf("GeoIP Error: %v. Is geoip-bin installed? (apt install geoip-bin geoip-database)", err)
		return true // Por seguridad, si falla el comando, notificamos
	}

	// 3. Guardar en caché
	rangeMu.Lock()
	rangeCache[ipRange] = currentIsES
	rangeMu.Unlock()

	log.Printf("GeoIP Lookup: IP %s -> Range %s.0/24 -> Spain: %v", ipStr, ipRange, currentIsES)
	return currentIsES
}