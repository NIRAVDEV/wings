package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// Request/Response structures
type CreateServerRequest struct {
	ServerName string `json:"serverName"`
	UserEmail  string `json:"userEmail"`
	HostPort   string `json:"hostPort,omitempty"`
}

type CreateServerResponse struct {
	Status   string `json:"status"`
	ServerId string `json:"serverId,omitempty"`
	Message  string `json:"message,omitempty"`
}

type GenericResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type FileManagerRequest struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"` // For writing
}

// Load token from .env file
func loadToken() string {
	_ = godotenv.Load()
	token := os.Getenv("HANDSHAKE_TOKEN")
	if token == "" {
		fmt.Println("Missing HANDSHAKE_TOKEN in .env")
		os.Exit(1)
	}
	return token
}

// Extract user ID prefix from email
func extractUserId(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) > 0 {
		return parts[0]
	}
	return "user"
}

// Middleware to check Bearer token
func tokenMiddleware(expected string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "Unauthorized - missing Bearer token", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != expected {
			http.Error(w, "Unauthorized - invalid token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// /server/create - creates server folder
func createServerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req CreateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.ServerName == "" || req.UserEmail == "" {
		http.Error(w, "serverName and userEmail required", http.StatusBadRequest)
		return
	}

	userId := extractUserId(req.UserEmail)
	serverId := fmt.Sprintf("%s-%s", req.ServerName, userId)
	volumePath := filepath.Join("./volume", serverId)

	if err := os.MkdirAll(volumePath, 0755); err != nil {
		http.Error(w, "Failed to create volume: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := CreateServerResponse{
		Status:   "ok",
		ServerId: serverId,
		Message:  "Server folder created",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// /server/start - start or run Docker container
func startServerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req CreateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.ServerName == "" || req.UserEmail == "" {
		http.Error(w, "serverName and userEmail required", http.StatusBadRequest)
		return
	}
	if req.HostPort == "" {
		req.HostPort = "25565"
	}

	userId := extractUserId(req.UserEmail)
	containerId := fmt.Sprintf("%s-%s", req.ServerName, userId)
	volumePath := filepath.Join("./volume", containerId)

	if _, err := os.Stat(volumePath); os.IsNotExist(err) {
		http.Error(w, "Server volume missing, create server first", http.StatusBadRequest)
		return
	}

	// Check if container exists
	checkCmd := exec.Command("docker", "inspect", containerId)
	err := checkCmd.Run()
	if err == nil {
		// Container exists, start it
		startCmd := exec.Command("docker", "start", containerId)
		if output, err := startCmd.CombinedOutput(); err != nil {
			http.Error(w, "Failed to start container: "+string(output), http.StatusInternalServerError)
			return
		}
	} else {
		// Container doesn't exist - run new container
		runCmd := exec.Command("docker", "run", "-d",
			"--name", containerId,
			"-v", volumePath+":/data",
			"-p", req.HostPort+":25565",
			"-e", "EULA=TRUE",
			"--restart", "unless-stopped",
			"itzg/minecraft-server",
		)
		if output, err := runCmd.CombinedOutput(); err != nil {
			http.Error(w, "Failed to run container: "+string(output), http.StatusInternalServerError)
			return
		}
	}

	resp := GenericResponse{
		Status:  "ok",
		Message: fmt.Sprintf("Server %s started on port %s", containerId, req.HostPort),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// /server/stop - stop running container
func stopServerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req CreateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.ServerName == "" || req.UserEmail == "" {
		http.Error(w, "serverName and userEmail required", http.StatusBadRequest)
		return
	}

	userId := extractUserId(req.UserEmail)
	containerId := fmt.Sprintf("%s-%s", req.ServerName, userId)

	stopCmd := exec.Command("docker", "stop", containerId)
	if output, err := stopCmd.CombinedOutput(); err != nil {
		http.Error(w, "Failed to stop container: "+string(output), http.StatusInternalServerError)
		return
	}

	resp := GenericResponse{
		Status:  "ok",
		Message: fmt.Sprintf("Server %s stopped", containerId),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// /server/restart - restart container
func restartServerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req CreateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.ServerName == "" || req.UserEmail == "" {
		http.Error(w, "serverName and userEmail required", http.StatusBadRequest)
		return
	}

	userId := extractUserId(req.UserEmail)
	containerId := fmt.Sprintf("%s-%s", req.ServerName, userId)

	restartCmd := exec.Command("docker", "restart", containerId)
	if output, err := restartCmd.CombinedOutput(); err != nil {
		http.Error(w, "Failed to restart container: "+string(output), http.StatusInternalServerError)
		return
	}

	resp := GenericResponse{
		Status:  "ok",
		Message: fmt.Sprintf("Server %s restarted", containerId),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// /file_manager - simple read/write files (basic example)
func fileManagerHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Read file content
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "Missing path parameter", http.StatusBadRequest)
			return
		}
		absPath := filepath.Join("./volume", path)
		data, err := ioutil.ReadFile(absPath)
		if err != nil {
			http.Error(w, "Failed to read file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(data)

	case http.MethodPost:
		// Write file content
		var req FileManagerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}
		if req.Path == "" {
			http.Error(w, "Missing path in body", http.StatusBadRequest)
			return
		}
		absPath := filepath.Join("./volume", req.Path)
		if err := ioutil.WriteFile(absPath, []byte(req.Content), 0644); err != nil {
			http.Error(w, "Failed to write file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(GenericResponse{Status: "ok", Message: "File written successfully"})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func main() {
	token := loadToken()

	mux := http.NewServeMux()
	mux.HandleFunc("/server/create", createServerHandler)
	mux.HandleFunc("/server/start", startServerHandler)
	mux.HandleFunc("/server/stop", stopServerHandler)
	mux.HandleFunc("/server/restart", restartServerHandler)
	mux.HandleFunc("/file_manager", fileManagerHandler)

	fmt.Println("Server listening on :25575")
	err := http.ListenAndServe(":25575", tokenMiddleware(token, mux))
	if err != nil {
		fmt.Println("Server failed:", err)
	}
}
