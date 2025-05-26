package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/gin-gonic/gin"
	"golang.org/x/sys/unix"
)

const IFNAMSIZ = 16

// CAN frame structure
type CanFrame struct {
	ID     uint32
	Length uint8
	_      [3]byte
	Data   [8]byte
}

// ioctl interface structure
type ifreq struct {
	Name  [IFNAMSIZ]byte
	Index int32
	_     [24 - 4]byte
}

// Request structures
type CanMessage struct {
	Interface string `json:"interface" binding:"required"` // Dynamic interface validation
	ID        uint32 `json:"id" binding:"required"`
	Data      []byte `json:"data" binding:"required,min=1,max=8"`
	Length    uint8  `json:"length,omitempty"`
}

// Finger pose request for backward compatibility
type FingerPoseRequest struct {
	Interface string `json:"interface,omitempty"` // Dynamic interface validation
	Pose      []byte `json:"pose" binding:"required,len=6"`
}

// Palm pose request for backward compatibility
type PalmPoseRequest struct {
	Interface string `json:"interface,omitempty"` // Dynamic interface validation
	Pose      []byte `json:"pose" binding:"required,len=4"`
}

// API response structure
type ApiResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// CAN interface structure
type CanInterface struct {
	Name    string
	FD      int
	Addr    *unix.SockaddrCAN
	Mutex   sync.Mutex
	Metrics struct {
		TotalSent      uint64
		TotalErrors    uint64
		LastSendTime   time.Time
		StartTime      time.Time
		LastErrorTime  time.Time
		LastErrorMsg   string
		AvgLatency     time.Duration
		MessageLatency []time.Duration
	}
}

// Global variables
var (
	canInterfaces   map[string]*CanInterface
	configuredPorts []string // Store configured CAN ports
)

// Configuration structure
type Config struct {
	CanPorts []string
	Port     string
}

// Parse configuration from command line and environment variables
func parseConfig() *Config {
	config := &Config{}

	// Command line flags
	var canPortsFlag string
	var serverPort string

	flag.StringVar(&canPortsFlag, "can-ports", "", "Comma-separated list of CAN interfaces (e.g., can0,can1)")
	flag.StringVar(&serverPort, "port", "5260", "HTTP server port")
	flag.Parse()

	// Environment variables (override command line)
	if envPorts := os.Getenv("CAN_PORTS"); envPorts != "" {
		canPortsFlag = envPorts
	}
	if envPort := os.Getenv("SERVER_PORT"); envPort != "" {
		serverPort = envPort
	}

	// Parse CAN ports
	if canPortsFlag != "" {
		config.CanPorts = strings.Split(canPortsFlag, ",")
		// Trim whitespace from each port
		for i, port := range config.CanPorts {
			config.CanPorts[i] = strings.TrimSpace(port)
		}
	} else {
		// Default to can0 if no ports specified
		config.CanPorts = []string{"can0"}
	}

	config.Port = serverPort

	return config
}

// Validate if interface is in configured ports
func isValidInterface(ifName string) bool {
	for _, port := range configuredPorts {
		if port == ifName {
			return true
		}
	}
	return false
}

// Initialize all CAN interfaces based on configuration
func initAllCAN(ports []string) error {
	canInterfaces = make(map[string]*CanInterface)
	configuredPorts = ports

	log.Printf("🔧 Initializing CAN interfaces: %v", ports)

	var lastErr error
	successCount := 0

	for _, ifName := range ports {
		err := initSingleCAN(ifName)
		if err != nil {
			lastErr = err
			log.Printf("❌ Failed to initialize %s: %v", ifName, err)
		} else {
			log.Printf("✅ Successfully initialized %s", ifName)
			successCount++
		}
	}

	// If all interfaces failed, return error
	if successCount == 0 {
		return fmt.Errorf("failed to initialize any CAN interface from %v: %v", ports, lastErr)
	}

	log.Printf("🎯 Successfully initialized %d/%d CAN interfaces", successCount, len(ports))
	return nil
}

