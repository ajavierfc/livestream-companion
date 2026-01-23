package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const tokenFile = "last_token.txt"
const ntfyURL = "https://ntfy.sh/mytv-X7kgmipX"

func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "fallback" + hex.EncodeToString(b[:4])
	}
	return hex.EncodeToString(b)
}

func saveToken(token string) {
	err := os.WriteFile(tokenFile, []byte(token), 0644)
	if err != nil {
		log.Printf("Error saving token to file: %v", err)
	}
}

func getStoredToken() string {
	content, err := os.ReadFile(tokenFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

func main() {
	port := flag.Int("port", 8000, "Port to listen on")
	domain := flag.String("domain", "", "Public domain (required)")
	flag.Parse()

	if *domain == "" {
		log.Fatal("Error: --domain argument is required")
	}

	http.HandleFunc("/validate", func(w http.ResponseWriter, r *http.Request) {
		originalURI := r.Header.Get("X-Original-URI")
		if originalURI == "" {
			originalURI = "/"
		}

		parsedURL, err := url.Parse(originalURI)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// --- EXCLUSIONS START ---
		lowerPath := strings.ToLower(parsedURL.Path)
		staticExtensions := []string{".ico", ".png", ".js", ".css", ".json", ".map"}
		
		isStatic := false
		for _, ext := range staticExtensions {
			if strings.HasSuffix(lowerPath, ext) {
				isStatic = true
				break
			}
		}

		if isStatic {
			// Allow static assets to pass without token validation
			w.WriteHeader(http.StatusOK)
			return
		}
		// --- EXCLUSIONS END ---

		receivedToken := parsedURL.Query().Get("secure")
		currentToken := getStoredToken()

		if receivedToken == "" {
			newToken := generateToken()
			saveToken(newToken)

			secureURL := fmt.Sprintf("https://%s%s?secure=%s", *domain, parsedURL.Path, newToken)

			payload := strings.NewReader("New access attempt. Link: " + secureURL)
			req, _ := http.NewRequest("POST", ntfyURL, payload)
			req.Header.Set("Title", "Security Alert - MyTV")
			
			go http.DefaultClient.Do(req)

			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "Forbidden: New token generated and sent via ntfy.")
			return
		}

		if receivedToken == currentToken {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "Forbidden: Invalid token.")
	})

	log.Printf("Auth server running on :%d for domain %s", *port, *domain)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}