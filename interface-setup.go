package main

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// InterfaceSetupConfig holds configuration for CAN interface setup
type InterfaceSetupConfig struct {
	Bitrate        int           `json:"bitrate"`
	SamplePoint    string        `json:"samplePoint,omitempty"`
	RestartMs      int           `json:"restartMs,omitempty"`
	AutoRecovery   bool          `json:"autoRecovery"`
	TimeoutSeconds int           `json:"timeoutSeconds"`
	RetryAttempts  int           `json:"retryAttempts"`
	RetryDelay     time.Duration `json:"retryDelay"`
}

// DefaultInterfaceSetupConfig returns default setup configuration
func DefaultInterfaceSetupConfig() InterfaceSetupConfig {
	return InterfaceSetupConfig{
		Bitrate:        1000000, // 1 Mbps
		SamplePoint:    "0.75",
		RestartMs:      100,
		AutoRecovery:   true,
		TimeoutSeconds: 10,
		RetryAttempts:  3,
		RetryDelay:     2 * time.Second,
	}
}

// InterfaceState represents the current state of a CAN interface
type InterfaceState struct {
	Name      string    `json:"name"`
	IsUp      bool      `json:"isUp"`
	Bitrate   int       `json:"bitrate"`
	State     string    `json:"state"` // UP, DOWN, ERROR-ACTIVE, etc.
	TxErrors  int       `json:"txErrors"`
	RxErrors  int       `json:"rxErrors"`
	RestartMs int       `json:"restartMs"`
	LastError string    `json:"lastError,omitempty"`
	SetupTime time.Time `json:"setupTime,omitempty"`
}

// CommandExecutor interface for dependency injection
type CommandExecutor interface {
	Execute(name string, args ...string) ([]byte, error)
	ExecuteWithTimeout(timeout time.Duration, name string, args ...string) ([]byte, error)
}

// SystemCommandExecutor implements CommandExecutor using real system commands
type SystemCommandExecutor struct{}

// NewSystemCommandExecutor creates a new system command executor
func NewSystemCommandExecutor() *SystemCommandExecutor {
	return &SystemCommandExecutor{}
}

// Execute executes a system command
func (e *SystemCommandExecutor) Execute(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	return output, err
}

// ExecuteWithTimeout executes a system command with timeout
func (e *SystemCommandExecutor) ExecuteWithTimeout(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	return output, err
}

// InterfaceSetupManager manages CAN interface setup and configuration
type InterfaceSetupManager struct {
	config          InterfaceSetupConfig
	commandExecutor CommandExecutor
	logger          Logger
}

// NewInterfaceSetupManager creates a new interface setup manager
func NewInterfaceSetupManager(config InterfaceSetupConfig, commandExecutor CommandExecutor, logger Logger) *InterfaceSetupManager {
	return &InterfaceSetupManager{
		config:          config,
		commandExecutor: commandExecutor,
		logger:          logger,
	}
}

// SetupInterface configures and brings up a CAN interface
func (ism *InterfaceSetupManager) SetupInterface(ifName string) error {
	ism.logger.Printf("🔧 Setting up CAN interface %s...", ifName)

	// First, check if interface exists
	if !ism.interfaceExists(ifName) {
		return fmt.Errorf("CAN interface %s does not exist", ifName)
	}

	// Get current state to see if interface is already up
	currentState, err := ism.GetInterfaceState(ifName)
	if err != nil {
		ism.logger.Printf("⚠️ Warning: could not get current state of %s: %v", ifName, err)
	}

	// If interface is already up and configured correctly, skip setup
	if currentState != nil && currentState.IsUp && currentState.Bitrate == ism.config.Bitrate {
		ism.logger.Printf("✅ Interface %s is already configured correctly (bitrate=%d)", ifName, currentState.Bitrate)
		return nil
	}

	// Bring interface down first (only if it's up)
	if currentState != nil && currentState.IsUp {
		if err := ism.bringInterfaceDown(ifName); err != nil {
			ism.logger.Printf("⚠️ Warning: failed to bring %s down: %v", ifName, err)
			// Try to force down
			if err := ism.forceInterfaceDown(ifName); err != nil {
				ism.logger.Printf("⚠️ Warning: failed to force %s down: %v", ifName, err)
			}
		}
		// Brief pause after bringing down
		time.Sleep(500 * time.Millisecond)
	}

	// Configure interface parameters
	if err := ism.configureInterface(ifName); err != nil {
		return fmt.Errorf("failed to configure %s: %w", ifName, err)
	}

	// Bring interface up
	if err := ism.bringInterfaceUp(ifName); err != nil {
		return fmt.Errorf("failed to bring %s up: %w", ifName, err)
	}

	// Verify interface is working
	if err := ism.verifyInterface(ifName); err != nil {
		return fmt.Errorf("interface %s verification failed: %w", ifName, err)
	}

	ism.logger.Printf("✅ CAN interface %s successfully configured and activated", ifName)
	return nil
}

