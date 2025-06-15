package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

// Service represents the main CAN communication service
type Service struct {
	config           *Config
	configProvider   ConfigProvider
	setupManager     *InterfaceSetupManager
	interfaceManager *InterfaceManager
	messageSender    *MessageSender
	messageListener  *CanMessageListener
	watchdog         *Watchdog
	monitor          *Monitor
	apiHandler       *APIHandler
	server           *http.Server
	logger           Logger
}

// NewService creates a new CAN communication service
func NewService() *Service {
	return &Service{
		logger: &DefaultLogger{},
	}
}

// Initialize initializes all service components
func (s *Service) Initialize() error {
	// Parse configuration
	configParser := NewConfigParser()
	config, err := configParser.ParseConfig()
	if err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Validate configuration
	if err := configParser.ValidateConfig(config); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	s.config = config
	s.configProvider = NewDefaultConfigProvider(config)

	s.logger.Printf("🚀 Starting CAN Communication Service")
	s.logger.Printf("📋 Configuration:")
	s.logger.Printf("   - CAN Ports: %v", config.CanPorts)
	s.logger.Printf("   - Server Port: %s", config.Port)

	// Initialize components
	if err := s.initializeComponents(); err != nil {
		return fmt.Errorf("failed to initialize components: %w", err)
	}

	// Setup CAN interfaces (new step)
	if err := s.setupCanInterfaces(); err != nil {
		s.logger.Printf("Warning: CAN interface setup issues: %v", err)
		// We continue even if some interfaces failed to setup
	}

	// Initialize CAN interfaces
	if err := s.interfaceManager.InitializeAll(); err != nil {
		s.logger.Printf("Warning: %v", err)
		// We continue even if some interfaces failed
	}

	// Start message listening for all active interfaces
	if err := s.startMessageListening(); err != nil {
		s.logger.Printf("Warning: message listening issues: %v", err)
		// We continue even if some listeners failed to start
	}

	// Setup HTTP server
	s.setupHTTPServer()

	return nil
}

// initializeComponents initializes all service components
func (s *Service) initializeComponents() error {
	// Create command executor for interface setup
	commandExecutor := NewSystemCommandExecutor()

	// Create interface setup manager
	setupConfig := DefaultInterfaceSetupConfig()
	s.setupManager = NewInterfaceSetupManager(setupConfig, commandExecutor, s.logger)

	// Validate setup configuration
	if err := s.setupManager.ValidateSetupConfig(); err != nil {
		return fmt.Errorf("setup configuration validation failed: %w", err)
	}

	// Create socket provider
	socketProvider := NewUnixSocketProvider()

	// Create interface manager
	s.interfaceManager = NewInterfaceManager(s.configProvider, socketProvider, s.logger)

	// Create message sender
	s.messageSender = NewMessageSender(s.interfaceManager, s.configProvider, socketProvider, s.logger)

	// Create message listener (new component)
	maxMessages := 100 // Configure maximum messages per interface
	s.messageListener = NewCanMessageListener(maxMessages, s.logger)

	// Create watchdog
	watchdogConfig := DefaultWatchdogConfig()
	s.watchdog = NewWatchdog(s.interfaceManager, watchdogConfig, s.logger)

	// Create monitor
	s.monitor = NewMonitor(s.interfaceManager, s.watchdog, s.configProvider)

	// Create API handler with setup manager and message listener
	s.apiHandler = NewAPIHandlerWithSetupAndListener(
		s.messageSender,
		s.monitor,
		s.setupManager,
		s.messageListener,
		s.logger,
	)

	return nil
}

// setupCanInterfaces sets up all configured CAN interfaces
func (s *Service) setupCanInterfaces() error {
	s.logger.Printf("🔧 Setting up CAN interfaces...")

	// Get available interfaces first
	available, err := s.setupManager.GetAvailableInterfaces()
	if err != nil {
		s.logger.Printf("⚠️ Warning: could not list available interfaces: %v", err)
	} else {
		s.logger.Printf("📡 Available CAN interfaces: %v", available)
	}

	var setupErrors []string
	successCount := 0

	for _, ifName := range s.config.CanPorts {
		s.logger.Printf("🔧 Setting up interface %s...", ifName)

		err := s.setupManager.SetupInterfaceWithRetry(ifName)
		if err != nil {
			setupErrors = append(setupErrors, fmt.Sprintf("%s: %v", ifName, err))
			s.logger.Printf("❌ Failed to setup %s: %v", ifName, err)
		} else {
			successCount++
			s.logger.Printf("✅ Successfully set up %s", ifName)

			// Verify interface state
			if state, err := s.setupManager.GetInterfaceState(ifName); err == nil {
				s.logger.Printf("📊 %s state: bitrate=%d, state=%s, up=%t",
					ifName, state.Bitrate, state.State, state.IsUp)
			}
		}
	}

	if successCount == 0 {
		return fmt.Errorf("failed to setup any CAN interfaces: %v", setupErrors)
	}

	s.logger.Printf("🎯 Successfully set up %d/%d CAN interfaces", successCount, len(s.config.CanPorts))

	if len(setupErrors) > 0 {
		return fmt.Errorf("partial setup failure: %v", setupErrors)
	}

	return nil
}

