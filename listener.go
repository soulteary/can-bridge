package main

import (
	"context"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// CanMessageLog represents a logged CAN message
type CanMessageLog struct {
	Interface string    `json:"interface"`
	ID        uint32    `json:"id"`
	Data      []byte    `json:"data"`
	Length    uint8     `json:"length"`
	Timestamp time.Time `json:"timestamp"`
	Direction string    `json:"direction"` // "RX" for received messages
}

// InterfaceMessageBuffer manages message history for a single interface
type InterfaceMessageBuffer struct {
	interfaceName string
	messages      []CanMessageLog
	maxSize       int
	mutex         sync.RWMutex
	totalReceived uint64
}

// NewInterfaceMessageBuffer creates a new message buffer for an interface
func NewInterfaceMessageBuffer(interfaceName string, maxSize int) *InterfaceMessageBuffer {
	return &InterfaceMessageBuffer{
		interfaceName: interfaceName,
		messages:      make([]CanMessageLog, 0, maxSize),
		maxSize:       maxSize,
	}
}

// AddMessage adds a new message to the buffer
func (buf *InterfaceMessageBuffer) AddMessage(msg CanMessageLog) {
	buf.mutex.Lock()
	defer buf.mutex.Unlock()

	buf.totalReceived++

	// Add message to buffer
	buf.messages = append(buf.messages, msg)

	// Maintain buffer size limit
	if len(buf.messages) > buf.maxSize {
		// Remove oldest message
		buf.messages = buf.messages[1:]
	}
}

// GetMessages returns a copy of all messages
func (buf *InterfaceMessageBuffer) GetMessages() []CanMessageLog {
	buf.mutex.RLock()
	defer buf.mutex.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]CanMessageLog, len(buf.messages))
	copy(result, buf.messages)
	return result
}

// GetRecentMessages returns the last N messages
func (buf *InterfaceMessageBuffer) GetRecentMessages(count int) []CanMessageLog {
	buf.mutex.RLock()
	defer buf.mutex.RUnlock()

	if count <= 0 {
		return []CanMessageLog{}
	}

	if count >= len(buf.messages) {
		// Return all messages
		result := make([]CanMessageLog, len(buf.messages))
		copy(result, buf.messages)
		return result
	}

	// Return last N messages
	start := len(buf.messages) - count
	result := make([]CanMessageLog, count)
	copy(result, buf.messages[start:])
	return result
}

// GetStatistics returns buffer statistics
func (buf *InterfaceMessageBuffer) GetStatistics() map[string]interface{} {
	buf.mutex.RLock()
	defer buf.mutex.RUnlock()

	return map[string]interface{}{
		"interface":     buf.interfaceName,
		"totalReceived": buf.totalReceived,
		"bufferedCount": len(buf.messages),
		"maxBufferSize": buf.maxSize,
		"bufferUsage":   float64(len(buf.messages)) / float64(buf.maxSize) * 100,
	}
}

// Clear clears all messages from the buffer
func (buf *InterfaceMessageBuffer) Clear() {
	buf.mutex.Lock()
	defer buf.mutex.Unlock()

	buf.messages = buf.messages[:0] // Clear slice but keep capacity
	buf.totalReceived = 0
}

// CanMessageListener manages listening to CAN messages on multiple interfaces
type CanMessageListener struct {
	buffers      map[string]*InterfaceMessageBuffer
	buffersMutex sync.RWMutex
	listeners    map[string]*interfaceListener
	maxMessages  int
	logger       Logger
	ctx          context.Context
	cancel       context.CancelFunc
}

// interfaceListener manages listening for a single interface
type interfaceListener struct {
	interfaceName string
	socket        int
	isRunning     bool
	stopChan      chan bool
	buffer        *InterfaceMessageBuffer
	logger        Logger
}