// SetupInterfaceWithRetry sets up interface with retry logic
func (ism *InterfaceSetupManager) SetupInterfaceWithRetry(ifName string) error {
	var lastErr error

	for attempt := 1; attempt <= ism.config.RetryAttempts; attempt++ {
		err := ism.SetupInterface(ifName)
		if err == nil {
			return nil
		}

		lastErr = err
		ism.logger.Printf("❌ Setup attempt %d/%d failed for %s: %v",
			attempt, ism.config.RetryAttempts, ifName, err)

		if attempt < ism.config.RetryAttempts {
			ism.logger.Printf("⏳ Retrying in %v...", ism.config.RetryDelay)
			time.Sleep(ism.config.RetryDelay)
		}
	}

	return fmt.Errorf("failed to setup %s after %d attempts: %w",
		ifName, ism.config.RetryAttempts, lastErr)
}

// interfaceExists checks if a CAN interface exists in the system
func (ism *InterfaceSetupManager) interfaceExists(ifName string) bool {
	output, err := ism.commandExecutor.Execute("ip", "link", "show", ifName)
	if err != nil {
		ism.logger.Printf("🔍 Interface check failed for %s: %v", ifName, err)
		return false
	}
	exists := strings.Contains(string(output), ifName)
	ism.logger.Printf("🔍 Interface %s exists: %t", ifName, exists)
	return exists
}

// bringInterfaceDown brings CAN interface down
func (ism *InterfaceSetupManager) bringInterfaceDown(ifName string) error {
	ism.logger.Printf("🔽 Bringing %s down...", ifName)
	timeout := time.Duration(ism.config.TimeoutSeconds) * time.Second
	output, err := ism.commandExecutor.ExecuteWithTimeout(timeout, "ip", "link", "set", ifName, "down")
	if err != nil {
		ism.logger.Printf("❌ Failed to bring %s down: %v, output: %s", ifName, err, string(output))
		return err
	}
	ism.logger.Printf("✅ Successfully brought %s down", ifName)
	return nil
}

// forceInterfaceDown forces interface down using different approach
func (ism *InterfaceSetupManager) forceInterfaceDown(ifName string) error {
	ism.logger.Printf("🔽 Force bringing %s down...", ifName)

	// Try using ifconfig as alternative
	timeout := time.Duration(ism.config.TimeoutSeconds) * time.Second
	output, err := ism.commandExecutor.ExecuteWithTimeout(timeout, "ifconfig", ifName, "down")
	if err != nil {
		ism.logger.Printf("❌ Failed to force %s down with ifconfig: %v, output: %s", ifName, err, string(output))
		return err
	}
	ism.logger.Printf("✅ Successfully forced %s down with ifconfig", ifName)
	return nil
}

// configureInterface configures CAN interface parameters
func (ism *InterfaceSetupManager) configureInterface(ifName string) error {
	ism.logger.Printf("⚙️ Configuring %s parameters...", ifName)

	args := []string{"link", "set", ifName, "type", "can"}

	// Add bitrate
	args = append(args, "bitrate", strconv.Itoa(ism.config.Bitrate))

	// Add sample point if specified
	if ism.config.SamplePoint != "" {
		args = append(args, "sample-point", ism.config.SamplePoint)
	}

	// Add restart-ms if specified
	if ism.config.RestartMs > 0 {
		args = append(args, "restart-ms", strconv.Itoa(ism.config.RestartMs))
	}

	ism.logger.Printf("📝 Executing: ip %s", strings.Join(args, " "))

	timeout := time.Duration(ism.config.TimeoutSeconds) * time.Second
	output, err := ism.commandExecutor.ExecuteWithTimeout(timeout, "ip", args...)

	if err != nil {
		ism.logger.Printf("❌ Configuration failed for %s: %v, output: %s", ifName, err, string(output))
		return fmt.Errorf("configuration failed: %v, output: %s", err, string(output))
	}

	ism.logger.Printf("✅ Successfully configured %s: bitrate=%d, sample-point=%s, restart-ms=%d",
		ifName, ism.config.Bitrate, ism.config.SamplePoint, ism.config.RestartMs)

	return nil
}