// Initialize single CAN interface with retry logic
func initSingleCAN(ifName string) error {
	var err error
	retries := 5
	retryDelay := 2 * time.Second

	for i := 0; i < retries; i++ {
		// Open CAN socket
		fd, err := unix.Socket(unix.AF_CAN, unix.SOCK_RAW, unix.CAN_RAW)
		if err == nil {
			// Get CAN interface index
			ifindex, err := getIfIndex(fd, ifName)
			if err == nil {
				// Bind to CAN interface
				addr := &unix.SockaddrCAN{Ifindex: ifindex}
				if err = unix.Bind(fd, addr); err == nil {
					// Create interface struct
					canIf := &CanInterface{
						Name: ifName,
						FD:   fd,
						Addr: addr,
					}

					// Initialize metrics
					canIf.Metrics.StartTime = time.Now()
					canIf.Metrics.MessageLatency = make([]time.Duration, 0, 100)

					// Add to map
					canInterfaces[ifName] = canIf

					log.Printf("✅ %s initialization successful", ifName)
					return nil
				}
			}
			// Close socket if binding failed
			unix.Close(fd)
		}

		log.Printf("⚠️ %s initialization attempt %d failed: %v. Retrying in %v...",
			ifName, i+1, err, retryDelay)
		time.Sleep(retryDelay)
	}

	return fmt.Errorf("failed to initialize %s after %d attempts: %v", ifName, retries, err)
}

// Get CAN interface index
func getIfIndex(fd int, ifname string) (int, error) {
	var ifr ifreq
	copy(ifr.Name[:], ifname)
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd),
		uintptr(unix.SIOCGIFINDEX),
		uintptr(unsafe.Pointer(&ifr)),
	)
	if errno != 0 {
		return 0, errno
	}
	return int(ifr.Index), nil
}

// Send raw CAN message with interface validation
func sendCanMessage(msg CanMessage) error {
	// Validate interface is configured
	if !isValidInterface(msg.Interface) {
		return fmt.Errorf("CAN interface %s is not configured. Available interfaces: %v",
			msg.Interface, configuredPorts)
	}

	// Get interface
	canIf, ok := canInterfaces[msg.Interface]
	if !ok {
		return fmt.Errorf("CAN interface %s not initialized", msg.Interface)
	}

	canIf.Mutex.Lock()
	defer canIf.Mutex.Unlock()

	startTime := time.Now()

	// Validate data length
	if len(msg.Data) > 8 {
		return fmt.Errorf("CAN data exceeds maximum length (8 bytes)")
	}

	// Prepare CAN frame
	frame := CanFrame{
		ID:     msg.ID,
		Length: uint8(len(msg.Data)),
	}

	// Copy data to frame
	for i := 0; i < len(msg.Data) && i < 8; i++ {
		frame.Data[i] = msg.Data[i]
	}

	// Send CAN frame
	buf := (*[16]byte)(unsafe.Pointer(&frame))[:]
	err := unix.Sendto(canIf.FD, buf, 0, canIf.Addr)

	// Update metrics
	if err == nil {
		canIf.Metrics.TotalSent++
		canIf.Metrics.LastSendTime = time.Now()

		// Calculate latency
		latency := time.Since(startTime)
		canIf.Metrics.MessageLatency = append(canIf.Metrics.MessageLatency, latency)
		if len(canIf.Metrics.MessageLatency) > 100 {
			canIf.Metrics.MessageLatency = canIf.Metrics.MessageLatency[1:]
		}

		// Compute average latency
		var totalLatency time.Duration
		for _, lat := range canIf.Metrics.MessageLatency {
			totalLatency += lat
		}
		canIf.Metrics.AvgLatency = totalLatency / time.Duration(len(canIf.Metrics.MessageLatency))

		// Log success
		log.Printf("✅ %s message sent: ID=0x%X, Data=[% X], Length=%d, Latency=%v",
			msg.Interface, msg.ID, msg.Data, frame.Length, latency)
	} else {
		canIf.Metrics.TotalErrors++
		canIf.Metrics.LastErrorTime = time.Now()
		canIf.Metrics.LastErrorMsg = err.Error()

		// Log error
		log.Printf("❌ %s message send failed: ID=0x%X, Error=%v", msg.Interface, msg.ID, err)
	}

	return err
}

// Send finger pose (for backward compatibility)
func sendFingerPose(ifName string, pose []byte) error {
	if len(pose) != 6 {
		return fmt.Errorf("invalid pose data length, need 6 bytes")
	}

	// Default to first configured port if not specified
	if ifName == "" {
		if len(configuredPorts) > 0 {
			ifName = configuredPorts[0]
		} else {
			return fmt.Errorf("no CAN interfaces configured")
		}
	}

	// Validate interface
	if !isValidInterface(ifName) {
		return fmt.Errorf("CAN interface %s is not configured. Available interfaces: %v",
			ifName, configuredPorts)
	}

	// Construct CAN message
	msg := CanMessage{
		Interface: ifName,
		ID:        0x28,
		Data:      append([]byte{0x01}, pose...),
	}

	return sendCanMessage(msg)
}

