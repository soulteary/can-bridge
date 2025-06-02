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
	interfaceManager *InterfaceManager
	messageSender    *MessageSender
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

	// Initialize CAN interfaces
	if err := s.interfaceManager.InitializeAll(); err != nil {
		s.logger.Printf("Warning: %v", err)
		// We continue even if some interfaces failed
	}

	// Setup HTTP server
	s.setupHTTPServer()

	return nil
}

// initializeComponents initializes all service components
func (s *Service) initializeComponents() error {
	// Create socket provider
	socketProvider := NewUnixSocketProvider()

	// Create interface manager
	s.interfaceManager = NewInterfaceManager(s.configProvider, socketProvider, s.logger)

	// Create message sender
	s.messageSender = NewMessageSender(s.interfaceManager, s.configProvider, socketProvider, s.logger)

	// Create watchdog
	watchdogConfig := DefaultWatchdogConfig()
	s.watchdog = NewWatchdog(s.interfaceManager, watchdogConfig, s.logger)

	// Create monitor
	s.monitor = NewMonitor(s.interfaceManager, s.watchdog, s.configProvider)

	// Create API handler
	s.apiHandler = NewAPIHandler(s.messageSender, s.monitor, s.logger)

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
	if err := s.watchdog.Start(ctx); err != nil {
		return fmt.Errorf("failed to start watchdog: %w", err)
	}

	// Start HTTP server in a goroutine
	go func() {
		s.logger.Printf("🌐 Starting HTTP server on %s", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("❌ HTTP server error: %v", err)
		}
	}()

	s.logger.Printf("✅ CAN Communication Service started successfully")
	return nil
}

// Stop gracefully stops the service
func (s *Service) Stop(ctx context.Context) error {
	s.logger.Printf("🛑 Stopping CAN Communication Service...")

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

	s.logger.Printf("✅ CAN Communication Service stopped")
	return nil
}

// GetStatus returns current service status
func (s *Service) GetStatus() map[string]interface{} {
	if s.monitor == nil {
		return map[string]interface{}{
			"status": "not_initialized",
		}
	}

	systemStatus := s.monitor.GetSystemStatus()
	return map[string]interface{}{
		"status":           "running",
		"uptime":           systemStatus.SystemUptime.String(),
		"activeInterfaces": systemStatus.ActiveInterfaces,
		"watchdogRunning":  systemStatus.WatchdogStatus.Running,
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