// startMessageListening starts message listening for all active interfaces
func (s *Service) startMessageListening() error {
	s.logger.Printf("👂 Starting message listening for active interfaces...")

	var listeningErrors []string
	successCount := 0

	// Get all active interfaces from interface manager
	activeInterfaces := s.interfaceManager.GetAllInterfaces()

	for ifName := range activeInterfaces {
		s.logger.Printf("👂 Starting listener for %s...", ifName)

		err := s.messageListener.StartListening(ifName)
		if err != nil {
			listeningErrors = append(listeningErrors, fmt.Sprintf("%s: %v", ifName, err))
			s.logger.Printf("❌ Failed to start listening on %s: %v", ifName, err)
		} else {
			successCount++
			s.logger.Printf("✅ Successfully started listening on %s", ifName)
		}
	}

	// Also try to start listening on configured ports that might become active later
	for _, ifName := range s.config.CanPorts {
		// Skip if already handled above
		if _, exists := activeInterfaces[ifName]; exists {
			continue
		}

		s.logger.Printf("👂 Attempting to start listener for configured interface %s...", ifName)
		err := s.messageListener.StartListening(ifName)
		if err != nil {
			s.logger.Printf("⚠️ Warning: could not start listening on %s (interface may not be ready): %v", ifName, err)
		} else {
			successCount++
			s.logger.Printf("✅ Successfully started listening on %s", ifName)
		}
	}

	s.logger.Printf("🎯 Successfully started listening on %d interfaces", successCount)

	if len(listeningErrors) > 0 {
		return fmt.Errorf("partial listening startup failure: %v", listeningErrors)
	}

	return nil
}

