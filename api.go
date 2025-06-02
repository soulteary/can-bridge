package main

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIHandler handles HTTP API requests
type APIHandler struct {
	messageSender *MessageSender
	monitor       *Monitor
	setupManager  *InterfaceSetupManager
	logger        Logger
}

// NewAPIHandler creates a new API handler (legacy, without setup manager)
func NewAPIHandler(messageSender *MessageSender, monitor *Monitor, logger Logger) *APIHandler {
	return &APIHandler{
		messageSender: messageSender,
		monitor:       monitor,
		setupManager:  nil,
		logger:        logger,
	}
}

// NewAPIHandlerWithSetup creates a new API handler with setup manager
func NewAPIHandlerWithSetup(messageSender *MessageSender, monitor *Monitor, setupManager *InterfaceSetupManager, logger Logger) *APIHandler {
	return &APIHandler{
		messageSender: messageSender,
		monitor:       monitor,
		setupManager:  setupManager,
		logger:        logger,
	}
}

// SetupRoutes configures all API routes
func (h *APIHandler) SetupRoutes(r *gin.Engine) {
	// Simple status page
	r.GET("/", h.handleRoot)

	api := r.Group("/api")
	{
		// Message endpoints
		api.POST("/can", h.handleCanMessage)
		api.POST("/fingers", h.handleFingerPose)
		api.POST("/palm", h.handlePalmPose)

		// Status and monitoring endpoints
		api.GET("/status", h.handleSystemStatus)
		api.GET("/interfaces", h.handleInterfacesList)
		api.GET("/interfaces/:name/status", h.handleInterfaceStatus)
		api.GET("/health", h.handleHealthSummary)
		api.GET("/metrics", h.handleMetrics)

		// Interface setup endpoints (new)
		if h.setupManager != nil {
			setup := api.Group("/setup")
			{
				setup.GET("/config", h.handleGetSetupConfig)
				setup.PUT("/config", h.handleUpdateSetupConfig)
				setup.GET("/available", h.handleGetAvailableInterfaces)
				setup.POST("/interfaces/:name", h.handleSetupInterface)
				setup.DELETE("/interfaces/:name", h.handleTeardownInterface)
				setup.POST("/interfaces/:name/reset", h.handleResetInterface)
				setup.GET("/interfaces/:name/state", h.handleGetInterfaceState)
				setup.POST("/interfaces/setup-all", h.handleSetupAllInterfaces)
				setup.POST("/interfaces/teardown-all", h.handleTeardownAllInterfaces)
			}
		}
	}
}

// handleRoot serves the root endpoint
func (h *APIHandler) handleRoot(c *gin.Context) {
	c.String(http.StatusOK, "CAN Communication Service is running")
}

// handleCanMessage handles raw CAN message requests
func (h *APIHandler) handleCanMessage(c *gin.Context) {
	var req CanMessage
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "Invalid CAN message request", err)
		return
	}

	// Validate message
	if err := h.messageSender.ValidateMessage(req); err != nil {
		h.respondError(c, http.StatusBadRequest, "Message validation failed", err)
		return
	}

	// Send the CAN message
	if err := h.messageSender.SendCanMessage(req); err != nil {
		h.respondError(c, http.StatusInternalServerError, "Failed to send CAN message", err)
		return
	}

	h.respondSuccess(c, "CAN message sent successfully", req)
}

// handleFingerPose handles finger control requests (legacy support)
func (h *APIHandler) handleFingerPose(c *gin.Context) {
	var req FingerPoseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "Invalid finger pose data", err)
		return
	}

	// Validate pose data
	if err := h.messageSender.ValidateFingerPose(req.Pose); err != nil {
		h.respondError(c, http.StatusBadRequest, "Finger pose validation failed", err)
		return
	}

	// Send the finger pose
	if err := h.messageSender.SendFingerPose(req.Interface, req.Pose); err != nil {
		h.respondError(c, http.StatusInternalServerError, "Failed to send finger pose", err)
		return
	}

	responseData := map[string]interface{}{
		"interface": req.Interface,
		"pose":      req.Pose,
	}
	h.respondSuccess(c, "Finger pose command sent successfully", responseData)
}

