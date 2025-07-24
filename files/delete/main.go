type DeleteFileRequest struct {
	ServerName string `json:"serverName"`
	UserEmail  string `json:"userEmail"`
	Path       string `json:"path"`
}

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