// setupHTTPServer configures the HTTP server
func (s *Service) setupHTTPServer() {
	// Set to production mode
	gin.SetMode(gin.ReleaseMode)

	// Create Gin engine with custom middleware
	r := gin.New()
	r.Use(RecoveryMiddleware(s.logger))
	r.Use(LoggingMiddleware(s.logger))
	r.Use(CORSMiddleware())

	// Setup API routes
	s.apiHandler.SetupRoutes(r)

	// Create HTTP server with timeouts
	serverAddr := ":" + s.config.Port
	s.server = &http.Server{
		Addr:         serverAddr,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	s.logger.Printf("🌐 CAN Communication Service will run at http://localhost%s", serverAddr)
}

// Start starts the service
func (s *Service) Start(ctx context.Context) error {
	// Start watchdog
	if s.config.EnableHealthCheck {
		if err := s.watchdog.Start(ctx); err != nil {
			return fmt.Errorf("failed to start watchdog: %w", err)
		}
	}

	// Start Node Finder in a separate goroutine
	if s.config.EnableFinder {
		go NodeFinder(s.config.SetupFinderInterval)
	}

	// Start HTTP server in a goroutine
	go func() {
		s.logger.Printf("🌐 Starting HTTP server on %s", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("❌ HTTP server error: %v", err)
		}
	}()

	s.logger.Printf("✅ CAN Communication Service started successfully")
	s.logger.Printf("📡 Message listening active on: %v", s.messageListener.GetListeningInterfaces())
	return nil
}

// Stop gracefully stops the service
func (s *Service) Stop(ctx context.Context) error {
	s.logger.Printf("🛑 Stopping CAN Communication Service...")

	// Stop message listening first
	if s.messageListener != nil {
		s.logger.Printf("🛑 Stopping message listener...")
		if err := s.messageListener.Shutdown(); err != nil {
			s.logger.Printf("Warning: failed to stop message listener: %v", err)
		}
	}

	// Stop watchdog
	if err := s.watchdog.Stop(); err != nil {
		s.logger.Printf("Warning: failed to stop watchdog: %v", err)
	}

	// Stop HTTP server
	if s.server != nil {
		if err := s.server.Shutdown(ctx); err != nil {
			s.logger.Printf("Warning: HTTP server shutdown error: %v", err)
		}
	}

	// Cleanup CAN interfaces
	if s.interfaceManager != nil {
		s.interfaceManager.Cleanup()
	}

	// Teardown CAN interfaces (new step)
	if s.setupManager != nil {
		s.teardownCanInterfaces()
	}

	s.logger.Printf("✅ CAN Communication Service stopped")
	return nil
}

// teardownCanInterfaces tears down all CAN interfaces
func (s *Service) teardownCanInterfaces() {
	s.logger.Printf("🔽 Tearing down CAN interfaces...")

	for _, ifName := range s.config.CanPorts {
		if err := s.setupManager.TeardownInterface(ifName); err != nil {
			s.logger.Printf("⚠️ Warning: failed to teardown %s: %v", ifName, err)
		}
	}

	s.logger.Printf("✅ CAN interfaces teardown complete")
}

// GetStatus returns current service status
func (s *Service) GetStatus() map[string]interface{} {
	if s.monitor == nil {
		return map[string]interface{}{
			"status": "not_initialized",
		}
	}

	systemStatus := s.monitor.GetSystemStatus()

	// Add setup manager status
	setupStatus := make(map[string]interface{})
	if s.setupManager != nil {
		setupStatus["config"] = s.setupManager.GetSetupConfig()

		// Get interface states
		interfaceStates := make(map[string]interface{})
		for _, ifName := range s.config.CanPorts {
			if state, err := s.setupManager.GetInterfaceState(ifName); err == nil {
				interfaceStates[ifName] = state
			} else {
				interfaceStates[ifName] = map[string]interface{}{
					"error": err.Error(),
				}
			}
		}
		setupStatus["interfaceStates"] = interfaceStates
	}

	// Add message listener status
	messageListenerStatus := make(map[string]interface{})
	if s.messageListener != nil {
		messageListenerStatus["listeningInterfaces"] = s.messageListener.GetListeningInterfaces()
		messageListenerStatus["statistics"] = s.messageListener.GetStatistics()
	}

	return map[string]interface{}{
		"status":           "running",
		"uptime":           systemStatus.SystemUptime.String(),
		"activeInterfaces": systemStatus.ActiveInterfaces,
		"watchdogRunning":  systemStatus.WatchdogStatus.Running,
		"setup":            setupStatus,
		"messageListener":  messageListenerStatus,
	}
}

// RestartInterfaceWithListening restarts an interface and its message listening
func (s *Service) RestartInterfaceWithListening(ifName string) error {
	s.logger.Printf("🔄 Restarting interface %s with message listening...", ifName)

	// Stop listening first
	if s.messageListener != nil {
		if err := s.messageListener.StopListening(ifName); err != nil {
			s.logger.Printf("⚠️ Warning: failed to stop listening on %s: %v", ifName, err)
		}
	}

	// Reset the interface
	if err := s.setupManager.ResetInterface(ifName); err != nil {
		return fmt.Errorf("failed to reset interface %s: %w", ifName, err)
	}

	// Wait a moment for interface to stabilize
	time.Sleep(1 * time.Second)

	// Restart listening
	if s.messageListener != nil {
		if err := s.messageListener.StartListening(ifName); err != nil {
			s.logger.Printf("⚠️ Warning: failed to restart listening on %s: %v", ifName, err)
			return fmt.Errorf("interface reset successful but failed to restart listening: %w", err)
		}
	}

	s.logger.Printf("✅ Successfully restarted interface %s with message listening", ifName)
	return nil
}

// GetMessageSummary returns a summary of message activity
func (s *Service) GetMessageSummary() map[string]interface{} {
	if s.messageListener == nil {
		return map[string]interface{}{
			"status": "message_listener_not_available",
		}
	}

	allStats := s.messageListener.GetStatistics()
	listeningInterfaces := s.messageListener.GetListeningInterfaces()

	totalReceived := uint64(0)
	totalBuffered := 0

	for _, stats := range allStats {
		if statsMap, ok := stats.(map[string]interface{}); ok {
			if totalRx, ok := statsMap["totalReceived"].(uint64); ok {
				totalReceived += totalRx
			}
			if buffered, ok := statsMap["bufferedCount"].(int); ok {
				totalBuffered += buffered
			}
		}
	}

	return map[string]interface{}{
		"listeningInterfaceCount": len(listeningInterfaces),
		"listeningInterfaces":     listeningInterfaces,
		"totalMessagesReceived":   totalReceived,
		"totalMessagesBuffered":   totalBuffered,
		"interfaceStatistics":     allStats,
	}
}

// main function
func main() {
	// Check if help was requested
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		PrintUsage()
		return
	}

	// Create service
	service := NewService()

	// Initialize service
	if err := service.Initialize(); err != nil {
		log.Fatalf("Failed to initialize service: %v", err)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start service
	if err := service.Start(ctx); err != nil {
		log.Fatalf("Failed to start service: %v", err)
	}

	// Print startup summary
	status := service.GetStatus()
	log.Printf("🎯 Service startup summary:")
	log.Printf("   - Active interfaces: %v", status["activeInterfaces"])
	log.Printf("   - Watchdog running: %v", status["watchdogRunning"])

	if messageListener, ok := status["messageListener"].(map[string]interface{}); ok {
		if listeningInterfaces, ok := messageListener["listeningInterfaces"].([]string); ok {
			log.Printf("   - Listening on: %v", listeningInterfaces)
		}
	}

	// Wait for interrupt signal for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Block until signal received
	<-sigChan
	log.Println("Shutdown signal received")

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop service
	if err := service.Stop(shutdownCtx); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}
