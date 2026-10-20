package misc

import (
	"bytes"
	"encoding/json"
	"net/http"
)

func SendJson(w http.ResponseWriter, statusCode int, payload any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(buf.Bytes())
}

func SendErrorMessage(w http.ResponseWriter, status int, message string) {
	http.Error(w, message, status)
}
