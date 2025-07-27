package main

import (
    "encoding/json"
    "fmt"
    "io"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "regexp"
    "strings"

    "github.com/gorilla/websocket"
    "github.com/joho/godotenv"
)

// ================== Request/Response Structs ==================
type CreateServerRequest struct {
    ServerName string `json:"serverName"`
    UserEmail  string `json:"userEmail"`
    Type       string `json:"type,omitempty"`
    Software   string `json:"software"`
    RAM        string `json:"ram,omitempty"`
    Storage    string `json:"storage,omitempty"`
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

type ConsoleRequest struct {
    ServerName string `json:"serverName"`
    UserEmail  string `json:"userEmail"`
    Command    string `json:"command"`
}

// ================== Utilities ==================
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

var dockerNameRe = regexp.MustCompile("[^a-zA-Z0-9.-]")

func sanitizeDockerName(s string) string {
    return dockerNameRe.ReplaceAllString(s, "")
}

func buildContainerId(serverName, userId string) string {
    raw := fmt.Sprintf("%s-%s", serverName, userId)
    return sanitizeDockerName(raw)
}

func selectImage(software string) string {
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

// ================== Handlers ==================
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
    volumePath, err := filepath.Abs(filepath.Join("volume", containerId))
    if err != nil {
        http.Error(w, "Failed to get absolute volume path: "+err.Error(), http.StatusInternalServerError)
        return
    }

    if err := os.MkdirAll(volumePath, 0755); err != nil {
        http.Error(w, "Failed to create volume: "+err.Error(), http.StatusInternalServerError)
        return
    }

    image := selectImage(req.Software)
    if output, err := exec.Command("docker", "pull", image).CombinedOutput(); err != nil {
        http.Error(w, "Failed to pull docker image: "+string(output), http.StatusInternalServerError)
        return
    }

    createCmd := exec.Command(
        "docker", "create",
        "--name", containerId,
        "-v", volumePath+":/data",
        "-e", "EULA=TRUE",
        "-e", "TYPE="+strings.ToUpper(req.Software),
        "-e", "MEMORY="+req.RAM,
        "-e", "STORAGE="+req.Storage,
        "--restart", "unless-stopped",
        image,
    )
    if output, err := createCmd.CombinedOutput(); err != nil {
        http.Error(w, "Failed to create docker container: "+string(output), http.StatusInternalServerError)
        return
    }

    resp := CreateServerResponse{
        Status:   "ok",
        ServerId: containerId,
        Message:  fmt.Sprintf("Server container created with image '%s'", image),
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}

func startServerHandler(w http.ResponseWriter, r *http.Request) {
    var req CreateServerRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON body", http.StatusBadRequest)
        return
    }
    if req.ServerName == "" || req.UserEmail == "" {
        http.Error(w, "serverName and userEmail required", http.StatusBadRequest)
        return
    }

    containerId := buildContainerId(req.ServerName, extractUserId(req.UserEmail))
    if output, err := exec.Command("docker", "start", containerId).CombinedOutput(); err != nil {
        http.Error(w, "Failed to start container: "+string(output), http.StatusInternalServerError)
        return
    }
    json.NewEncoder(w).Encode(GenericResponse{Status: "ok", Message: "Server started"})
}

func stopServerHandler(w http.ResponseWriter, r *http.Request) {
    var req CreateServerRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON body", http.StatusBadRequest)
        return
    }
    if req.ServerName == "" || req.UserEmail == "" {
        http.Error(w, "serverName and userEmail required", http.StatusBadRequest)
        return
    }

    containerId := buildContainerId(req.ServerName, extractUserId(req.UserEmail))
    if output, err := exec.Command("docker", "stop", containerId).CombinedOutput(); err != nil {
        http.Error(w, "Failed to stop container: "+string(output), http.StatusInternalServerError)
        return
    }
    json.NewEncoder(w).Encode(GenericResponse{Status: "ok", Message: "Server stopped"})
}

func restartServerHandler(w http.ResponseWriter, r *http.Request) {
    var req CreateServerRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON body", http.StatusBadRequest)
        return
    }
    if req.ServerName == "" || req.UserEmail == "" {
        http.Error(w, "serverName and userEmail required", http.StatusBadRequest)
        return
    }

    containerId := buildContainerId(req.ServerName, extractUserId(req.UserEmail))
    if output, err := exec.Command("docker", "restart", containerId).CombinedOutput(); err != nil {
        http.Error(w, "Failed to restart container: "+string(output), http.StatusInternalServerError)
        return
    }
    json.NewEncoder(w).Encode(GenericResponse{Status: "ok", Message: "Server restarted"})
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

