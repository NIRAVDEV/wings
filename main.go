package main

import (
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

// CreateServerRequest defines the expected JSON body for /server/start
type CreateServerRequest struct {
	ServerName string `json:"serverName"` // e.g. "lobby"
	UserEmail  string `json:"userEmail"`  // e.g. "alice@example.com"
	HostPort   string `json:"hostPort"`   // e.g. "25565" - port to expose on host
}

// GenericResponse is a generic status response
type GenericResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// loadToken loads HANDSHAKE_TOKEN from .env file
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

// extractUserId extracts the part before '@' from email, e.g., 'alice@example.com' -> 'alice'
func extractUserId(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 0 {
		return "unknown"
	}
	return parts[0]
}
func getServerDataDir(serverName, userEmail string) string {
	userId := extractUserId(userEmail)
	return filepath.Join("./volume", fmt.Sprintf("%s-%s", serverName, userId))
}
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
func downloadFileHandler(w http.ResponseWriter, r *http.Request) {
	serverName := r.URL.Query().Get("serverName")
	userEmail := r.URL.Query().Get("userEmail")
	filePath := r.URL.Query().Get("path") // must be relative, required

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


// startMCServer creates/starts the container with unique name: <serverName>-<userId>
func startMCServer(name, userEmail, hostPort string) error {
	userId := extractUserId(userEmail)
	containerName := fmt.Sprintf("%s-%s", name, userId)
	volumePath := filepath.Join("./volume", containerName)

	// Create volume folder if it does not exist
	if err := os.MkdirAll(volumePath, 0755); err != nil {
		return fmt.Errorf("failed to create volume dir: %w", err)
	}

	// Check if container already exists
	cmdCheck := exec.Command("docker", "ps", "-a", "-q", "-f", "name="+containerName)
	output, err := cmdCheck.Output()
	if err != nil {
		return fmt.Errorf("docker check error: %w", err)
	}

	if len(output) > 0 {
		// Container exists, try to start it
		cmdStart := exec.Command("docker", "start", containerName)
		out, err := cmdStart.CombinedOutput()
		if err != nil {
			return fmt.Errorf("docker start error: %s, %w", string(out), err)
		}
		return nil
	}

	// Run a new container
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

// startServerHandler handles POST /server/start
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
		http.Error(w, "serverName and userEmail are required", http.StatusBadRequest)
		return
	}

	if req.HostPort == "" {
		// Default port if not passed
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

// handshakeHandler handles POST /handshake
func handshakeHandler(w http.ResponseWriter, r *http.Request) {
	resp := HandshakeResponse{Status: "ok"}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// tokenMiddleware checks Authorization header is "Bearer <token>"
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
// Start Minecraft server container
func startMCServer(name, userEmail, hostPort string) error {
	userId := extractUserId(userEmail)
	containerName := fmt.Sprintf("%s-%s", name, userId)
	volumePath := filepath.Join("./volume", containerName)

	if err := os.MkdirAll(volumePath, 0755); err != nil {
		return fmt.Errorf("failed to create volume dir: %w", err)
	}

	// Check if container exists
	cmdCheck := exec.Command("docker", "ps", "-a", "-q", "-f", "name="+containerName)
	output, err := cmdCheck.Output()
	if err != nil {
		return fmt.Errorf("docker check error: %w", err)
	}

	if len(output) > 0 {
		// Start container if it is stopped
		cmdStart := exec.Command("docker", "start", containerName)
		out, err := cmdStart.CombinedOutput()
		if err != nil {
			return fmt.Errorf("docker start error: %s, %w", string(out), err)
		}
		return nil
	}

	// Run new container
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

// Handler for POST /server/start
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
		req.HostPort = "25565" // default port
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

// Handler for POST /server/stop
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

// Handler for POST /server/restart
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


func main() {
	token := loadToken()

	mux := http.NewServeMux()
	mux.HandleFunc("/handshake", handshakeHandler)
	mux.HandleFunc("/server/start", startServerHandler)

	protectedHandler := tokenMiddleware(token, mux)

	fmt.Println("Node HTTP server listening on :25575")
	if err := http.ListenAndServe(":25575", protectedHandler); err != nil {
		panic(err)
	}
mux := http.NewServeMux()

mux.HandleFunc("/handshake", handshakeHandler)

// Add new server control endpoints
mux.HandleFunc("/server/start", startServerHandler)
mux.HandleFunc("/server/stop", stopServerHandler)
mux.HandleFunc("/server/restart", restartServerHandler)

protectedHandler := tokenMiddleware(token, mux)

fmt.Println("Node HTTP server listening on :25575")
err := http.ListenAndServe(":25575", protectedHandler)
if err != nil {
	panic(err)
}

}
