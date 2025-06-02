package main

import (
	"encoding/json"
	"log"
	"net"
	"strings"
	"time"
)

// DeviceInfo represents information about the device
type DeviceInfo struct {
	Name    string `json:"name"`
	IP      string `json:"ip"`
	MAC     string `json:"mac"`
	Model   string `json:"model"`
	Version string `json:"version"`
}

func NodeFinder() {
	broadcastAddr := "255.255.255.255:9999"

	conn, err := net.DialUDP("udp4", nil, resolveUDPAddr(broadcastAddr))
	if err != nil {
		log.Fatalf("❌ Failed to connect to broadcast address: %v", err)
	}
	defer conn.Close()

	localIP, mac := getLocalIPAndMAC()
	device := DeviceInfo{
		Name:    "Can-Bridge",
		IP:      localIP,
		MAC:     mac,
		Model:   "LinkerHand OSS",
		Version: VERSION,
	}

	for {
		data, err := json.Marshal(device)
		if err != nil {
			log.Printf("⚠️ JSON serialization error: %v", err)
			continue
		}

		_, err = conn.Write(data)
		if err != nil {
			log.Printf("❌ Broadcast failed: %v", err)
		} else {
			log.Printf("📡 Broadcast successful: %s", string(data))
		}

		time.Sleep(5 * time.Second)
	}
}

// resolveUDPAddr resolves a UDP address from string
func resolveUDPAddr(addr string) *net.UDPAddr {
	udpAddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		log.Fatalf("❌ Failed to resolve address: %v", err)
	}
	return udpAddr
}

// getLocalIPAndMAC retrieves the IPv4 address and corresponding MAC address of the local device
func getLocalIPAndMAC() (string, string) {
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Fatalf("❌ Failed to get network interfaces: %v", err)
	}

	for _, iface := range interfaces {
		// Skip invalid and loopback interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if ok && ipnet.IP.To4() != nil {
				mac := formatMACAddress(iface.HardwareAddr)
				return ipnet.IP.String(), mac
			}
		}
	}

	return "", ""
}

// formatMACAddress formats MAC address into a standard string representation
func formatMACAddress(hwAddr net.HardwareAddr) string {
	return strings.ToUpper(hwAddr.String())
}