func fileUploadHandler(w http.ResponseWriter, r *http.Request) {
    r.ParseMultipartForm(10 << 20)
    file, handler, err := r.FormFile("file")
    if err != nil {
        http.Error(w, "Error retrieving the file: "+err.Error(), http.StatusBadRequest)
        return
    }
    defer file.Close()

    path := r.FormValue("path")
    if path == "" {
        http.Error(w, "Missing path field", http.StatusBadRequest)
        return
    }

    dstPath := filepath.Join("volume", path, handler.Filename)
    out, err := os.Create(dstPath)
    if err != nil {
        http.Error(w, "Unable to create the file: "+err.Error(), http.StatusInternalServerError)
        return
    }
    defer out.Close()
    io.Copy(out, file)

    json.NewEncoder(w).Encode(GenericResponse{Status: "ok", Message: "File uploaded successfully"})
}

func fileDownloadHandler(w http.ResponseWriter, r *http.Request) {
    path := r.URL.Query().Get("path")
    if path == "" {
        http.Error(w, "Missing path parameter", http.StatusBadRequest)
        return
    }

    absPath := filepath.Join("volume", path)
    file, err := os.Open(absPath)
    if err != nil {
        http.Error(w, "File not found: "+err.Error(), http.StatusNotFound)
        return
    }
    defer file.Close()

    w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(path))
    w.Header().Set("Content-Type", "application/octet-stream")
    io.Copy(w, file)
}

func serverStatusHandler(w http.ResponseWriter, r *http.Request) {
    serverName := r.URL.Query().Get("serverName")
    userEmail := r.URL.Query().Get("userEmail")
    if serverName == "" || userEmail == "" {
        http.Error(w, "serverName and userEmail required", http.StatusBadRequest)
        return
    }

    containerId := buildContainerId(serverName, extractUserId(userEmail))
    output, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", containerId).Output()
    if err != nil {
        http.Error(w, "Failed to get status: "+err.Error(), http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(GenericResponse{Status: "ok", Message: strings.TrimSpace(string(output))})
}

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

func consoleHandler(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println("WebSocket upgrade failed:", err)
        return
    }
    defer conn.Close()

    type ConsolePayload struct {
        ServerName string `json:"serverName"`
        UserEmail  string `json:"userEmail"`
        Command    string `json:"command,omitempty"`
        Action     string `json:"action"`
    }

    // Read initial connection payload
    _, msg, err := conn.ReadMessage()
    if err != nil {
        log.Println("Failed to read initial message:", err)
        return
    }

    var init ConsolePayload
    if err := json.Unmarshal(msg, &init); err != nil {
        conn.WriteMessage(websocket.TextMessage, []byte("Invalid init JSON"))
        return
    }

    userId := extractUserId(init.UserEmail)
    containerId := buildContainerId(init.ServerName, userId)

    // Start docker logs -f
    logCmd := exec.Command("docker", "logs", "-f", containerId)
    stdout, err := logCmd.StdoutPipe()
    if err != nil {
        conn.WriteMessage(websocket.TextMessage, []byte("Failed to attach to logs"))
        return
    }

    if err := logCmd.Start(); err != nil {
        conn.WriteMessage(websocket.TextMessage, []byte("Failed to start log stream"))
        return
    }

    // Stream logs to WebSocket
    go func() {
        buf := make([]byte, 1024)
        for {
            n, err := stdout.Read(buf)
            if err != nil {
                break
            }
            if n > 0 {
                conn.WriteMessage(websocket.TextMessage, buf[:n])
            }
        }
    }()

    // Listen for commands
    for {
        _, msg, err := conn.ReadMessage()
        if err != nil {
            break
        }

        var cmdPayload ConsolePayload
        if err := json.Unmarshal(msg, &cmdPayload); err != nil {
            conn.WriteMessage(websocket.TextMessage, []byte("Invalid command payload"))
            continue
        }

        if cmdPayload.Action == "command" && cmdPayload.Command != "" {
            dockerCmd := exec.Command("docker", "exec", containerId, "rcon-cli", cmdPayload.Command)
            out, err := dockerCmd.CombinedOutput()
            if err != nil {
                conn.WriteMessage(websocket.TextMessage, []byte("Command failed: "+err.Error()+"\n"+string(out)))
                continue
            }
            conn.WriteMessage(websocket.TextMessage, out)
        }
    }

    _ = logCmd.Process.Kill()
}

// ================== Main ==================

func main() {
    token := loadToken()
    mux := http.NewServeMux()
    mux.HandleFunc("/server/create", createServerHandler)
    mux.HandleFunc("/server/start", startServerHandler)
    mux.HandleFunc("/server/stop", stopServerHandler)
    mux.HandleFunc("/server/restart", restartServerHandler)
    mux.HandleFunc("/server/status", serverStatusHandler)
    mux.HandleFunc("/ws/console", consoleHandler)
    mux.HandleFunc("/file_manager", fileManagerHandler)
    mux.HandleFunc("/file/upload", fileUploadHandler)
    mux.HandleFunc("/file/download", fileDownloadHandler)
    fmt.Println("Agent listening on :25575")
    if err := http.ListenAndServe(":25575", tokenMiddleware(token, mux)); err != nil {
        fmt.Println("Server failed:", err)
    }
}