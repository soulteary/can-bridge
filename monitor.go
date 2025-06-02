package main

import (
	"fmt"
	"time"
)

// SystemStatus represents overall system status
type SystemStatus struct {
	Interfaces          map[string]InterfaceStatus `json:"interfaces"`
	ActiveInterfaces    int                        `json:"activeInterfaces"`
	ConfiguredPorts     []string                   `json:"configuredPorts"`
	AvailableInterfaces []string                   `json:"availableInterfaces"`
	WatchdogStatus      WatchdogStatus             `json:"watchdogStatus"`
	SystemUptime        time.Duration              `json:"systemUptime"`
	Timestamp           time.Time                  `json:"timestamp"`
}

// InterfaceStatus represents the status of a single interface
type InterfaceStatus struct {
	Name          string       `json:"name"`
	Active        bool         `json:"active"`
	Uptime        string       `json:"uptime"`
	TotalSent     uint64       `json:"totalSent"`
	TotalErrors   uint64       `json:"totalErrors"`
	SuccessRate   string       `json:"successRate"`
	LastSendTime  time.Time    `json:"lastSendTime"`
	LastErrorTime time.Time    `json:"lastErrorTime"`
	LastErrorMsg  string       `json:"lastErrorMsg"`
	AvgLatency    string       `json:"avgLatency"`
	Health        HealthStatus `json:"health"`
}

// HealthStatus represents health information
type HealthStatus struct {
	Status       string    `json:"status"` // "healthy", "warning", "critical"
	LastCheck    time.Time `json:"lastCheck"`
	ChecksPassed int       `json:"checksPassed"`
	ChecksFailed int       `json:"checksFailed"`
}

// WatchdogStatus represents watchdog status
type WatchdogStatus struct {
	Running          bool           `json:"running"`
	CheckInterval    time.Duration  `json:"checkInterval"`
	RecoveryEnabled  bool           `json:"recoveryEnabled"`
	RecoveryAttempts map[string]int `json:"recoveryAttempts"`
	LastCheck        time.Time      `json:"lastCheck"`
}

// Monitor handles system monitoring and status reporting
type Monitor struct {
	interfaceManager *InterfaceManager
	watchdog         *Watchdog
	configProvider   ConfigProvider
	startTime        time.Time
	healthChecks     map[string]*HealthTracker
}

// HealthTracker tracks health check results for an interface
type HealthTracker struct {
	ChecksPassed int
	ChecksFailed int
	LastCheck    time.Time
}

// NewMonitor creates a new monitor
func NewMonitor(interfaceManager *InterfaceManager, watchdog *Watchdog, configProvider ConfigProvider) *Monitor {
	return &Monitor{
		interfaceManager: interfaceManager,
		watchdog:         watchdog,
		configProvider:   configProvider,
		startTime:        time.Now(),
		healthChecks:     make(map[string]*HealthTracker),
	}
}

// GetSystemStatus returns complete system status
func (m *Monitor) GetSystemStatus() SystemStatus {
	interfaces := m.getInterfaceStatuses()

	return SystemStatus{
		Interfaces:          interfaces,
		ActiveInterfaces:    m.interfaceManager.GetInterfaceCount(),
		ConfiguredPorts:     m.configProvider.GetCanPorts(),
		AvailableInterfaces: m.getAvailableInterfaces(),
		WatchdogStatus:      m.getWatchdogStatus(),
		SystemUptime:        time.Since(m.startTime),
		Timestamp:           time.Now(),
	}
}

// getInterfaceStatuses returns status for all interfaces
func (m *Monitor) getInterfaceStatuses() map[string]InterfaceStatus {
	result := make(map[string]InterfaceStatus)
	interfaces := m.interfaceManager.GetAllInterfaces()

	for name, canIf := range interfaces {
		stats := canIf.GetStats()
		health := m.checkInterfaceHealth(name)

		result[name] = InterfaceStatus{
			Name:          name,
			Active:        true,
			Uptime:        stats.Uptime.String(),
			TotalSent:     stats.TotalSent,
			TotalErrors:   stats.TotalErrors,
			SuccessRate:   fmt.Sprintf("%.2f%%", stats.SuccessRate()),
			LastSendTime:  stats.LastSendTime,
			LastErrorTime: stats.LastErrorTime,
			LastErrorMsg:  stats.LastErrorMsg,
			AvgLatency:    stats.AvgLatency.String(),
			Health:        health,
		}
	}

	// Add configured but inactive interfaces
	for _, port := range m.configProvider.GetCanPorts() {
		if _, exists := result[port]; !exists {
			result[port] = InterfaceStatus{
				Name:   port,
				Active: false,
				Health: HealthStatus{
					Status:    "critical",
					LastCheck: time.Now(),
				},
			}
		}
	}

	return result
}

