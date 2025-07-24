package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type HandshakeResponse struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func loadToken() string {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file")
		os.Exit(1)
	}
	token := os.Getenv("HANDSHAKE_TOKEN")
	if token == "" {
		fmt.Println("HANDSHAKE_TOKEN missing in .env")
		os.Exit(1)
	}
	return token
}

// Middleware to verify Authorization header "Bearer <token>"
func tokenMiddleware(expectedToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized - missing Bearer token", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token != expectedToken {
			http.Error(w, "Unauthorized - invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Handshake handler, just confirm the token is valid
func handshakeHandler(w http.ResponseWriter, r *http.Request) {
	resp := HandshakeResponse{Status: "ok"}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	token := loadToken()

	mux := http.NewServeMux()
	mux.HandleFunc("/handshake", handshakeHandler)

	// Wrap all routes with token middleware
	protectedHandler := tokenMiddleware(token, mux)

	fmt.Println("Node HTTP server listening on :25575")
	err := http.ListenAndServe(":25575", protectedHandler)
	if err != nil {
		panic(err)
	}
}