// bringInterfaceUp brings CAN interface up
func (ism *InterfaceSetupManager) bringInterfaceUp(ifName string) error {
	ism.logger.Printf("🚀 Bringing %s up...", ifName)
	timeout := time.Duration(ism.config.TimeoutSeconds) * time.Second
	output, err := ism.commandExecutor.ExecuteWithTimeout(timeout, "ip", "link", "set", ifName, "up")

	if err != nil {
		ism.logger.Printf("❌ Failed to bring %s up: %v, output: %s", ifName, err, string(output))
		return fmt.Errorf("failed to bring interface up: %v, output: %s", err, string(output))
	}

	ism.logger.Printf("✅ Successfully brought %s up", ifName)
	return nil
}

// verifyInterface verifies that the interface is working properly
func (ism *InterfaceSetupManager) verifyInterface(ifName string) error {
	ism.logger.Printf("🔍 Verifying %s configuration...", ifName)

	state, err := ism.GetInterfaceState(ifName)
	if err != nil {
		return fmt.Errorf("failed to get interface state: %w", err)
	}

	if !state.IsUp {
		return fmt.Errorf("interface is not up")
	}

	if state.Bitrate != ism.config.Bitrate {
		return fmt.Errorf("bitrate mismatch: expected %d, got %d",
			ism.config.Bitrate, state.Bitrate)
	}

	if strings.Contains(strings.ToUpper(state.State), "ERROR") && !strings.Contains(strings.ToUpper(state.State), "ERROR-ACTIVE") {
		return fmt.Errorf("interface is in error state: %s", state.State)
	}

	ism.logger.Printf("✅ Interface %s verification passed: up=%t, bitrate=%d, state=%s",
		ifName, state.IsUp, state.Bitrate, state.State)

	return nil
}

// GetInterfaceState gets current state of a CAN interface
func (ism *InterfaceSetupManager) GetInterfaceState(ifName string) (*InterfaceState, error) {
	output, err := ism.commandExecutor.Execute("ip", "-details", "link", "show", ifName)
	if err != nil {
		return nil, fmt.Errorf("failed to get interface details: %w", err)
	}

	return ism.parseInterfaceState(ifName, string(output))
}

// parseInterfaceState parses interface state from ip command output
func (ism *InterfaceSetupManager) parseInterfaceState(ifName, output string) (*InterfaceState, error) {
	state := &InterfaceState{
		Name: ifName,
	}

	// Check if interface is UP
	if strings.Contains(output, "state UP") {
		state.IsUp = true
	} else if strings.Contains(output, "state DOWN") {
		state.IsUp = false
	}

	// Extract more detailed state information
	if match := regexp.MustCompile(`state (\w+(?:-\w+)*)`).FindStringSubmatch(output); len(match) > 1 {
		state.State = match[1]
	}

	// Extract bitrate
	if match := regexp.MustCompile(`bitrate (\d+)`).FindStringSubmatch(output); len(match) > 1 {
		if bitrate, err := strconv.Atoi(match[1]); err == nil {
			state.Bitrate = bitrate
		}
	}

	// Extract restart-ms
	if match := regexp.MustCompile(`restart-ms (\d+)`).FindStringSubmatch(output); len(match) > 1 {
		if restartMs, err := strconv.Atoi(match[1]); err == nil {
			state.RestartMs = restartMs
		}
	}

	// Get additional CAN statistics if available
	ism.getCanStatistics(state, ifName)

	return state, nil
}

// getCanStatistics gets additional CAN statistics
func (ism *InterfaceSetupManager) getCanStatistics(state *InterfaceState, ifName string) {
	// Try to get CAN statistics from /proc/net/can/stats
	output, err := ism.commandExecutor.Execute("cat", fmt.Sprintf("/proc/net/can/stats/%s", ifName))
	if err == nil {
		ism.parseCanStatistics(state, string(output))
	}

	// Try alternative approach with ip -s link show
	output, err = ism.commandExecutor.Execute("ip", "-s", "link", "show", ifName)
	if err == nil {
		ism.parseIpStatistics(state, string(output))
	}
}

