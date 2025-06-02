package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// Configuration structure
type Config struct {
	CanPorts []string
	Port     string
}

// ConfigProvider interface for dependency injection
type ConfigProvider interface {
	GetCanPorts() []string
	GetServerPort() string
	ValidateInterface(ifName string) bool
}

// DefaultConfigProvider implements ConfigProvider
type DefaultConfigProvider struct {
	config *Config
}

// NewDefaultConfigProvider creates a new default config provider
func NewDefaultConfigProvider(config *Config) *DefaultConfigProvider {
	return &DefaultConfigProvider{config: config}
}

// GetCanPorts returns configured CAN ports
func (p *DefaultConfigProvider) GetCanPorts() []string {
	return p.config.CanPorts
}

// GetServerPort returns server port
func (p *DefaultConfigProvider) GetServerPort() string {
	return p.config.Port
}

// ValidateInterface checks if interface is in configured ports
func (p *DefaultConfigProvider) ValidateInterface(ifName string) bool {
	for _, port := range p.config.CanPorts {
		if port == ifName {
			return true
		}
	}
	return false
}

// ConfigParser handles parsing configuration from various sources
type ConfigParser struct{}

// NewConfigParser creates a new config parser
func NewConfigParser() *ConfigParser {
	return &ConfigParser{}
}

// ParseConfig parses configuration from command line and environment variables
func (cp *ConfigParser) ParseConfig() (*Config, error) {
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
		config.CanPorts = cp.parseCanPorts(canPortsFlag)
	} else {
		// Default to can0 if no ports specified
		config.CanPorts = []string{"can0"}
	}

	// Validate server port
	if serverPort == "" {
		return nil, fmt.Errorf("server port cannot be empty")
	}
	config.Port = serverPort

	return config, nil
}

// parseCanPorts parses comma-separated CAN ports string
func (cp *ConfigParser) parseCanPorts(portsStr string) []string {
	ports := strings.Split(portsStr, ",")
	// Trim whitespace from each port
	for i, port := range ports {
		ports[i] = strings.TrimSpace(port)
	}
	return ports
}

// ValidateConfig validates the configuration
func (cp *ConfigParser) ValidateConfig(config *Config) error {
	if len(config.CanPorts) == 0 {
		return fmt.Errorf("at least one CAN port must be specified")
	}

	for _, port := range config.CanPorts {
		if strings.TrimSpace(port) == "" {
			return fmt.Errorf("CAN port name cannot be empty")
		}
	}

	if config.Port == "" {
		return fmt.Errorf("server port cannot be empty")
	}

	return nil
}

// PrintUsage prints usage information
func PrintUsage() {
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
