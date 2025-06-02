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
	logger        Logger
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(messageSender *MessageSender, monitor *Monitor, logger Logger) *APIHandler {
	return &APIHandler{
		messageSender: messageSender,
		monitor:       monitor,
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

		// Administrative endpoints
		api.GET("/metrics", h.handleMetrics)
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

// Helper methods for consistent response formatting

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

// Middleware functions

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
