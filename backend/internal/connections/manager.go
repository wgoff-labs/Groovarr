package connections

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/groovarr/groovarr/backend/internal/clients"
	"github.com/groovarr/groovarr/backend/internal/discord"
	"github.com/groovarr/groovarr/backend/internal/store"
)

// Status values
const (
	StatusDisconnected = "disconnected"
	StatusConnecting   = "connecting"
	StatusConnected    = "connected"
	StatusError        = "error"
)

// Service identifiers
const (
	ServiceLidarr  = "lidarr"
	ServiceDiscord = "discord"
	ServiceLastFM  = "lastfm"
)

// ServiceStatus describes the current state of one external service.
type ServiceStatus struct {
	Service   string    `json:"service"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	LastCheck time.Time `json:"last_check"`
}

// LogEntry records a connection event.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Service   string    `json:"service"`
	Level     string    `json:"level"` // "info", "error", "warn"
	Message   string    `json:"message"`
}

// Manager tracks the state of all external service connections.
type Manager struct {
	mu      sync.RWMutex
	lidarr  *clients.LidarrClient
	status  map[string]*ServiceStatus
	logs    []LogEntry
	maxLogs int
}

var global *Manager
var globalMu sync.Mutex

// New returns the global connection manager, creating it if needed.
func New() *Manager {
	globalMu.Lock()
	defer globalMu.Unlock()
	if global == nil {
		global = &Manager{
			status:  make(map[string]*ServiceStatus),
			logs:    make([]LogEntry, 0, 200),
			maxLogs: 200,
		}
		// Seed initial disconnected state for all known services
		for _, svc := range []string{ServiceLidarr, ServiceDiscord, ServiceLastFM} {
			global.status[svc] = &ServiceStatus{
				Service:   svc,
				Status:    StatusDisconnected,
				LastCheck: time.Now(),
			}
		}
	}
	return global
}

// Init attempts to connect to all services that have credentials in the database.
// Call this once at startup after the database has been initialised.
func Init() {
	m := New()
	m.log("info", "core", "Initialising connections from saved credentials...")

	// Lidarr
	lidarrURL, _ := store.SettingGet("lidarr_url")
	lidarrKey, _ := store.SettingGet("lidarr_api_key")
	if lidarrURL != "" && lidarrKey != "" {
		m.ConnectLidarr()
	}

	// Discord
	discordToken, _ := store.SettingGet("discord_token")
	if discordToken != "" {
		m.ConnectDiscord()
	}

	// LastFM — always "connected" if key is set (it's a read-only API)
	lastfmKey, _ := store.SettingGet("lastfm_api_key")
	if lastfmKey != "" {
		m.setStatus(ServiceLastFM, StatusConnected, "")
		m.log("info", ServiceLastFM, "Last.fm API key present")
	}
}

// ── Status helpers ─────────────────────────────────────────────────────────────

// GetStatus returns the status for a named service.
func (m *Manager) GetStatus(service string) *ServiceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.status[service]; ok {
		return s
	}
	return &ServiceStatus{Service: service, Status: StatusDisconnected, LastCheck: time.Now()}
}

// GetAllStatuses returns statuses for all tracked services.
func (m *Manager) GetAllStatuses() []*ServiceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*ServiceStatus, 0, len(m.status))
	for _, s := range m.status {
		result = append(result, s)
	}
	return result
}

func (m *Manager) setStatus(service, status, errMsg string) {
	m.mu.Lock()
	m.status[service] = &ServiceStatus{
		Service:   service,
		Status:    status,
		Error:     errMsg,
		LastCheck: time.Now(),
	}
	m.mu.Unlock()
}

// ── Lidarr ────────────────────────────────────────────────────────────────────

// ConnectLidarr attempts to connect to Lidarr using credentials from the database.
func (m *Manager) ConnectLidarr() {
	m.setStatus(ServiceLidarr, StatusConnecting, "")
	m.log("info", ServiceLidarr, "Connecting to Lidarr...")

	lidarrURL, _ := store.SettingGet("lidarr_url")
	lidarrKey, _ := store.SettingGet("lidarr_api_key")
	c, err := clients.NewLidarrClientWith(lidarrURL, lidarrKey)
	if err != nil {
		m.setStatus(ServiceLidarr, StatusError, err.Error())
		m.log("error", ServiceLidarr, fmt.Sprintf("Client creation failed: %v", err))
		return
	}

	// Verify by fetching root folders
	_, err = c.GetRootFolders()
	if err != nil {
		m.setStatus(ServiceLidarr, StatusError, err.Error())
		m.log("error", ServiceLidarr, fmt.Sprintf("Connection verified failed: %v", err))
		return
	}

	m.mu.Lock()
	m.lidarr = c
	m.mu.Unlock()
	m.setStatus(ServiceLidarr, StatusConnected, "")
	m.log("info", ServiceLidarr, "Connected to Lidarr successfully")
}

// DisconnectLidarr disconnects the Lidarr client.
func (m *Manager) DisconnectLidarr() {
	m.mu.Lock()
	m.lidarr = nil
	m.mu.Unlock()
	m.setStatus(ServiceLidarr, StatusDisconnected, "")
	m.log("info", ServiceLidarr, "Disconnected from Lidarr")
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

// ── Discord ────────────────────────────────────────────────────────────────────

// ConnectDiscord starts the Discord bot using the token from the database.
func (m *Manager) ConnectDiscord() {
	m.setStatus(ServiceDiscord, StatusConnecting, "")
	m.log("info", ServiceDiscord, "Connecting to Discord...")

	discordToken, _ := store.SettingGet("discord_token")
	if discordToken == "" {
		m.setStatus(ServiceDiscord, StatusError, "discord_token not set in settings")
		m.log("error", ServiceDiscord, "discord_token not set in settings")
		return
	}

	bot, err := discord.New(discordToken)
	if err != nil {
		m.setStatus(ServiceDiscord, StatusError, err.Error())
		m.log("error", ServiceDiscord, fmt.Sprintf("Discord bot creation failed: %v", err))
		return
	}

	if err := bot.Start(); err != nil {
		m.setStatus(ServiceDiscord, StatusError, err.Error())
		m.log("error", ServiceDiscord, fmt.Sprintf("Discord bot start failed: %v", err))
		return
	}

	discord.SetGlobalBot(bot)
	m.setStatus(ServiceDiscord, StatusConnected, "")
	m.log("info", ServiceDiscord, "Discord bot connected")
}

// DisconnectDiscord stops the Discord bot.
func (m *Manager) DisconnectDiscord() {
	if bot := discord.GetBot(); bot != nil {
		bot.Stop()
		discord.SetGlobalBot(nil)
	}
	m.setStatus(ServiceDiscord, StatusDisconnected, "")
	m.log("info", ServiceDiscord, "Discord bot disconnected")
}

// ── LastFM ────────────────────────────────────────────────────────────────────

// ConnectLastFM checks whether the LastFM API key is present.
// Since LastFM has no persistent connection, we just update the status.
func (m *Manager) ConnectLastFM() {
	key, _ := store.SettingGet("lastfm_api_key")
	if key == "" {
		m.setStatus(ServiceLastFM, StatusError, "lastfm_api_key not set")
		m.log("error", ServiceLastFM, "lastfm_api_key not set")
		return
	}
	m.setStatus(ServiceLastFM, StatusConnected, "")
	m.log("info", ServiceLastFM, "Last.fm API key present")
}

// DisconnectLastFM clears the LastFM status.
func (m *Manager) DisconnectLastFM() {
	m.setStatus(ServiceLastFM, StatusDisconnected, "")
	m.log("info", ServiceLastFM, "Last.fm disconnected")
}

// ── Logs ──────────────────────────────────────────────────────────────────────

// GetLogs returns all connection logs, newest first.
func (m *Manager) GetLogs() []LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	logs := make([]LogEntry, len(m.logs))
	copy(logs, m.logs)
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}
	return logs
}

// ClearLogs removes all log entries.
func (m *Manager) ClearLogs() {
	m.mu.Lock()
	m.logs = make([]LogEntry, 0, m.maxLogs)
	m.mu.Unlock()
}

func (m *Manager) log(level, service, msg string) {
	m.mu.Lock()
	entry := LogEntry{
		Timestamp: time.Now(),
		Service:   service,
		Level:     level,
		Message:   msg,
	}
	m.logs = append(m.logs, entry)
	if len(m.logs) > m.maxLogs {
		m.logs = m.logs[len(m.logs)-m.maxLogs:]
	}
	m.mu.Unlock()

	// Mirror to stdout for server-side log inspection
	if level == "error" {
		log.Printf("[conn:%s] ERROR: %s", service, msg)
	} else {
		log.Printf("[conn:%s] %s", service, msg)
	}
}

// GetManager is an alias for New, for use by other packages.
func GetManager() *Manager { return New() }
