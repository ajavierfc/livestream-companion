package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

type AppData struct {
    AuthorizedIPs map[string]bool            `json:"authorized_ips"`
    IpTokens      map[string]map[string]bool `json:"ip_tokens"`
}

var (
    data     = AppData{AuthorizedIPs: make(map[string]bool), IpTokens: make(map[string]map[string]bool)}
    mu       sync.RWMutex
    dataFile = "auth.json"
)

func loadData() {
    file, err := os.ReadFile(dataFile)
    if err != nil {
        if os.IsNotExist(err) {
            return
        }
        log.Printf("Error loading data: %v", err)
        return
    }
    mu.Lock()
    defer mu.Unlock()
    json.Unmarshal(file, &data)
}

func saveData() {
    mu.RLock()
    defer mu.RUnlock()
    bytes, err := json.MarshalIndent(data, "", "  ")
    if err != nil {
        log.Printf("Error marshaling data: %v", err)
        return
    }
    os.WriteFile(dataFile, bytes, 0644)
}

func generateToken() string {
    b := make([]byte, 12)
    rand.Read(b)
    return hex.EncodeToString(b)
}

func getIP(r *http.Request) string {
    ip := r.Header.Get("X-Forwarded-For")
    if ip == "" {
        ip, _, _ = net.SplitHostPort(r.RemoteAddr)
    }
    return strings.TrimSpace(strings.Split(ip, ",")[0])
}

func main() {
    port := flag.Int("port", 8000, "Port to listen on")
    domain := flag.String("domain", "", "Public domain (required)")
    ntfyURL := flag.String("ntfy", "", "full ntfy url with topic (required)")
    flag.Parse()

    if *domain == "" {
        log.Fatal("Error: --domain argument is required")
    }

    if *ntfyURL == "" {
        log.Fatal("Error: --ntfy argument is required")
    }

    loadData()

    // Route to authorize IPs
    http.HandleFunc("/auth-ip", func(w http.ResponseWriter, r *http.Request) {
        // FIXED: Using r.URL.Query()
        ipToAuth := r.URL.Query().Get("ip")
        if ipToAuth == "" {
            http.Error(w, "Missing IP", 400)
            return
        }
        mu.Lock()
        data.AuthorizedIPs[ipToAuth] = true
        if data.IpTokens[ipToAuth] == nil {
            data.IpTokens[ipToAuth] = make(map[string]bool)
        }
        mu.Unlock()
        saveData()
        fmt.Fprintf(w, "IP %s authorized successfully!", ipToAuth)
    })

    // Endpoint to revoke IP authorization
    http.HandleFunc("/revoke-ip", func(w http.ResponseWriter, r *http.Request) {
        ipToRevoke := r.URL.Query().Get("ip")
        if ipToRevoke == "" {
            http.Error(w, "Missing IP parameter", http.StatusBadRequest)
            return
        }

        mu.Lock()
        delete(data.AuthorizedIPs, ipToRevoke)
        delete(data.IpTokens, ipToRevoke)
        mu.Unlock()

        saveData()

        log.Printf("Access revoked for IP: %s", ipToRevoke)
        fmt.Fprintf(w, "Authorization for IP %s has been revoked.", ipToRevoke)
    })

    http.HandleFunc("/validate", func(w http.ResponseWriter, r *http.Request) {
        clientIP := getIP(r)
        originalURI := r.Header.Get("X-Original-URI")
        parsedURL, _ := url.Parse(originalURI)
        lowerPath := strings.ToLower(parsedURL.Path)

        // 1. Exclusions
        staticExts := []string{".js", ".css", ".png", ".ico", ".json", ".map", ".svg"}
        for _, ext := range staticExts {
            if strings.HasSuffix(lowerPath, ext) {
                w.WriteHeader(http.StatusOK)
                return
            }
        }

        mu.RLock()
        isAuthorized := data.AuthorizedIPs[clientIP]
        mu.RUnlock()

        // 2. IP NOT AUTHORIZED -> 401 Unauthorized
        if !isAuthorized {
            authLink := fmt.Sprintf("https://%s/auth-ip?ip=%s", *domain, clientIP)
            revokeLink := fmt.Sprintf("https://%s/revoke-ip?ip=%s", *domain, clientIP)

            go func() {
                message := fmt.Sprintf("Access attempt from unauthorized IP: %s", clientIP)
                req, _ := http.NewRequest("POST", *ntfyURL, strings.NewReader(message))

                req.Header.Set("Title", "Security Alert - MyTV")
                req.Header.Set("Priority", "high")
                req.Header.Set("Tags", "warning,lock")

                actions := fmt.Sprintf(
                    "view, Authorize IP, %s; view, Revoke IP, %s",
                    authLink,
                    revokeLink,
                )
                req.Header.Set("Action", actions)

                client := &http.Client{}
                resp, _ := client.Do(req)
                defer resp.Body.Close()
            }()

            w.WriteHeader(http.StatusUnauthorized)
            return
        }

        // 3. IP AUTHORIZED - TOKEN CHECK
        receivedToken := parsedURL.Query().Get("secure")
        mu.RLock()
        validTokens := data.IpTokens[clientIP]
        tokenIsValid := validTokens[receivedToken]
        mu.RUnlock()

        if receivedToken == "" {
            // Transparent redirect with new token linked to this IP
            newToken := generateToken()
            mu.Lock()
            data.IpTokens[clientIP][newToken] = true
            mu.Unlock()
            saveData()

            q := parsedURL.Query()
            q.Set("secure", newToken)

            newLocation := parsedURL.Path
            if newLocation == "" {
                newLocation = "/"
            }
            if q.Encode() != "" {
                newLocation += "?" + q.Encode()
            }

            w.Header().Set("Location", newLocation)
            w.WriteHeader(http.StatusTemporaryRedirect)
            return
        }

        // Valid token for this IP
        if tokenIsValid {
            w.WriteHeader(http.StatusOK)
            return
        }

        // 4. IP AUTHORIZED BUT TOKEN INVALID -> 403 Forbidden
        w.WriteHeader(http.StatusForbidden)
        fmt.Fprint(w, "Forbidden: Invalid token for this IP.")
    })

    log.Printf("Auth server running on :%d with %s", *port, dataFile)
    log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}