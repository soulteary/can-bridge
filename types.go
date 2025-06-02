package main

import (
	"sync"
	"time"

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
	Interface string `json:"interface" binding:"required"`
	ID        uint32 `json:"id" binding:"required"`
	Data      []byte `json:"data" binding:"required,min=1,max=8"`
	Length    uint8  `json:"length,omitempty"`
}

// API response structure
type ApiResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// Metrics structure for better testing
type InterfaceMetrics struct {
	TotalSent      uint64
	TotalErrors    uint64
	LastSendTime   time.Time
	StartTime      time.Time
	LastErrorTime  time.Time
	LastErrorMsg   string
	AvgLatency     time.Duration
	MessageLatency []time.Duration
	mutex          sync.RWMutex
}

// NewInterfaceMetrics creates a new metrics instance
func NewInterfaceMetrics() *InterfaceMetrics {
	return &InterfaceMetrics{
		StartTime:      time.Now(),
		MessageLatency: make([]time.Duration, 0, 100),
	}
}

// RecordSuccess updates metrics for successful send
func (m *InterfaceMetrics) RecordSuccess(latency time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.TotalSent++
	m.LastSendTime = time.Now()

	// Update latency tracking
	m.MessageLatency = append(m.MessageLatency, latency)
	if len(m.MessageLatency) > 100 {
		m.MessageLatency = m.MessageLatency[1:]
	}

	// Compute average latency
	var totalLatency time.Duration
	for _, lat := range m.MessageLatency {
		totalLatency += lat
	}
	if len(m.MessageLatency) > 0 {
		m.AvgLatency = totalLatency / time.Duration(len(m.MessageLatency))
	}
}

// RecordError updates metrics for failed send
func (m *InterfaceMetrics) RecordError(err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.TotalErrors++
	m.LastErrorTime = time.Now()
	m.LastErrorMsg = err.Error()
}

// GetStats returns a snapshot of current metrics
func (m *InterfaceMetrics) GetStats() InterfaceStats {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return InterfaceStats{
		TotalSent:     m.TotalSent,
		TotalErrors:   m.TotalErrors,
		LastSendTime:  m.LastSendTime,
		StartTime:     m.StartTime,
		LastErrorTime: m.LastErrorTime,
		LastErrorMsg:  m.LastErrorMsg,
		AvgLatency:    m.AvgLatency,
		Uptime:        time.Since(m.StartTime),
	}
}

// InterfaceStats represents a snapshot of metrics
type InterfaceStats struct {
	TotalSent     uint64
	TotalErrors   uint64
	LastSendTime  time.Time
	StartTime     time.Time
	LastErrorTime time.Time
	LastErrorMsg  string
	AvgLatency    time.Duration
	Uptime        time.Duration
}

// SuccessRate calculates the success rate percentage
func (s InterfaceStats) SuccessRate() float64 {
	if s.TotalSent == 0 {
		return 100.0
	}
	return 100 * float64(s.TotalSent-s.TotalErrors) / float64(s.TotalSent)
}

// CAN interface structure
type CanInterface struct {
	Name    string
	FD      int
	Addr    *unix.SockaddrCAN
	Metrics *InterfaceMetrics
	mutex   sync.Mutex
}

// NewCanInterface creates a new CAN interface instance
func NewCanInterface(name string, fd int, addr *unix.SockaddrCAN) *CanInterface {
	return &CanInterface{
		Name:    name,
		FD:      fd,
		Addr:    addr,
		Metrics: NewInterfaceMetrics(),
	}
}

// Lock locks the interface mutex
func (c *CanInterface) Lock() {
	c.mutex.Lock()
}

// Unlock unlocks the interface mutex
func (c *CanInterface) Unlock() {
	c.mutex.Unlock()
}

// GetStats returns interface statistics
func (c *CanInterface) GetStats() InterfaceStats {
	return c.Metrics.GetStats()
}