// checkInterfaceHealth performs health check and updates tracker
func (m *Monitor) checkInterfaceHealth(ifName string) HealthStatus {
	// Get or create health tracker
	tracker, exists := m.healthChecks[ifName]
	if !exists {
		tracker = &HealthTracker{}
		m.healthChecks[ifName] = tracker
	}

	// Perform health check
	isHealthy := m.interfaceManager.CheckHealth(ifName)
	tracker.LastCheck = time.Now()

	if isHealthy {
		tracker.ChecksPassed++
	} else {
		tracker.ChecksFailed++
	}

	// Determine health status
	status := m.determineHealthStatus(tracker)

	return HealthStatus{
		Status:       status,
		LastCheck:    tracker.LastCheck,
		ChecksPassed: tracker.ChecksPassed,
		ChecksFailed: tracker.ChecksFailed,
	}
}

// determineHealthStatus determines health status based on check history
func (m *Monitor) determineHealthStatus(tracker *HealthTracker) string {
	total := tracker.ChecksPassed + tracker.ChecksFailed
	if total == 0 {
		return "unknown"
	}

	successRate := float64(tracker.ChecksPassed) / float64(total)

	switch {
	case successRate >= 0.95:
		return "healthy"
	case successRate >= 0.80:
		return "warning"
	default:
		return "critical"
	}
}

// getWatchdogStatus returns watchdog status
func (m *Monitor) getWatchdogStatus() WatchdogStatus {
	config := m.watchdog.GetConfig()

	return WatchdogStatus{
		Running:          m.watchdog.IsRunning(),
		CheckInterval:    config.CheckInterval,
		RecoveryEnabled:  config.RecoveryEnabled,
		RecoveryAttempts: m.watchdog.GetRecoveryStatus(),
		LastCheck:        time.Now(), // This could be enhanced to track actual last check
	}
}

// getAvailableInterfaces returns list of available interface names
func (m *Monitor) getAvailableInterfaces() []string {
	return m.configProvider.GetCanPorts()
}

// GetInterfaceStatus returns status for a specific interface
func (m *Monitor) GetInterfaceStatus(ifName string) (InterfaceStatus, error) {
	statuses := m.getInterfaceStatuses()
	status, exists := statuses[ifName]
	if !exists {
		return InterfaceStatus{}, fmt.Errorf("interface %s not found", ifName)
	}
	return status, nil
}

// GetHealthSummary returns a summary of system health
func (m *Monitor) GetHealthSummary() map[string]interface{} {
	status := m.GetSystemStatus()

	healthySummary := map[string]int{
		"healthy":  0,
		"warning":  0,
		"critical": 0,
		"unknown":  0,
	}

	for _, ifStatus := range status.Interfaces {
		healthySummary[ifStatus.Health.Status]++
	}

	overallHealth := "healthy"
	if healthySummary["critical"] > 0 {
		overallHealth = "critical"
	} else if healthySummary["warning"] > 0 {
		overallHealth = "warning"
	}

	return map[string]interface{}{
		"overallHealth":      overallHealth,
		"totalInterfaces":    len(status.Interfaces),
		"activeInterfaces":   status.ActiveInterfaces,
		"healthDistribution": healthySummary,
		"systemUptime":       status.SystemUptime.String(),
		"watchdogActive":     status.WatchdogStatus.Running,
	}
}

// ResetHealthTracking resets health tracking for an interface
func (m *Monitor) ResetHealthTracking(ifName string) {
	delete(m.healthChecks, ifName)
}

// ResetAllHealthTracking resets health tracking for all interfaces
func (m *Monitor) ResetAllHealthTracking() {
	m.healthChecks = make(map[string]*HealthTracker)
}