// NewCanMessageListener creates a new CAN message listener
func NewCanMessageListener(maxMessages int, logger Logger) *CanMessageListener {
	ctx, cancel := context.WithCancel(context.Background())
	return &CanMessageListener{
		buffers:     make(map[string]*InterfaceMessageBuffer),
		listeners:   make(map[string]*interfaceListener),
		maxMessages: maxMessages,
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// StartListening starts listening on a specific CAN interface
func (cml *CanMessageListener) StartListening(interfaceName string) error {
	cml.buffersMutex.Lock()
	defer cml.buffersMutex.Unlock()

	// Check if already listening
	if listener, exists := cml.listeners[interfaceName]; exists && listener.isRunning {
		cml.logger.Printf("📡 Already listening on %s", interfaceName)
		return nil
	}

	cml.logger.Printf("📡 Starting CAN message listener for %s", interfaceName)

	// Create message buffer
	buffer := NewInterfaceMessageBuffer(interfaceName, cml.maxMessages)
	cml.buffers[interfaceName] = buffer

	// Create socket for listening
	socket, err := unix.Socket(unix.AF_CAN, unix.SOCK_RAW, unix.CAN_RAW)
	if err != nil {
		return fmt.Errorf("failed to create listening socket: %w", err)
	}

	// Get interface index
	var ifr ifreq
	copy(ifr.Name[:], interfaceName)
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(socket),
		uintptr(unix.SIOCGIFINDEX),
		uintptr(unsafe.Pointer(&ifr)),
	)
	if errno != 0 {
		unix.Close(socket)
		return fmt.Errorf("failed to get interface index: %v", errno)
	}

	// Bind socket to interface
	addr := &unix.SockaddrCAN{Ifindex: int(ifr.Index)}
	if err := unix.Bind(socket, addr); err != nil {
		unix.Close(socket)
		return fmt.Errorf("failed to bind listening socket: %w", err)
	}

	// Create listener
	listener := &interfaceListener{
		interfaceName: interfaceName,
		socket:        socket,
		isRunning:     false,
		stopChan:      make(chan bool, 1),
		buffer:        buffer,
		logger:        cml.logger,
	}

	cml.listeners[interfaceName] = listener

	// Start listening goroutine
	go cml.listenOnInterface(listener)

	cml.logger.Printf("✅ Started listening on %s", interfaceName)
	return nil
}

// StopListening stops listening on a specific interface
func (cml *CanMessageListener) StopListening(interfaceName string) error {
	cml.buffersMutex.Lock()
	defer cml.buffersMutex.Unlock()

	listener, exists := cml.listeners[interfaceName]
	if !exists {
		return fmt.Errorf("not listening on interface %s", interfaceName)
	}

	cml.logger.Printf("🛑 Stopping listener for %s", interfaceName)

	// Signal stop
	if listener.isRunning {
		listener.stopChan <- true
	}

	// Close socket
	if err := unix.Close(listener.socket); err != nil {
		cml.logger.Printf("⚠️ Warning: failed to close listening socket for %s: %v", interfaceName, err)
	}

	// Remove from listeners map
	delete(cml.listeners, interfaceName)

	cml.logger.Printf("✅ Stopped listening on %s", interfaceName)
	return nil
}

// listenOnInterface performs the actual message listening for an interface
func (cml *CanMessageListener) listenOnInterface(listener *interfaceListener) {
	listener.isRunning = true
	defer func() {
		listener.isRunning = false
	}()

	cml.logger.Printf("👂 Listening thread started for %s", listener.interfaceName)

	buffer := make([]byte, 16) // Size of CAN frame

	for {
		select {
		case <-listener.stopChan:
			cml.logger.Printf("🛑 Stop signal received for %s", listener.interfaceName)
			return
		case <-cml.ctx.Done():
			cml.logger.Printf("🛑 Context cancelled for %s", listener.interfaceName)
			return
		default:
			// Set read timeout to avoid blocking indefinitely
			tv := unix.Timeval{Sec: 1, Usec: 0}
			if err := unix.SetsockoptTimeval(listener.socket, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv); err != nil {
				cml.logger.Printf("⚠️ Failed to set socket timeout for %s: %v", listener.interfaceName, err)
			}

			// Try to read CAN frame
			n, err := unix.Read(listener.socket, buffer)
			if err != nil {
				// Check if it's a timeout (expected) or real error
				if errno, ok := err.(unix.Errno); ok && errno == unix.EAGAIN {
					continue // Timeout, continue listening
				}
				cml.logger.Printf("❌ Read error on %s: %v", listener.interfaceName, err)
				continue
			}

			if n >= 16 { // Minimum CAN frame size
				// Parse CAN frame
				frame := (*CanFrame)(unsafe.Pointer(&buffer[0]))

				// Create message log entry
				data := make([]byte, frame.Length)
				copy(data, frame.Data[:frame.Length])

				msg := CanMessageLog{
					Interface: listener.interfaceName,
					ID:        frame.ID,
					Data:      data,
					Length:    frame.Length,
					Timestamp: time.Now(),
					Direction: "RX",
				}

				// Add to buffer
				listener.buffer.AddMessage(msg)

				// Log received message (with rate limiting to avoid spam)
				if listener.buffer.totalReceived%100 == 1 || listener.buffer.totalReceived <= 10 {
					cml.logger.Printf("📨 %s RX: ID=0x%X, Data=[% X], Length=%d",
						listener.interfaceName, msg.ID, msg.Data, msg.Length)
				}
			}
		}
	}
}

// GetMessages returns messages for a specific interface
func (cml *CanMessageListener) GetMessages(interfaceName string) ([]CanMessageLog, error) {
	cml.buffersMutex.RLock()
	defer cml.buffersMutex.RUnlock()

	buffer, exists := cml.buffers[interfaceName]
	if !exists {
		return nil, fmt.Errorf("no message buffer for interface %s", interfaceName)
	}

	return buffer.GetMessages(), nil
}

// GetRecentMessages returns the last N messages for a specific interface
func (cml *CanMessageListener) GetRecentMessages(interfaceName string, count int) ([]CanMessageLog, error) {
	cml.buffersMutex.RLock()
	defer cml.buffersMutex.RUnlock()

	buffer, exists := cml.buffers[interfaceName]
	if !exists {
		return nil, fmt.Errorf("no message buffer for interface %s", interfaceName)
	}

	return buffer.GetRecentMessages(count), nil
}

// GetAllMessages returns messages for all interfaces
func (cml *CanMessageListener) GetAllMessages() map[string][]CanMessageLog {
	cml.buffersMutex.RLock()
	defer cml.buffersMutex.RUnlock()

	result := make(map[string][]CanMessageLog)
	for ifName, buffer := range cml.buffers {
		result[ifName] = buffer.GetMessages()
	}
	return result
}

// GetStatistics returns statistics for all interfaces
func (cml *CanMessageListener) GetStatistics() map[string]interface{} {
	cml.buffersMutex.RLock()
	defer cml.buffersMutex.RUnlock()

	result := make(map[string]interface{})
	for ifName, buffer := range cml.buffers {
		result[ifName] = buffer.GetStatistics()
	}
	return result
}

// GetInterfaceStatistics returns statistics for a specific interface
func (cml *CanMessageListener) GetInterfaceStatistics(interfaceName string) (map[string]interface{}, error) {
	cml.buffersMutex.RLock()
	defer cml.buffersMutex.RUnlock()

	buffer, exists := cml.buffers[interfaceName]
	if !exists {
		return nil, fmt.Errorf("no message buffer for interface %s", interfaceName)
	}

	return buffer.GetStatistics(), nil
}

// ClearMessages clears message buffer for a specific interface
func (cml *CanMessageListener) ClearMessages(interfaceName string) error {
	cml.buffersMutex.RLock()
	defer cml.buffersMutex.RUnlock()

	buffer, exists := cml.buffers[interfaceName]
	if !exists {
		return fmt.Errorf("no message buffer for interface %s", interfaceName)
	}

	buffer.Clear()
	cml.logger.Printf("🧹 Cleared message buffer for %s", interfaceName)
	return nil
}

// ClearAllMessages clears message buffers for all interfaces
func (cml *CanMessageListener) ClearAllMessages() {
	cml.buffersMutex.RLock()
	defer cml.buffersMutex.RUnlock()

	for ifName, buffer := range cml.buffers {
		buffer.Clear()
		cml.logger.Printf("🧹 Cleared message buffer for %s", ifName)
	}
}

// IsListening checks if currently listening on an interface
func (cml *CanMessageListener) IsListening(interfaceName string) bool {
	cml.buffersMutex.RLock()
	defer cml.buffersMutex.RUnlock()

	listener, exists := cml.listeners[interfaceName]
	return exists && listener.isRunning
}

// GetListeningInterfaces returns list of interfaces currently being listened to
func (cml *CanMessageListener) GetListeningInterfaces() []string {
	cml.buffersMutex.RLock()
	defer cml.buffersMutex.RUnlock()

	var interfaces []string
	for ifName, listener := range cml.listeners {
		if listener.isRunning {
			interfaces = append(interfaces, ifName)
		}
	}
	return interfaces
}

// Shutdown stops all listeners and cleans up resources
func (cml *CanMessageListener) Shutdown() error {
	cml.logger.Printf("🛑 Shutting down CAN message listener...")

	// Cancel context
	cml.cancel()

	// Stop all listeners
	cml.buffersMutex.Lock()
	defer cml.buffersMutex.Unlock()

	var errors []string
	for ifName := range cml.listeners {
		if err := cml.stopListeningUnsafe(ifName); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", ifName, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	cml.logger.Printf("✅ CAN message listener shutdown complete")
	return nil
}

// stopListeningUnsafe stops listening without acquiring mutex (internal use)
func (cml *CanMessageListener) stopListeningUnsafe(interfaceName string) error {
	listener, exists := cml.listeners[interfaceName]
	if !exists {
		return fmt.Errorf("not listening on interface %s", interfaceName)
	}

	// Signal stop
	if listener.isRunning {
		listener.stopChan <- true
	}

	// Close socket
	if err := unix.Close(listener.socket); err != nil {
		cml.logger.Printf("⚠️ Warning: failed to close listening socket for %s: %v", interfaceName, err)
	}

	// Remove from listeners map
	delete(cml.listeners, interfaceName)

	return nil
}
