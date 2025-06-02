package main

import (
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
	return cmd.CombinedOutput()
}

// ExecuteWithTimeout executes a system command with timeout
func (e *SystemCommandExecutor) ExecuteWithTimeout(timeout time.Duration, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)

	// Set up timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			output, _ := cmd.CombinedOutput()
			return output, err
		}
		return cmd.CombinedOutput()
	case <-time.After(timeout):
		cmd.Process.Kill()
		return nil, fmt.Errorf("command timed out after %v", timeout)
	}
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

	// Bring interface down first (in case it's already up)
	if err := ism.bringInterfaceDown(ifName); err != nil {
		ism.logger.Printf("⚠️ Warning: failed to bring %s down: %v", ifName, err)
		// Continue anyway - interface might not have been up
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
		return false
	}
	return strings.Contains(string(output), ifName)
}

// bringInterfaceDown brings CAN interface down
func (ism *InterfaceSetupManager) bringInterfaceDown(ifName string) error {
	timeout := time.Duration(ism.config.TimeoutSeconds) * time.Second
	_, err := ism.commandExecutor.ExecuteWithTimeout(timeout, "ip", "link", "set", ifName, "down")
	return err
}

// configureInterface configures CAN interface parameters
func (ism *InterfaceSetupManager) configureInterface(ifName string) error {
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

	timeout := time.Duration(ism.config.TimeoutSeconds) * time.Second
	output, err := ism.commandExecutor.ExecuteWithTimeout(timeout, "ip", args...)

	if err != nil {
		return fmt.Errorf("configuration failed: %v, output: %s", err, string(output))
	}

	ism.logger.Printf("🔧 Configured %s: bitrate=%d, sample-point=%s, restart-ms=%d",
		ifName, ism.config.Bitrate, ism.config.SamplePoint, ism.config.RestartMs)

	return nil
}

// bringInterfaceUp brings CAN interface up
func (ism *InterfaceSetupManager) bringInterfaceUp(ifName string) error {
	timeout := time.Duration(ism.config.TimeoutSeconds) * time.Second
	output, err := ism.commandExecutor.ExecuteWithTimeout(timeout, "ip", "link", "set", ifName, "up")

	if err != nil {
		return fmt.Errorf("failed to bring interface up: %v, output: %s", err, string(output))
	}

	ism.logger.Printf("🚀 Interface %s brought up successfully", ifName)
	return nil
}

// verifyInterface verifies that the interface is working properly
func (ism *InterfaceSetupManager) verifyInterface(ifName string) error {
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

	if strings.Contains(state.State, "ERROR") {
		return fmt.Errorf("interface is in error state: %s", state.State)
	}

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
	state.IsUp = strings.Contains(output, "state UP")

	// Extract bitrate
	if match := regexp.MustCompile(`bitrate (\d+)`).FindStringSubmatch(output); len(match) > 1 {
		if bitrate, err := strconv.Atoi(match[1]); err == nil {
			state.Bitrate = bitrate
		}
	}

	// Extract state
	if match := regexp.MustCompile(`state (\w+(?:-\w+)*)`).FindStringSubmatch(output); len(match) > 1 {
		state.State = match[1]
	}

	// Extract restart-ms
	if match := regexp.MustCompile(`restart-ms (\d+)`).FindStringSubmatch(output); len(match) > 1 {
		if restartMs, err := strconv.Atoi(match[1]); err == nil {
			state.RestartMs = restartMs
		}
	}

	// Get error counters
	ism.parseErrorCounters(state, output)

	return state, nil
}

// parseErrorCounters extracts error counters from interface output
func (ism *InterfaceSetupManager) parseErrorCounters(state *InterfaceState, output string) {
	// This would need to parse actual CAN statistics
	// For now, we'll use basic parsing
	if match := regexp.MustCompile(`TX errors (\d+)`).FindStringSubmatch(output); len(match) > 1 {
		if txErrors, err := strconv.Atoi(match[1]); err == nil {
			state.TxErrors = txErrors
		}
	}

	if match := regexp.MustCompile(`RX errors (\d+)`).FindStringSubmatch(output); len(match) > 1 {
		if rxErrors, err := strconv.Atoi(match[1]); err == nil {
			state.RxErrors = rxErrors
		}
	}
}

// ResetInterface resets a CAN interface (down and up)
func (ism *InterfaceSetupManager) ResetInterface(ifName string) error {
	ism.logger.Printf("🔄 Resetting CAN interface %s", ifName)

	if err := ism.bringInterfaceDown(ifName); err != nil {
		return fmt.Errorf("failed to bring interface down: %w", err)
	}

	time.Sleep(100 * time.Millisecond) // Brief pause

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
