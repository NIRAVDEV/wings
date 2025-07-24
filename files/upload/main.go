import "encoding/base64"

type UploadFileRequest struct {
	ServerName    string `json:"serverName"`
	UserEmail     string `json:"userEmail"`
	Path          string `json:"path"`
	ContentBase64 string `json:"contentBase64"`
}

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

	// Ensure parent directories exist
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