// parseCanStatistics parses CAN-specific statistics
func (ism *InterfaceSetupManager) parseCanStatistics(state *InterfaceState, output string) {
	// Parse TX errors
	if match := regexp.MustCompile(`bus_error_tx:\s*(\d+)`).FindStringSubmatch(output); len(match) > 1 {
		if txErrors, err := strconv.Atoi(match[1]); err == nil {
			state.TxErrors = txErrors
		}
	}

	// Parse RX errors
	if match := regexp.MustCompile(`bus_error_rx:\s*(\d+)`).FindStringSubmatch(output); len(match) > 1 {
		if rxErrors, err := strconv.Atoi(match[1]); err == nil {
			state.RxErrors = rxErrors
		}
	}
}

// parseIpStatistics parses statistics from ip command output
func (ism *InterfaceSetupManager) parseIpStatistics(state *InterfaceState, output string) {
	// Look for error counters in ip -s output
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if strings.Contains(line, "RX:") && i+1 < len(lines) {
			// Next line should contain error stats
			if match := regexp.MustCompile(`\d+\s+\d+\s+(\d+)`).FindStringSubmatch(lines[i+1]); len(match) > 1 {
				if rxErrors, err := strconv.Atoi(match[1]); err == nil {
					state.RxErrors = rxErrors
				}
			}
		}
		if strings.Contains(line, "TX:") && i+1 < len(lines) {
			// Next line should contain error stats
			if match := regexp.MustCompile(`\d+\s+\d+\s+(\d+)`).FindStringSubmatch(lines[i+1]); len(match) > 1 {
				if txErrors, err := strconv.Atoi(match[1]); err == nil {
					state.TxErrors = txErrors
				}
			}
		}
	}
}

// ResetInterface resets a CAN interface (down and up)
func (ism *InterfaceSetupManager) ResetInterface(ifName string) error {
	ism.logger.Printf("🔄 Resetting CAN interface %s", ifName)

	if err := ism.bringInterfaceDown(ifName); err != nil {
		return fmt.Errorf("failed to bring interface down: %w", err)
	}

	time.Sleep(500 * time.Millisecond) // Brief pause

	if err := ism.bringInterfaceUp(ifName); err != nil {
		return fmt.Errorf("failed to bring interface up: %w", err)
	}

	ism.logger.Printf("✅ Interface %s reset successfully", ifName)
	return nil
}

// TeardownInterface brings down a CAN interface
func (ism *InterfaceSetupManager) TeardownInterface(ifName string) error {
	ism.logger.Printf("🔽 Tearing down CAN interface %s", ifName)

	if err := ism.bringInterfaceDown(ifName); err != nil {
		return fmt.Errorf("failed to teardown interface: %w", err)
	}

	ism.logger.Printf("✅ Interface %s teardown complete", ifName)
	return nil
}

// GetAvailableInterfaces returns list of available CAN interfaces in the system
func (ism *InterfaceSetupManager) GetAvailableInterfaces() ([]string, error) {
	output, err := ism.commandExecutor.Execute("ip", "link", "show", "type", "can")
	if err != nil {
		return nil, fmt.Errorf("failed to list CAN interfaces: %w", err)
	}

	var interfaces []string
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if match := regexp.MustCompile(`^\d+:\s+(\w+):`).FindStringSubmatch(line); len(match) > 1 {
			interfaces = append(interfaces, match[1])
		}
	}

	ism.logger.Printf("🔍 Found %d CAN interfaces: %v", len(interfaces), interfaces)
	return interfaces, nil
}

// ValidateSetupConfig validates the setup configuration
func (ism *InterfaceSetupManager) ValidateSetupConfig() error {
	if ism.config.Bitrate <= 0 {
		return fmt.Errorf("bitrate must be positive")
	}

	if ism.config.TimeoutSeconds <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	if ism.config.RetryAttempts <= 0 {
		return fmt.Errorf("retry attempts must be positive")
	}

	if ism.config.SamplePoint != "" {
		if point, err := strconv.ParseFloat(ism.config.SamplePoint, 64); err != nil || point <= 0 || point >= 1 {
			return fmt.Errorf("sample point must be between 0 and 1")
		}
	}

	return nil
}

// GetSetupConfig returns current setup configuration
func (ism *InterfaceSetupManager) GetSetupConfig() InterfaceSetupConfig {
	return ism.config
}

// UpdateSetupConfig updates the setup configuration
func (ism *InterfaceSetupManager) UpdateSetupConfig(config InterfaceSetupConfig) error {
	ism.config = config
	return ism.ValidateSetupConfig()
}
