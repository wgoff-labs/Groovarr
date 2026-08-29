package connections

import (
	"encoding/json"
	"net/http"

	"github.com/groovarr/groovarr/backend/internal/clients"
	"github.com/groovarr/groovarr/backend/internal/store"
)

// ConnectionsHandler handles connection status, connect, disconnect.
func ConnectionsHandler(w http.ResponseWriter, r *http.Request) {
	m := New()

	switch r.Method {
	case http.MethodGet:
		// Return all statuses
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"statuses": m.GetAllStatuses(),
		})

	case http.MethodPost:
		var req struct {
			Action  string `json:"action"` // "connect_lidarr", "disconnect_lidarr", "test_lidarr"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		switch req.Action {
		case "connect_lidarr":
			go m.ConnectLidarr()
			m.log("info", "lidarr", "Connect requested via API")

		case "disconnect_lidarr":
			m.DisconnectLidarr()

		case "test_lidarr":
			// Synchronous test — doesn't change state
			lidarrURL, _ := store.SettingGet("lidarr_url")
			lidarrKey, _ := store.SettingGet("lidarr_api_key")
			if lidarrURL == "" || lidarrKey == "" {
				json.NewEncoder(w).Encode(map[string]string{
					"status": "error",
					"error":  "Lidarr URL and API key are required",
				})
				return
			}
			c, err := clients.NewLidarrClientWith(lidarrURL, lidarrKey)
			if err != nil {
				json.NewEncoder(w).Encode(map[string]string{
					"status": "error",
					"error":  err.Error(),
				})
				return
			}
			_, err = c.GetRootFolders()
			if err != nil {
				json.NewEncoder(w).Encode(map[string]string{
					"status": "error",
					"error":  err.Error(),
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{
				"status": "ok",
				"error":  "",
			})
			return

		default:
			http.Error(w, "unknown action: "+req.Action, http.StatusBadRequest)
			return
		}

		// Return updated statuses
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"statuses": m.GetAllStatuses(),
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// LogsHandler returns connection logs.
func LogsHandler(w http.ResponseWriter, r *http.Request) {
	m := New()

	switch r.Method {
	case http.MethodGet:
		logs := m.GetLogs()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"logs": logs,
		})

	case http.MethodDelete:
		m.ClearLogs()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
