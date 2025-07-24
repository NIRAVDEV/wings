package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// HandshakeResponse is the response JSON for /handshake
type HandshakeResponse struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// CreateServerRequest defines the expected JSON body for /server endpoints
type CreateServerRequest struct {
	ServerName string `json:"serverName"` // e.g. "lobby"
	UserEmail  string `json:"userEmail"`  // e.g. "alice@example.com"
	HostPort   string `json:"hostPort"`   // optional for /server/start
}

// GenericResponse is a generic status response
type GenericResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// UploadFileRequest defines JSON body for file upload/edit
type UploadFileRequest struct {
	ServerName    string `json:"serverName"`
	UserEmail     string `json:"userEmail"`
	Path          string `json:"path"`
	ContentBase64 string `json:"contentBase64"`
}

// DeleteFileRequest defines JSON body for file delete
type DeleteFileRequest struct {
	ServerName string `json:"serverName"`
	UserEmail  string `json:"userEmail"`
	Path       string `json:"path"`
}

// Load handshake token from .env file
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

// Extract user id (before @) from email
func extractUserId(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 0 {
		return "unknown"
	}
	return parts[0]
}

// Get server data directory path
func getServerDataDir(serverName, userEmail string) string {
	userId := extractUserId(userEmail)
	return filepath.Join("./volume", fmt.Sprintf("%s-%s", serverName, userId))
}

// Middleware to check Bearer token in Authorization header
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

// Handshake handler: simply responds ok if token is valid (middleware verifies token)
func handshakeHandler(w http.ResponseWriter, r *http.Request) {
	resp := HandshakeResponse{Status: "ok"}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Start MC server container or start if already exists
func startMCServer(name, userEmail, hostPort string) error {
	userId := extractUserId(userEmail)
	containerName := fmt.Sprintf("%s-%s", name, userId)
	volumePath := getServerDataDir(name, userEmail)

	if err := os.MkdirAll(volumePath, 0755); err != nil {
		return fmt.Errorf("failed to create volume dir: %w", err)
	}

	cmdCheck := exec.Command("docker", "ps", "-a", "-q", "-f", "name="+containerName)
	output, err := cmdCheck.Output()
	if err != nil {
		return fmt.Errorf("docker check error: %w", err)
	}

	if len(output) > 0 {
		cmdStart := exec.Command("docker", "start", containerName)
		out, err := cmdStart.CombinedOutput()
		if err != nil {
			return fmt.Errorf("docker start error: %s, %w", string(out), err)
		}
		return nil
	}

	cmdRun := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"-p", hostPort+":25565",
		"-v", volumePath+":/data",
		"itzg/minecraft-server",
	)
	out, err := cmdRun.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run error: %s, %w", string(out), err)
	}
	return nil
}

// Stop MC server container
func stopMCServer(name, userEmail string) error {
	userId := extractUserId(userEmail)
	containerName := fmt.Sprintf("%s-%s", name, userId)

	cmd := exec.Command("docker", "stop", containerName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker stop error: %s, %w", string(out), err)
	}
	return nil
}

// Restart MC server container
func restartMCServer(name, userEmail string) error {
	userId := extractUserId(userEmail)
	containerName := fmt.Sprintf("%s-%s", name, userId)

	cmd := exec.Command("docker", "restart", containerName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker restart error: %s, %w", string(out), err)
	}
	return nil
}

// Handler: POST /server/start
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
	err := startMCServer(req.ServerName, req.UserEmail, req.HostPort)
	if err != nil {
		resp := GenericResponse{Status: "error", Message: err.Error()}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}
	resp := GenericResponse{Status: "ok", Message: "Server started successfully"}
	json.NewEncoder(w).Encode(resp)
}

// Handler: POST /server/stop
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
	err := stopMCServer(req.ServerName, req.UserEmail)
	if err != nil {
		resp := GenericResponse{Status: "error", Message: err.Error()}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}
	resp := GenericResponse{Status: "ok", Message: "Server stopped successfully"}
	json.NewEncoder(w).Encode(resp)
}

