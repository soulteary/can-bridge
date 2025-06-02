package main

import (
	"fmt"
	"time"
	"unsafe"
)

// MessageSender handles sending CAN messages
type MessageSender struct {
	interfaceManager *InterfaceManager
	configProvider   ConfigProvider
	socketProvider   SocketProvider
	logger           Logger
}

// NewMessageSender creates a new message sender
func NewMessageSender(interfaceManager *InterfaceManager, configProvider ConfigProvider, socketProvider SocketProvider, logger Logger) *MessageSender {
	return &MessageSender{
		interfaceManager: interfaceManager,
		configProvider:   configProvider,
		socketProvider:   socketProvider,
		logger:           logger,
	}
}

// SendCanMessage sends a raw CAN message with interface validation
func (ms *MessageSender) SendCanMessage(msg CanMessage) error {
	// Validate interface is configured
	if !ms.configProvider.ValidateInterface(msg.Interface) {
		return fmt.Errorf("CAN interface %s is not configured. Available interfaces: %v",
			msg.Interface, ms.configProvider.GetCanPorts())
	}

	// Get interface
	canIf, ok := ms.interfaceManager.GetInterface(msg.Interface)
	if !ok {
		return fmt.Errorf("CAN interface %s not initialized", msg.Interface)
	}

	// Validate data length
	if len(msg.Data) > 8 {
		return fmt.Errorf("CAN data exceeds maximum length (8 bytes)")
	}

	return ms.sendMessage(canIf, msg)
}

// sendMessage performs the actual message sending
func (ms *MessageSender) sendMessage(canIf *CanInterface, msg CanMessage) error {
	canIf.Lock()
	defer canIf.Unlock()

	startTime := time.Now()

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
	err := ms.socketProvider.SendTo(canIf.FD, buf, canIf.Addr)

	// Update metrics
	if err == nil {
		latency := time.Since(startTime)
		canIf.Metrics.RecordSuccess(latency)

		// Log success
		ms.logger.Printf("✅ %s message sent: ID=0x%X, Data=[% X], Length=%d, Latency=%v",
			msg.Interface, msg.ID, msg.Data, frame.Length, latency)
	} else {
		canIf.Metrics.RecordError(err)

		// Log error
		ms.logger.Printf("❌ %s message send failed: ID=0x%X, Error=%v", msg.Interface, msg.ID, err)
	}

	return err
}

// ValidateMessage validates a CAN message before sending
func (ms *MessageSender) ValidateMessage(msg CanMessage) error {
	if msg.Interface == "" {
		return fmt.Errorf("interface name is required")
	}

	if !ms.configProvider.ValidateInterface(msg.Interface) {
		return fmt.Errorf("CAN interface %s is not configured. Available interfaces: %v",
			msg.Interface, ms.configProvider.GetCanPorts())
	}

	if len(msg.Data) == 0 {
		return fmt.Errorf("message data cannot be empty")
	}

	if len(msg.Data) > 8 {
		return fmt.Errorf("CAN data exceeds maximum length (8 bytes)")
	}

	return nil
}
