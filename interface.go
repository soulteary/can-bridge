package main

import (
	"fmt"
	"log"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// SocketProvider interface for dependency injection
type SocketProvider interface {
	CreateSocket() (int, error)
	GetIfIndex(fd int, ifname string) (int, error)
	Bind(fd int, addr *unix.SockaddrCAN) error
	SendTo(fd int, buf []byte, addr *unix.SockaddrCAN) error
	Close(fd int) error
}

// UnixSocketProvider implements SocketProvider using real Unix sockets
type UnixSocketProvider struct{}

// NewUnixSocketProvider creates a new Unix socket provider
func NewUnixSocketProvider() *UnixSocketProvider {
	return &UnixSocketProvider{}
}

// CreateSocket creates a new CAN socket
func (p *UnixSocketProvider) CreateSocket() (int, error) {
	return unix.Socket(unix.AF_CAN, unix.SOCK_RAW, unix.CAN_RAW)
}

// GetIfIndex gets CAN interface index
func (p *UnixSocketProvider) GetIfIndex(fd int, ifname string) (int, error) {
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

// Bind binds socket to CAN interface
func (p *UnixSocketProvider) Bind(fd int, addr *unix.SockaddrCAN) error {
	return unix.Bind(fd, addr)
}

// SendTo sends data to CAN interface
func (p *UnixSocketProvider) SendTo(fd int, buf []byte, addr *unix.SockaddrCAN) error {
	return unix.Sendto(fd, buf, 0, addr)
}

// Close closes the socket
func (p *UnixSocketProvider) Close(fd int) error {
	return unix.Close(fd)
}

// InterfaceManager manages CAN interfaces
type InterfaceManager struct {
	interfaces     map[string]*CanInterface
	configProvider ConfigProvider
	socketProvider SocketProvider
	logger         Logger
}

// Logger interface for dependency injection
type Logger interface {
	Printf(format string, v ...interface{})
}

// DefaultLogger implements Logger using standard log package
type DefaultLogger struct{}

func (l *DefaultLogger) Printf(format string, v ...interface{}) {
	log.Printf(format, v...)
}

// NewInterfaceManager creates a new interface manager
func NewInterfaceManager(configProvider ConfigProvider, socketProvider SocketProvider, logger Logger) *InterfaceManager {
	return &InterfaceManager{
		interfaces:     make(map[string]*CanInterface),
		configProvider: configProvider,
		socketProvider: socketProvider,
		logger:         logger,
	}
}

// InitializeAll initializes all CAN interfaces based on configuration
func (im *InterfaceManager) InitializeAll() error {
	ports := im.configProvider.GetCanPorts()
	im.logger.Printf("🔧 Initializing CAN interfaces: %v", ports)

	var lastErr error
	successCount := 0

	for _, ifName := range ports {
		err := im.InitializeSingle(ifName)
		if err != nil {
			lastErr = err
			im.logger.Printf("❌ Failed to initialize %s: %v", ifName, err)
		} else {
			im.logger.Printf("✅ Successfully initialized %s", ifName)
			successCount++
		}
	}

	// If all interfaces failed, return error
	if successCount == 0 {
		return fmt.Errorf("failed to initialize any CAN interface from %v: %v", ports, lastErr)
	}

	im.logger.Printf("🎯 Successfully initialized %d/%d CAN interfaces", successCount, len(ports))
	return nil
}

// InitializeSingle initializes a single CAN interface with retry logic
func (im *InterfaceManager) InitializeSingle(ifName string) error {
	retries := 5
	retryDelay := 2 * time.Second

	for i := 0; i < retries; i++ {
		canIf, err := im.createInterface(ifName)
		if err == nil {
			im.interfaces[ifName] = canIf
			im.logger.Printf("✅ %s initialization successful", ifName)
			return nil
		}

		im.logger.Printf("⚠️ %s initialization attempt %d failed: %v. Retrying in %v...",
			ifName, i+1, err, retryDelay)
		time.Sleep(retryDelay)
	}

	return fmt.Errorf("failed to initialize %s after %d attempts", ifName, retries)
}

// createInterface creates a single CAN interface
func (im *InterfaceManager) createInterface(ifName string) (*CanInterface, error) {
	// Open CAN socket
	fd, err := im.socketProvider.CreateSocket()
	if err != nil {
		return nil, fmt.Errorf("failed to create socket: %w", err)
	}

	// Get CAN interface index
	ifindex, err := im.socketProvider.GetIfIndex(fd, ifName)
	if err != nil {
		im.socketProvider.Close(fd)
		return nil, fmt.Errorf("failed to get interface index: %w", err)
	}

	// Bind to CAN interface
	addr := &unix.SockaddrCAN{Ifindex: ifindex}
	if err = im.socketProvider.Bind(fd, addr); err != nil {
		im.socketProvider.Close(fd)
		return nil, fmt.Errorf("failed to bind to interface: %w", err)
	}

	// Create interface struct
	canIf := NewCanInterface(ifName, fd, addr)
	return canIf, nil
}

// GetInterface returns a CAN interface by name
func (im *InterfaceManager) GetInterface(name string) (*CanInterface, bool) {
	canIf, ok := im.interfaces[name]
	return canIf, ok
}

// GetAllInterfaces returns all interfaces
func (im *InterfaceManager) GetAllInterfaces() map[string]*CanInterface {
	result := make(map[string]*CanInterface)
	for k, v := range im.interfaces {
		result[k] = v
	}
	return result
}

// RemoveInterface removes an interface from the manager
func (im *InterfaceManager) RemoveInterface(name string) error {
	canIf, ok := im.interfaces[name]
	if !ok {
		return fmt.Errorf("interface %s not found", name)
	}

	// Close the socket
	err := im.socketProvider.Close(canIf.FD)
	if err != nil {
		im.logger.Printf("Warning: failed to close socket for %s: %v", name, err)
	}

	// Remove from map
	delete(im.interfaces, name)
	return nil
}

// Cleanup closes all interfaces
func (im *InterfaceManager) Cleanup() {
	im.logger.Printf("🧹 Cleaning up CAN interfaces...")
	for name, canIf := range im.interfaces {
		err := im.socketProvider.Close(canIf.FD)
		if err != nil {
			im.logger.Printf("Warning: failed to close %s: %v", name, err)
		}
	}
	im.interfaces = make(map[string]*CanInterface)
}

// CheckHealth performs a health check on an interface
func (im *InterfaceManager) CheckHealth(ifName string) bool {
	canIf, ok := im.interfaces[ifName]
	if !ok {
		return false
	}

	canIf.Lock()
	defer canIf.Unlock()

	// Simple probe message (0x00 is typically a diagnostic/echo ID)
	frame := CanFrame{
		ID:     0x00,
		Length: 1,
		Data:   [8]byte{0x00},
	}

	buf := (*[16]byte)(unsafe.Pointer(&frame))[:]
	err := im.socketProvider.SendTo(canIf.FD, buf, canIf.Addr)

	if err != nil {
		im.logger.Printf("⚠️ %s health check failed: %v", ifName, err)
		return false
	}

	return true
}

// GetInterfaceCount returns the number of active interfaces
func (im *InterfaceManager) GetInterfaceCount() int {
	return len(im.interfaces)
}

// IsInterfaceActive checks if an interface is active
func (im *InterfaceManager) IsInterfaceActive(name string) bool {
	_, ok := im.interfaces[name]
	return ok
}