// Send palm pose (for backward compatibility)
func sendPalmPose(ifName string, pose []byte) error {
	if len(pose) != 4 {
		return fmt.Errorf("invalid pose data length, need 4 bytes")
	}

	// Default to first configured port if not specified
	if ifName == "" {
		if len(configuredPorts) > 0 {
			ifName = configuredPorts[0]
		} else {
			return fmt.Errorf("no CAN interfaces configured")
		}
	}

	// Validate interface
	if !isValidInterface(ifName) {
		return fmt.Errorf("CAN interface %s is not configured. Available interfaces: %v",
			ifName, configuredPorts)
	}

	// Construct CAN message
	msg := CanMessage{
		Interface: ifName,
		ID:        0x28,
		Data:      append([]byte{0x04}, pose...),
	}

	return sendCanMessage(msg)
}

// Health check for CAN interface
func checkCanHealth(ifName string) bool {
	canIf, ok := canInterfaces[ifName]
	if !ok {
		return false
	}

	canIf.Mutex.Lock()
	defer canIf.Mutex.Unlock()

	// Simple probe message (0x00 is typically a diagnostic/echo ID)
	frame := CanFrame{
		ID:     0x00,
		Length: 1,
		Data:   [8]byte{0x00},
	}

	buf := (*[16]byte)(unsafe.Pointer(&frame))[:]
	err := unix.Sendto(canIf.FD, buf, 0, canIf.Addr)

	if err != nil {
		log.Printf("⚠️ %s health check failed: %v", ifName, err)
		return false
	}

	return true
}

// Automatic watchdog to monitor and recover CAN connections
func startCanWatchdog() {
	go func() {
		for {
			// Check CAN interface health every 10 seconds
			time.Sleep(10 * time.Second)

			// Check each interface
			for ifName, canIf := range canInterfaces {
				// Skip health check if no errors or recent successful sends after errors
				if canIf.Metrics.LastErrorTime.IsZero() ||
					canIf.Metrics.LastSendTime.After(canIf.Metrics.LastErrorTime) ||
					time.Since(canIf.Metrics.LastErrorTime) >= 30*time.Second {
					continue
				}

				// Check interface health
				if !checkCanHealth(ifName) {
					log.Printf("🔄 %s interface appears down, attempting to reinitialize...", ifName)

					// Close existing socket
					unix.Close(canIf.FD)

					// Remove from map
					delete(canInterfaces, ifName)

					// Attempt to reinitialize
					if err := initSingleCAN(ifName); err != nil {
						log.Printf("❌ %s reinitialization failed: %v", ifName, err)
					} else {
						log.Printf("✅ %s interface successfully reinitialized", ifName)
					}
				}
			}
		}
	}()
}

