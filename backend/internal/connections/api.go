package connections

import (
	"encoding/json"
	"net/http"
)

// ConnectionsHandler handles connection status queries and connect/disconnect actions.
func ConnectionsHandler(w http.ResponseWriter, r *http.Request) {
	m := New()

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"statuses": m.GetAllStatuses(),
		})

	case http.MethodPost:
		var req struct {
			Service string `json:"service"` // "lidarr" | "discord" | "lastfm"
			Action  string `json:"action"` // "connect" | "disconnect"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		switch req.Service {
		case ServiceLidarr:
			switch req.Action {
			case "connect":
				go m.ConnectLidarr()
			case "disconnect":
				m.DisconnectLidarr()
			default:
				http.Error(w, "unknown action for lidarr: "+req.Action, http.StatusBadRequest)
				return
			}
		case ServiceDiscord:
			switch req.Action {
			case "connect":
				go m.ConnectDiscord()
			case "disconnect":
				m.DisconnectDiscord()
			default:
				http.Error(w, "unknown action for discord: "+req.Action, http.StatusBadRequest)
				return
			}
		case ServiceLastFM:
			switch req.Action {
			case "connect":
				m.ConnectLastFM()
			case "disconnect":
				m.DisconnectLastFM()
			default:
				http.Error(w, "unknown action for lastfm: "+req.Action, http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, "unknown service: "+req.Service, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"statuses": m.GetAllStatuses(),
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// LogsHandler returns or clears connection logs.
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