// handlePalmPose handles palm control requests (legacy support)
func (h *APIHandler) handlePalmPose(c *gin.Context) {
	var req PalmPoseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "Invalid palm pose data", err)
		return
	}

	// Validate pose data
	if err := h.messageSender.ValidatePalmPose(req.Pose); err != nil {
		h.respondError(c, http.StatusBadRequest, "Palm pose validation failed", err)
		return
	}

	// Send the palm pose
	if err := h.messageSender.SendPalmPose(req.Interface, req.Pose); err != nil {
		h.respondError(c, http.StatusInternalServerError, "Failed to send palm pose", err)
		return
	}

	responseData := map[string]interface{}{
		"interface": req.Interface,
		"pose":      req.Pose,
	}
	h.respondSuccess(c, "Palm pose command sent successfully", responseData)
}

// handleSystemStatus returns complete system status
func (h *APIHandler) handleSystemStatus(c *gin.Context) {
	status := h.monitor.GetSystemStatus()
	h.respondSuccess(c, "", status)
}

// handleInterfacesList returns available CAN interfaces
func (h *APIHandler) handleInterfacesList(c *gin.Context) {
	status := h.monitor.GetSystemStatus()

	data := map[string]interface{}{
		"configuredPorts": status.ConfiguredPorts,
		"activePorts": func() []string {
			var active []string
			for name, ifStatus := range status.Interfaces {
				if ifStatus.Active {
					active = append(active, name)
				}
			}
			return active
		}(),
		"totalInterfaces": len(status.Interfaces),
		"activeCount":     status.ActiveInterfaces,
	}

	h.respondSuccess(c, "", data)
}

// handleInterfaceStatus returns status for a specific interface
func (h *APIHandler) handleInterfaceStatus(c *gin.Context) {
	ifName := c.Param("name")
	if ifName == "" {
		h.respondError(c, http.StatusBadRequest, "Interface name is required", nil)
		return
	}

	status, err := h.monitor.GetInterfaceStatus(ifName)
	if err != nil {
		h.respondError(c, http.StatusNotFound, "Interface not found", err)
		return
	}

	h.respondSuccess(c, "", status)
}

// handleHealthSummary returns system health summary
func (h *APIHandler) handleHealthSummary(c *gin.Context) {
	summary := h.monitor.GetHealthSummary()
	h.respondSuccess(c, "", summary)
}

// handleMetrics returns detailed metrics for monitoring systems
func (h *APIHandler) handleMetrics(c *gin.Context) {
	status := h.monitor.GetSystemStatus()

	// Transform to metrics format
	metrics := map[string]interface{}{
		"system": map[string]interface{}{
			"uptime_seconds":        status.SystemUptime.Seconds(),
			"active_interfaces":     status.ActiveInterfaces,
			"configured_interfaces": len(status.ConfiguredPorts),
			"watchdog_enabled":      status.WatchdogStatus.Running,
		},
		"interfaces": make(map[string]interface{}),
	}

	// Add per-interface metrics
	interfaceMetrics := make(map[string]interface{})
	for name, ifStatus := range status.Interfaces {
		interfaceMetrics[name] = map[string]interface{}{
			"active":               ifStatus.Active,
			"total_sent":           ifStatus.TotalSent,
			"total_errors":         ifStatus.TotalErrors,
			"success_rate":         parseSuccessRate(ifStatus.SuccessRate),
			"health_status":        ifStatus.Health.Status,
			"health_checks_passed": ifStatus.Health.ChecksPassed,
			"health_checks_failed": ifStatus.Health.ChecksFailed,
		}
	}
	metrics["interfaces"] = interfaceMetrics

	h.respondSuccess(c, "", metrics)
}

// ====== Interface Setup Handlers (New) ======

// handleGetSetupConfig returns current setup configuration
func (h *APIHandler) handleGetSetupConfig(c *gin.Context) {
	if h.setupManager == nil {
		h.respondError(c, http.StatusServiceUnavailable, "Setup manager not available", nil)
		return
	}

	config := h.setupManager.GetSetupConfig()
	h.respondSuccess(c, "", config)
}

