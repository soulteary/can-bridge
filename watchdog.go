package main

import (
	"context"
	"sync"
	"time"
)

// WatchdogConfig holds watchdog configuration
type WatchdogConfig struct {
	CheckInterval       time.Duration
	ErrorThreshold      time.Duration
	RecoveryEnabled     bool
	MaxRecoveryAttempts int
}

// DefaultWatchdogConfig returns default watchdog configuration
func DefaultWatchdogConfig() WatchdogConfig {
	return WatchdogConfig{
		CheckInterval:       10 * time.Second,
		ErrorThreshold:      30 * time.Second,
		RecoveryEnabled:     true,
		MaxRecoveryAttempts: 3,
	}
}

// Watchdog monitors and recovers CAN connections
type Watchdog struct {
	interfaceManager *InterfaceManager
	config           WatchdogConfig
	logger           Logger
	running          bool
	stopChan         chan struct{}
	wg               sync.WaitGroup
	mu               sync.RWMutex
	recoveryAttempts map[string]int
}

// NewWatchdog creates a new watchdog
func NewWatchdog(interfaceManager *InterfaceManager, config WatchdogConfig, logger Logger) *Watchdog {
	return &Watchdog{
		interfaceManager: interfaceManager,
		config:           config,
		logger:           logger,
		stopChan:         make(chan struct{}),
		recoveryAttempts: make(map[string]int),
	}
}

// Start starts the watchdog monitoring
func (w *Watchdog) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.mu.Unlock()

	w.logger.Printf("🐕 Starting CAN interface watchdog")

	w.wg.Add(1)
	go w.monitorLoop(ctx)

	return nil
}

// Stop stops the watchdog monitoring
func (w *Watchdog) Stop() error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = false
	w.mu.Unlock()

	close(w.stopChan)
	w.wg.Wait()

	w.logger.Printf("🐕 Watchdog stopped")
	return nil
}

// IsRunning returns whether the watchdog is running
func (w *Watchdog) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running
}

// monitorLoop is the main monitoring loop
func (w *Watchdog) monitorLoop(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Printf("🐕 Watchdog stopping due to context cancellation")
			return
		case <-w.stopChan:
			w.logger.Printf("🐕 Watchdog stopping due to stop signal")
			return
		case <-ticker.C:
			w.checkInterfaces()
		}
	}
}

// checkInterfaces checks all interfaces for health issues
func (w *Watchdog) checkInterfaces() {
	interfaces := w.interfaceManager.GetAllInterfaces()

	for ifName, canIf := range interfaces {
		if w.shouldCheckInterface(canIf) {
			if !w.interfaceManager.CheckHealth(ifName) {
				w.handleUnhealthyInterface(ifName)
			} else {
				// Reset recovery attempts on successful health check
				w.resetRecoveryAttempts(ifName)
			}
		}
	}
}

// shouldCheckInterface determines if an interface needs health checking
func (w *Watchdog) shouldCheckInterface(canIf *CanInterface) bool {
	stats := canIf.GetStats()

	// Skip health check if no errors or recent successful sends after errors
	if stats.LastErrorTime.IsZero() ||
		stats.LastSendTime.After(stats.LastErrorTime) ||
		time.Since(stats.LastErrorTime) >= w.config.ErrorThreshold {
		return false
	}

	return true
}

// handleUnhealthyInterface handles an unhealthy interface
func (w *Watchdog) handleUnhealthyInterface(ifName string) {
	if !w.config.RecoveryEnabled {
		w.logger.Printf("⚠️ %s interface appears down, but recovery is disabled", ifName)
		return
	}

	attempts := w.getRecoveryAttempts(ifName)
	if attempts >= w.config.MaxRecoveryAttempts {
		w.logger.Printf("❌ %s interface recovery failed after %d attempts, giving up", ifName, attempts)
		return
	}

	w.logger.Printf("🔄 %s interface appears down, attempting to reinitialize (attempt %d/%d)...",
		ifName, attempts+1, w.config.MaxRecoveryAttempts)

	if err := w.recoverInterface(ifName); err != nil {
		w.incrementRecoveryAttempts(ifName)
		w.logger.Printf("❌ %s reinitialization failed: %v", ifName, err)
	} else {
		w.resetRecoveryAttempts(ifName)
		w.logger.Printf("✅ %s interface successfully reinitialized", ifName)
	}
}

// recoverInterface attempts to recover a failed interface
func (w *Watchdog) recoverInterface(ifName string) error {
	// Remove the failed interface
	if err := w.interfaceManager.RemoveInterface(ifName); err != nil {
		w.logger.Printf("Warning: failed to remove interface %s: %v", ifName, err)
	}

	// Attempt to reinitialize
	return w.interfaceManager.InitializeSingle(ifName)
}

// getRecoveryAttempts gets the number of recovery attempts for an interface
func (w *Watchdog) getRecoveryAttempts(ifName string) int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.recoveryAttempts[ifName]
}

// incrementRecoveryAttempts increments recovery attempts for an interface
func (w *Watchdog) incrementRecoveryAttempts(ifName string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.recoveryAttempts[ifName]++
}

// resetRecoveryAttempts resets recovery attempts for an interface
func (w *Watchdog) resetRecoveryAttempts(ifName string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.recoveryAttempts, ifName)
}

// GetRecoveryStatus returns recovery status for all interfaces
func (w *Watchdog) GetRecoveryStatus() map[string]int {
	w.mu.RLock()
	defer w.mu.RUnlock()

	result := make(map[string]int)
	for k, v := range w.recoveryAttempts {
		result[k] = v
	}
	return result
}

// UpdateConfig updates watchdog configuration
func (w *Watchdog) UpdateConfig(config WatchdogConfig) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.config = config
}

// GetConfig returns current watchdog configuration
func (w *Watchdog) GetConfig() WatchdogConfig {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.config
}
