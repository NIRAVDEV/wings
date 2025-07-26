package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
)

// Request/Response structures
type CreateServerRequest struct {
	ServerName string `json:"serverName"`
	UserEmail  string `json:"userEmail"`
	Type       string `json:"type,omitempty"` // Optional type of server
	Software   string `json:"software"`      // Required software type (e.g., "
	RAM        string `json:"ram,omitempty"` // Optional RAM allocation
	Storage    string `json:"storage,omitempty"` // Optional storage allocation
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
	Content string `json:"content,omitempty"`
}

// Utility functions
func loadToken() string {
	_ = godotenv.Load()
	token := os.Getenv("HANDSHAKE_TOKEN")
	if token == "" {
		fmt.Println("Missing HANDSHAKE_TOKEN in .env")
		os.Exit(1)
	}
	return token
}
func extractUserId(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) > 0 {
		return parts[0]
	}
	return "user"
}

// Only allow [a-zA-Z0-9.-] in Docker names
var dockerNameRe = regexp.MustCompile("[^a-zA-Z0-9.-]")

func sanitizeDockerName(s string) string {
	return dockerNameRe.ReplaceAllString(s, "")
}
func buildContainerId(serverName, userId string) string {
	raw := fmt.Sprintf("%s-%s", serverName, userId)
	return sanitizeDockerName(raw)
}
func selectImage(software string) string {
    // Always return the universal image
    return "itzg/minecraft-server:latest"
}
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

// Handlers
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
	if req.ServerName == "" || req.UserEmail == "" || req.Software == "" {
		http.Error(w, "serverName, userEmail, and software required", http.StatusBadRequest)
		return
	}
	userId := extractUserId(req.UserEmail)
	containerId := buildContainerId(req.ServerName, userId)
	VolumePath, err := filepath.Abs(filepath.Join("volume", containerId))
	if err != nil {
		http.Error(w, "Failed to get absolute volume path: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Println("Creating volume directory at:", VolumePath)

	if err := os.MkdirAll(VolumePath, 0755); err != nil {
		http.Error(w, "Failed to create volume: "+err.Error(), http.StatusInternalServerError)
		return
	}
	image := selectImage(req.Software)
	// Pull docker image
	pullCmd := exec.Command("docker", "pull", image)
	if output, err := pullCmd.CombinedOutput(); err != nil {
		http.Error(w, "Failed to pull docker image: "+string(output), http.StatusInternalServerError)
		return
	}

	if err := os.MkdirAll(VolumePath, 0755); err != nil {
		http.Error(w, "Failed to create volume: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Println("Creating server...")
	fmt.Printf("ServerName: %s, Email: %s, Image: %s\n", req.ServerName, req.UserEmail, image)
	fmt.Println("VolumePath:", VolumePath)
	
	// Create (do not start) server container
	createCmd := exec.Command(
		"docker", "create",
		"--name", containerId,
		"-v", VolumePath+":/data",
		"-e", "EULA=TRUE",
		"-e", "TYPE="+strings.ToUpper(req.Software),
		"-e", "MEMORY="+req.RAM,
		"-e", "STORAGE="+req.Storage,
		"--restart", "unless-stopped",
		image,
	)
	fmt.Println("Running command:", createCmd.String())

	output, err := createCmd.CombinedOutput()
	if err != nil {
		http.Error(w, "Failed to create docker container: "+string(output), http.StatusInternalServerError)
		return
	}
	fmt.Println("Docker create output:", string(output))
	resp := CreateServerResponse{
		Status:   "ok",
		ServerId: containerId,
		Message:  fmt.Sprintf("Server container created with image '%s'", image),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

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
	userId := extractUserId(req.UserEmail)
	containerId := buildContainerId(req.ServerName, userId)
	// Try to start
	startCmd := exec.Command("docker", "start", containerId)
	if output, err := startCmd.CombinedOutput(); err != nil {
		http.Error(w, "Failed to start container: "+string(output), http.StatusInternalServerError)
		return
	}
	resp := GenericResponse{
		Status:  "ok",
		Message: fmt.Sprintf("Server %s started", containerId),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

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
	containerId := buildContainerId(req.ServerName, userId)
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
	containerId := buildContainerId(req.ServerName, userId)
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
func fileManagerHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
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
	if err := http.ListenAndServe(":25575", tokenMiddleware(token, mux)); err != nil {
		fmt.Println("Server failed:", err)
	}
}