// SetupConfigRequest represents a setup configuration update request
type SetupConfigRequest struct {
	Bitrate        *int    `json:"bitrate,omitempty"`
	SamplePoint    *string `json:"samplePoint,omitempty"`
	RestartMs      *int    `json:"restartMs,omitempty"`
	AutoRecovery   *bool   `json:"autoRecovery,omitempty"`
	TimeoutSeconds *int    `json:"timeoutSeconds,omitempty"`
	RetryAttempts  *int    `json:"retryAttempts,omitempty"`
}

// handleUpdateSetupConfig updates setup configuration
func (h *APIHandler) handleUpdateSetupConfig(c *gin.Context) {
	if h.setupManager == nil {
		h.respondError(c, http.StatusServiceUnavailable, "Setup manager not available", nil)
		return
	}

	var req SetupConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "Invalid setup configuration", err)
		return
	}

	// Get current config
	config := h.setupManager.GetSetupConfig()

	// Update fields if provided
	if req.Bitrate != nil {
		config.Bitrate = *req.Bitrate
	}
	if req.SamplePoint != nil {
		config.SamplePoint = *req.SamplePoint
	}
	if req.RestartMs != nil {
		config.RestartMs = *req.RestartMs
	}
	if req.AutoRecovery != nil {
		config.AutoRecovery = *req.AutoRecovery
	}
	if req.TimeoutSeconds != nil {
		config.TimeoutSeconds = *req.TimeoutSeconds
	}
	if req.RetryAttempts != nil {
		config.RetryAttempts = *req.RetryAttempts
	}

	// Update configuration
	if err := h.setupManager.UpdateSetupConfig(config); err != nil {
		h.respondError(c, http.StatusBadRequest, "Invalid configuration", err)
		return
	}

	h.respondSuccess(c, "Setup configuration updated successfully", config)
}

// handleGetAvailableInterfaces returns available CAN interfaces
func (h *APIHandler) handleGetAvailableInterfaces(c *gin.Context) {
	if h.setupManager == nil {
		h.respondError(c, http.StatusServiceUnavailable, "Setup manager not available", nil)
		return
	}

	interfaces, err := h.setupManager.GetAvailableInterfaces()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "Failed to get available interfaces", err)
		return
	}

	data := map[string]interface{}{
		"interfaces": interfaces,
		"count":      len(interfaces),
	}

	h.respondSuccess(c, "", data)
}

// SetupInterfaceRequest represents an interface setup request
type SetupInterfaceRequest struct {
	Bitrate     *int    `json:"bitrate,omitempty"`
	SamplePoint *string `json:"samplePoint,omitempty"`
	RestartMs   *int    `json:"restartMs,omitempty"`
	WithRetry   *bool   `json:"withRetry,omitempty"`
}

// handleSetupInterface sets up a specific CAN interface
func (h *APIHandler) handleSetupInterface(c *gin.Context) {
	if h.setupManager == nil {
		h.respondError(c, http.StatusServiceUnavailable, "Setup manager not available", nil)
		return
	}

	ifName := c.Param("name")
	if ifName == "" {
		h.respondError(c, http.StatusBadRequest, "Interface name is required", nil)
		return
	}

	var req SetupInterfaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body - use defaults
		req = SetupInterfaceRequest{}
	}

	// If custom parameters provided, temporarily update config
	originalConfig := h.setupManager.GetSetupConfig()
	if req.Bitrate != nil || req.SamplePoint != nil || req.RestartMs != nil {
		tempConfig := originalConfig
		if req.Bitrate != nil {
			tempConfig.Bitrate = *req.Bitrate
		}
		if req.SamplePoint != nil {
			tempConfig.SamplePoint = *req.SamplePoint
		}
		if req.RestartMs != nil {
			tempConfig.RestartMs = *req.RestartMs
		}

		// Temporarily update config
		h.setupManager.UpdateSetupConfig(tempConfig)
		defer h.setupManager.UpdateSetupConfig(originalConfig) // Restore original
	}

	// Setup interface
	var err error
	withRetry := req.WithRetry != nil && *req.WithRetry
	if withRetry {
		err = h.setupManager.SetupInterfaceWithRetry(ifName)
	} else {
		err = h.setupManager.SetupInterface(ifName)
	}

	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "Failed to setup interface", err)
		return
	}

	// Get interface state
	state, err := h.setupManager.GetInterfaceState(ifName)
	if err != nil {
		h.logger.Printf("Warning: could not get interface state after setup: %v", err)
		state = &InterfaceState{Name: ifName}
	}

	h.respondSuccess(c, fmt.Sprintf("Interface %s setup successfully", ifName), state)
}

