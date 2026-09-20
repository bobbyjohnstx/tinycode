package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
)

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func requestDirectory(r *http.Request, fallback string) string {
	if dir := r.URL.Query().Get("directory"); dir != "" {
		return dir
	}
	if hdr := r.Header.Get("x-tinycode-directory"); hdr != "" {
		if decoded, err := url.PathUnescape(hdr); err == nil {
			return decoded
		}
		return hdr
	}
	return fallback
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	defer r.Body.Close()
	err := json.NewDecoder(r.Body).Decode(v)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
