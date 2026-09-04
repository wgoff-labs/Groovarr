package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ValidateJSON ensures the request body is valid and meets requirements.
func ValidateJSON(r *http.Request, v interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return nil
}

// BadRequest helper for standard error messages.
func BadRequest(w http.ResponseWriter, msg string) {
	http.Error(w, msg, http.StatusBadRequest)
}