// handleTeardownInterface tears down a specific CAN interface
func (h *APIHandler) handleTeardownInterface(c *gin.Context) {
	if h.setupManager == nil {
		h.respondError(c, http.StatusServiceUnavailable, "Setup manager not available", nil)
		return
	}

	ifName := c.Param("name")
	if ifName == "" {
		h.respondError(c, http.StatusBadRequest, "Interface name is required", nil)
		return
	}

	if err := h.setupManager.TeardownInterface(ifName); err != nil {
		h.respondError(c, http.StatusInternalServerError, "Failed to teardown interface", err)
		return
	}

	responseData := map[string]interface{}{
		"interface": ifName,
		"status":    "torn_down",
	}

	h.respondSuccess(c, fmt.Sprintf("Interface %s torn down successfully", ifName), responseData)
}

// handleResetInterface resets a specific CAN interface
func (h *APIHandler) handleResetInterface(c *gin.Context) {
	if h.setupManager == nil {
		h.respondError(c, http.StatusServiceUnavailable, "Setup manager not available", nil)
		return
	}

	ifName := c.Param("name")
	if ifName == "" {
		h.respondError(c, http.StatusBadRequest, "Interface name is required", nil)
		return
	}

	if err := h.setupManager.ResetInterface(ifName); err != nil {
		h.respondError(c, http.StatusInternalServerError, "Failed to reset interface", err)
		return
	}

	// Get interface state after reset
	state, err := h.setupManager.GetInterfaceState(ifName)
	if err != nil {
		h.logger.Printf("Warning: could not get interface state after reset: %v", err)
		state = &InterfaceState{Name: ifName}
	}

	h.respondSuccess(c, fmt.Sprintf("Interface %s reset successfully", ifName), state)
}

// handleGetInterfaceState returns the current state of a CAN interface
func (h *APIHandler) handleGetInterfaceState(c *gin.Context) {
	if h.setupManager == nil {
		h.respondError(c, http.StatusServiceUnavailable, "Setup manager not available", nil)
		return
	}

	ifName := c.Param("name")
	if ifName == "" {
		h.respondError(c, http.StatusBadRequest, "Interface name is required", nil)
		return
	}

	state, err := h.setupManager.GetInterfaceState(ifName)
	if err != nil {
		h.respondError(c, http.StatusNotFound, "Failed to get interface state", err)
		return
	}

	h.respondSuccess(c, "", state)
}

// SetupAllInterfacesRequest represents a request to setup all interfaces
type SetupAllInterfacesRequest struct {
	Interfaces []string `json:"interfaces,omitempty"` // If empty, use configured interfaces
	WithRetry  *bool    `json:"withRetry,omitempty"`
	Parallel   *bool    `json:"parallel,omitempty"`
}

// handleSetupAllInterfaces sets up all or specified interfaces
func (h *APIHandler) handleSetupAllInterfaces(c *gin.Context) {
	if h.setupManager == nil {
		h.respondError(c, http.StatusServiceUnavailable, "Setup manager not available", nil)
		return
	}

	var req SetupAllInterfacesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body
		req = SetupAllInterfacesRequest{}
	}

	// Get interfaces to setup
	var interfaces []string
	if len(req.Interfaces) > 0 {
		interfaces = req.Interfaces
	} else {
		// Use system status to get configured ports
		status := h.monitor.GetSystemStatus()
		interfaces = status.ConfiguredPorts
	}

	withRetry := req.WithRetry != nil && *req.WithRetry
	results := make(map[string]interface{})
	var setupErrors []string

	for _, ifName := range interfaces {
		var err error
		if withRetry {
			err = h.setupManager.SetupInterfaceWithRetry(ifName)
		} else {
			err = h.setupManager.SetupInterface(ifName)
		}

		if err != nil {
			setupErrors = append(setupErrors, fmt.Sprintf("%s: %v", ifName, err))
			results[ifName] = map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			}
		} else {
			// Get interface state
			if state, err := h.setupManager.GetInterfaceState(ifName); err == nil {
				results[ifName] = map[string]interface{}{
					"success": true,
					"state":   state,
				}
			} else {
				results[ifName] = map[string]interface{}{
					"success": true,
					"warning": "could not get state after setup",
				}
			}
		}
	}

	responseData := map[string]interface{}{
		"results":      results,
		"totalCount":   len(interfaces),
		"successCount": len(interfaces) - len(setupErrors),
		"errorCount":   len(setupErrors),
	}

	if len(setupErrors) > 0 {
		responseData["errors"] = setupErrors
		h.respondSuccess(c, "Partial setup completed with errors", responseData)
	} else {
		h.respondSuccess(c, "All interfaces setup successfully", responseData)
	}
}

