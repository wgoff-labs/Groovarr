package connections

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/groovarr/groovarr/backend/internal/clients"
	"github.com/groovarr/groovarr/backend/internal/store"
)

// Status values
const (
	StatusDisconnected = "disconnected"
	StatusConnecting   = "connecting"
	StatusConnected    = "connected"
	StatusError       = "error"
)

// Service statuses
type ServiceStatus struct {
	Service   string    `json:"service"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	LastCheck time.Time `json:"last_check"`
}

// LogEntry for connection events
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Service   string    `json:"service"`
	Level     string    `json:"level"` // "info", "error", "warn"
	Message   string    `json:"message"`
}

// Manager manages external service connections.
type Manager struct {
	mu       sync.RWMutex
	lidarr   *clients.LidarrClient
	status   map[string]*ServiceStatus
	logs     []LogEntry
	maxLogs  int
}

var global *Manager
var globalMu sync.Mutex

// New creates (or returns) the global connection manager.
func New() *Manager {
	globalMu.Lock()
	defer globalMu.Unlock()
	if global == nil {
		global = &Manager{
			status:  make(map[string]*ServiceStatus),
			logs:    make([]LogEntry, 0, 200),
			maxLogs: 200,
		}
		// Seed initial disconnected state
		global.status["lidarr"] = &ServiceStatus{
			Service:   "lidarr",
			Status:    StatusDisconnected,
			LastCheck: time.Now(),
		}
	}
	return global
}

// Init attempts to connect to all services that have credentials in the database.
// Call this once at startup after config has been loaded from DB.
func Init() {
	m := New()
	m.log("info", "lidarr", "Initializing connections from saved credentials...")

	lidarrURL, _ := store.SettingGet("lidarr_url")
	lidarrKey, _ := store.SettingGet("lidarr_api_key")
	if lidarrURL != "" && lidarrKey != "" {
		m.ConnectLidarr()
	}
}

// GetStatus returns the current status for a service.
func (m *Manager) GetStatus(service string) *ServiceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.status[service]; ok {
		return s
	}
	return &ServiceStatus{Service: service, Status: StatusDisconnected, LastCheck: time.Now()}
}

// GetAllStatuses returns all service statuses.
func (m *Manager) GetAllStatuses() []*ServiceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*ServiceStatus, 0, len(m.status))
	for _, s := range m.status {
		result = append(result, s)
	}
	return result
}

// ConnectLidarr attempts to connect to Lidarr using stored credentials.
func (m *Manager) ConnectLidarr() {
	m.mu.Lock()
	m.status["lidarr"] = &ServiceStatus{Service: "lidarr", Status: StatusConnecting, LastCheck: time.Now()}
	m.mu.Unlock()

	m.log("info", "lidarr", "Connecting to Lidarr...")

	lidarrURL, _ := store.SettingGet("lidarr_url")
	lidarrKey, _ := store.SettingGet("lidarr_api_key")
	c, err := clients.NewLidarrClientWith(lidarrURL, lidarrKey)
	if err != nil {
		m.mu.Lock()
		m.status["lidarr"] = &ServiceStatus{
			Service:   "lidarr",
			Status:    StatusError,
			Error:     err.Error(),
			LastCheck: time.Now(),
		}
		m.mu.Unlock()
		m.log("error", "lidarr", fmt.Sprintf("Connection failed: %v", err))
		return
	}

	// Verify with a test call
	_, err = c.GetRootFolders()
	if err != nil {
		m.mu.Lock()
		m.status["lidarr"] = &ServiceStatus{
			Service:   "lidarr",
			Status:    StatusError,
			Error:     err.Error(),
			LastCheck: time.Now(),
		}
		m.mu.Unlock()
		m.log("error", "lidarr", fmt.Sprintf("Connection verified failed: %v", err))
		return
	}

	m.mu.Lock()
	m.lidarr = c
	m.status["lidarr"] = &ServiceStatus{
		Service:   "lidarr",
		Status:    StatusConnected,
		LastCheck: time.Now(),
	}
	m.mu.Unlock()
	m.log("info", "lidarr", "Connected to Lidarr successfully")

	// Persist connection state
	store.SettingSet("lidarr_connected", "true")
}

// DisconnectLidarr disconnects from Lidarr.
func (m *Manager) DisconnectLidarr() {
	m.mu.Lock()
	if m.lidarr != nil {
		m.lidarr = nil
	}
	m.status["lidarr"] = &ServiceStatus{
		Service:   "lidarr",
		Status:    StatusDisconnected,
		LastCheck: time.Now(),
	}
	m.mu.Unlock()
	m.log("info", "lidarr", "Disconnected from Lidarr")
	store.SettingSet("lidarr_connected", "false")
}

// GetLidarrClient returns the active Lidarr client if connected.
func (m *Manager) GetLidarrClient() (*clients.LidarrClient, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.lidarr == nil {
		return nil, fmt.Errorf("lidarr not connected")
	}
	return m.lidarr, nil
}

// GetLogs returns all connection logs, newest first.
func (m *Manager) GetLogs() []LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	logs := make([]LogEntry, len(m.logs))
	copy(logs, m.logs)
	// Return newest first
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}
	return logs
}

// ClearLogs clears all connection logs.
func (m *Manager) ClearLogs() {
	m.mu.Lock()
	m.logs = make([]LogEntry, 0, m.maxLogs)
	m.mu.Unlock()
}

func (m *Manager) log(level, service, msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := LogEntry{
		Timestamp: time.Now(),
		Service:   service,
		Level:     level,
		Message:   msg,
	}
	m.logs = append(m.logs, entry)
	// Trim to max size
	if len(m.logs) > m.maxLogs {
		m.logs = m.logs[len(m.logs)-m.maxLogs:]
	}
	// Also print to stdout for server logs
	switch level {
	case "error":
		log.Printf("[conn] ERROR lidarr: %s", msg)
	default:
		log.Printf("[conn] %s: %s", service, msg)
	}
}

// GetManager returns the global connection manager, creating it if necessary.
func GetManager() *Manager {
	return New()
}