// Handler: POST /server/restart
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
	err := restartMCServer(req.ServerName, req.UserEmail)
	if err != nil {
		resp := GenericResponse{Status: "error", Message: err.Error()}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}
	resp := GenericResponse{Status: "ok", Message: "Server restarted successfully"}
	json.NewEncoder(w).Encode(resp)
}

// Handler: GET /files/list
func listFilesHandler(w http.ResponseWriter, r *http.Request) {
	serverName := r.URL.Query().Get("serverName")
	userEmail := r.URL.Query().Get("userEmail")
	relPath := r.URL.Query().Get("path") // optional

	if serverName == "" || userEmail == "" {
		http.Error(w, "serverName and userEmail are required", http.StatusBadRequest)
		return
	}

	baseDir := getServerDataDir(serverName, userEmail)
	dirPath := baseDir
	if relPath != "" {
		dirPath = filepath.Join(baseDir, relPath)
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, "Failed to read directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	type FileEntry struct {
		Name  string `json:"name"`
		IsDir bool   `json:"isDir"`
	}
	var files []FileEntry
	for _, entry := range entries {
		files = append(files, FileEntry{
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

// Handler: GET /files/download
func downloadFileHandler(w http.ResponseWriter, r *http.Request) {
	serverName := r.URL.Query().Get("serverName")
	userEmail := r.URL.Query().Get("userEmail")
	filePath := r.URL.Query().Get("path")

	if serverName == "" || userEmail == "" || filePath == "" {
		http.Error(w, "serverName, userEmail, and path are required", http.StatusBadRequest)
		return
	}

	baseDir := getServerDataDir(serverName, userEmail)
	fullPath := filepath.Join(baseDir, filePath)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		http.Error(w, "Failed to read file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(filePath))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

// Handler: POST /files/upload (upload or overwrite)
func uploadFileHandler(w http.ResponseWriter, r *http.Request) {
	var req UploadFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.ServerName == "" || req.UserEmail == "" || req.Path == "" || req.ContentBase64 == "" {
		http.Error(w, "serverName, userEmail, path, and contentBase64 are required", http.StatusBadRequest)
		return
	}

	baseDir := getServerDataDir(req.ServerName, req.UserEmail)
	fullPath := filepath.Join(baseDir, req.Path)

	content, err := base64.StdEncoding.DecodeString(req.ContentBase64)
	if err != nil {
		http.Error(w, "Failed to decode base64: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		http.Error(w, "Failed to create directories: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(fullPath, content, 0644); err != nil {
		http.Error(w, "Failed to write file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(GenericResponse{Status: "ok", Message: "File uploaded"})
}

// Handler: POST /files/delete
func deleteFileHandler(w http.ResponseWriter, r *http.Request) {
	var req DeleteFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.ServerName == "" || req.UserEmail == "" || req.Path == "" {
		http.Error(w, "serverName, userEmail, and path are required", http.StatusBadRequest)
		return
	}

	baseDir := getServerDataDir(req.ServerName, req.UserEmail)
	fullPath := filepath.Join(baseDir, req.Path)

	if err := os.Remove(fullPath); err != nil {
		http.Error(w, "Failed to delete file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(GenericResponse{Status: "ok", Message: "File deleted"})
}

func main() {
	token := loadToken()

	mux := http.NewServeMux()

	// Authentication protected routes
	mux.HandleFunc("/handshake", handshakeHandler)

	mux.HandleFunc("/server/start", startServerHandler)
	mux.HandleFunc("/server/stop", stopServerHandler)
	mux.HandleFunc("/server/restart", restartServerHandler)

	mux.HandleFunc("/files/list", listFilesHandler)
	mux.HandleFunc("/files/download", downloadFileHandler)
	mux.HandleFunc("/files/upload", uploadFileHandler)
	mux.HandleFunc("/files/delete", deleteFileHandler)

	protectedHandler := tokenMiddleware(token, mux)

	fmt.Println("Node HTTP server listening on :25575")
	if err := http.ListenAndServe(":25575", protectedHandler); err != nil {
		panic(err)
	}
}