// handleTeardownAllInterfaces tears down all configured interfaces
func (h *APIHandler) handleTeardownAllInterfaces(c *gin.Context) {
	if h.setupManager == nil {
		h.respondError(c, http.StatusServiceUnavailable, "Setup manager not available", nil)
		return
	}

	// Get configured ports
	status := h.monitor.GetSystemStatus()
	interfaces := status.ConfiguredPorts

	results := make(map[string]interface{})
	var teardownErrors []string

	for _, ifName := range interfaces {
		if err := h.setupManager.TeardownInterface(ifName); err != nil {
			teardownErrors = append(teardownErrors, fmt.Sprintf("%s: %v", ifName, err))
			results[ifName] = map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			}
		} else {
			results[ifName] = map[string]interface{}{
				"success": true,
				"status":  "torn_down",
			}
		}
	}

	responseData := map[string]interface{}{
		"results":      results,
		"totalCount":   len(interfaces),
		"successCount": len(interfaces) - len(teardownErrors),
		"errorCount":   len(teardownErrors),
	}

	if len(teardownErrors) > 0 {
		responseData["errors"] = teardownErrors
		h.respondSuccess(c, "Partial teardown completed with errors", responseData)
	} else {
		h.respondSuccess(c, "All interfaces torn down successfully", responseData)
	}
}

// ====== Helper methods for consistent response formatting ======

// respondSuccess sends a successful JSON response
func (h *APIHandler) respondSuccess(c *gin.Context, message string, data interface{}) {
	response := ApiResponse{
		Status: "success",
		Data:   data,
	}
	if message != "" {
		response.Message = message
	}
	c.JSON(http.StatusOK, response)
}

// respondError sends an error JSON response
func (h *APIHandler) respondError(c *gin.Context, statusCode int, message string, err error) {
	response := ApiResponse{
		Status: "error",
		Error:  message,
	}

	if err != nil {
		response.Error = message + ": " + err.Error()
		h.logger.Printf("API Error: %s - %v", message, err)
	}

	c.JSON(statusCode, response)
}

// parseSuccessRate converts success rate string to float
func parseSuccessRate(rateStr string) float64 {
	// Simple parsing - in production you might want more robust parsing
	var rate float64
	if n, err := fmt.Sscanf(rateStr, "%f%%", &rate); n == 1 && err == nil {
		return rate
	}
	return 0.0
}

// ====== Middleware functions ======

// LoggingMiddleware provides request logging
func LoggingMiddleware(logger Logger) gin.HandlerFunc {
	return gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/api/status", "/api/health"}, // Skip status check logging
		Formatter: func(param gin.LogFormatterParams) string {
			return fmt.Sprintf("%s - [%s] \"%s %s %s %d %s \"%s\" %s\"\n",
				param.ClientIP,
				param.TimeStamp.Format("02/Jan/2006:15:04:05 -0700"),
				param.Method,
				param.Path,
				param.Request.Proto,
				param.StatusCode,
				param.Latency,
				param.Request.UserAgent(),
				param.ErrorMessage,
			)
		},
	})
}

// CORSMiddleware provides CORS support
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// RecoveryMiddleware provides panic recovery
func RecoveryMiddleware(logger Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		logger.Printf("Panic recovered: %v", recovered)
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Status: "error",
			Error:  "Internal server error",
		})
	})
}