// Setup API routes
func setupRoutes(r *gin.Engine) {
	// Simple status page
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "CAN Communication Service is running")
	})

	api := r.Group("/api")
	{
		// Raw CAN message endpoint
		api.POST("/can", func(c *gin.Context) {
			var req CanMessage
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, ApiResponse{
					Status: "error",
					Error:  "Invalid CAN message request: " + err.Error(),
				})
				return
			}

			// Send the CAN message
			if err := sendCanMessage(req); err != nil {
				c.JSON(http.StatusInternalServerError, ApiResponse{
					Status: "error",
					Error:  "Failed to send CAN message: " + err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, ApiResponse{
				Status:  "success",
				Message: "CAN message sent successfully",
				Data:    req,
			})
		})

		// Finger control endpoint (legacy support)
		api.POST("/fingers", func(c *gin.Context) {
			var req FingerPoseRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, ApiResponse{
					Status: "error",
					Error:  "Invalid finger pose data: " + err.Error(),
				})
				return
			}

			// Send the finger pose
			if err := sendFingerPose(req.Interface, req.Pose); err != nil {
				c.JSON(http.StatusInternalServerError, ApiResponse{
					Status: "error",
					Error:  "Failed to send finger pose: " + err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, ApiResponse{
				Status:  "success",
				Message: "Finger pose command sent successfully",
				Data: map[string]interface{}{
					"interface": req.Interface,
					"pose":      req.Pose,
				},
			})
		})

		// Palm control endpoint (legacy support)
		api.POST("/palm", func(c *gin.Context) {
			var req PalmPoseRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, ApiResponse{
					Status: "error",
					Error:  "Invalid palm pose data: " + err.Error(),
				})
				return
			}

			// Send the palm pose
			if err := sendPalmPose(req.Interface, req.Pose); err != nil {
				c.JSON(http.StatusInternalServerError, ApiResponse{
					Status: "error",
					Error:  "Failed to send palm pose: " + err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, ApiResponse{
				Status:  "success",
				Message: "Palm pose command sent successfully",
				Data: map[string]interface{}{
					"interface": req.Interface,
					"pose":      req.Pose,
				},
			})
		})

		// System status and metrics for all interfaces
		api.GET("/status", func(c *gin.Context) {
			interfaceStatuses := make(map[string]interface{})

			for ifName, canIf := range canInterfaces {
				canIf.Mutex.Lock()
				uptime := time.Since(canIf.Metrics.StartTime)
				totalSent := canIf.Metrics.TotalSent
				totalErrors := canIf.Metrics.TotalErrors
				lastSend := canIf.Metrics.LastSendTime
				lastError := canIf.Metrics.LastErrorTime
				lastErrorMsg := canIf.Metrics.LastErrorMsg
				avgLatency := canIf.Metrics.AvgLatency
				canIf.Mutex.Unlock()

				// Check current CAN health
				canHealth := checkCanHealth(ifName)

				// Calculate success rate
				var successRate float64
				if totalSent > 0 {
					successRate = 100 * float64(totalSent-totalErrors) / float64(totalSent)
				} else {
					successRate = 100.0
				}

				interfaceStatuses[ifName] = map[string]interface{}{
					"active":        canHealth,
					"uptime":        uptime.String(),
					"totalSent":     totalSent,
					"totalErrors":   totalErrors,
					"successRate":   fmt.Sprintf("%.2f%%", successRate),
					"lastSendTime":  lastSend,
					"lastErrorTime": lastError,
					"lastErrorMsg":  lastErrorMsg,
					"avgLatency":    avgLatency.String(),
				}
			}

			c.JSON(http.StatusOK, ApiResponse{
				Status: "success",
				Data: map[string]interface{}{
					"interfaces":          interfaceStatuses,
					"activeInterfaces":    len(canInterfaces),
					"configuredPorts":     configuredPorts,
					"availableInterfaces": configuredPorts,
				},
			})
		})

		// Get available CAN interfaces
		api.GET("/interfaces", func(c *gin.Context) {
			c.JSON(http.StatusOK, ApiResponse{
				Status: "success",
				Data: map[string]interface{}{
					"configuredPorts": configuredPorts,
					"activePorts": func() []string {
						var active []string
						for ifName := range canInterfaces {
							active = append(active, ifName)
						}
						return active
					}(),
				},
			})
		})
	}
}

func printUsage() {
	fmt.Println("CAN Communication Service")
	fmt.Println("Usage:")
	fmt.Println("  -can-ports string    Comma-separated list of CAN interfaces (default: can0)")
	fmt.Println("  -port string         HTTP server port (default: 8080)")
	fmt.Println("")
	fmt.Println("Environment Variables:")
	fmt.Println("  CAN_PORTS           Comma-separated list of CAN interfaces")
	fmt.Println("  SERVER_PORT         HTTP server port")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  ./can-bridge -can-ports can0,can1")
	fmt.Println("  ./can-bridge -can-ports can0,can1,vcan0 -port 9090")
	fmt.Println("  CAN_PORTS=can0,can1 ./can-bridge")
}

func main() {
	// Parse configuration
	config := parseConfig()

	// Check if help was requested
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		printUsage()
		return
	}

	// Set to production mode
	gin.SetMode(gin.ReleaseMode)

	log.Printf("🚀 Starting CAN Communication Service")
	log.Printf("📋 Configuration:")
	log.Printf("   - CAN Ports: %v", config.CanPorts)
	log.Printf("   - Server Port: %s", config.Port)

	// Initialize CAN interfaces with error handling
	if err := initAllCAN(config.CanPorts); err != nil {
		log.Printf("Warning: %v", err)
		// We continue even if some interfaces failed
	}

	// Clean up on exit
	defer func() {
		log.Println("🧹 Cleaning up CAN interfaces...")
		for _, canIf := range canInterfaces {
			unix.Close(canIf.FD)
		}
	}()

	// Start CAN watchdog
	startCanWatchdog()

	// Create Gin engine with minimal logging
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/api/status"}, // Don't log status checks
	}))

	// Setup API routes
	setupRoutes(r)

	// Start server
	serverAddr := ":" + config.Port
	log.Printf("🌐 CAN Communication Service running at http://localhost%s", serverAddr)

	// Use a custom server with timeouts
	server := &http.Server{
		Addr:         serverAddr,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
