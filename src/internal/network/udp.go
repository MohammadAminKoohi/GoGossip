package network

import (
	"fmt"
	"log/slog"
	"net"
)

// Handler is called for each received UDP packet (from address, payload).
type Handler func(from *net.UDPAddr, data []byte)

// Send sends data to the given UDP address (e.g. "127.0.0.1:8000").
func Send(addr string, data []byte) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("resolve udp address: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return fmt.Errorf("dial udp: %w", err)
	}
	defer conn.Close()
	_, err = conn.Write(data)
	return err
}

func Listen(addr string, handler Handler) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("resolve udp address: %w", err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("listen udp: %w", err)
	}
	defer conn.Close()

	buf := make([]byte, 65535)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			slog.Error("udp read failed", slog.String("error", err.Error()))
			continue
		}
		if n == 0 {
			continue
		}
		payload := make([]byte, n)
		copy(payload, buf[:n])
		handler(from, payload)
	}
}
