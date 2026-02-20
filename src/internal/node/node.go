package node

import (
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/google/uuid"
	"gorm.io/gorm/logger"
)

type Config struct {
	Port         int
	Bootstrap    string
	Fanout       int
	TTL          int
	PeerLimit    int
	PingInterval int
	PeerTimeout  int
	Seed         int64
}

type Node struct {
	cfg      Config
	selfAddr string
	uuid 	 uuid.UUID

	mu       sync.RWMutex
	listener net.Listener
}

func New(cfg Config) (*Node, error) {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		slog.Error("invalid port: %d", cfg.Port)
		return nil, fmt.Errorf("invalid port: %d", cfg.Port)
	}

	n := &Node{
		cfg:    	cfg,
		selfAddr: 	fmt.Sprintf("127.0.0.1:%d", cfg.Port),
		uuid:   	uuid.New(),
	}

	logger.Info("Node created", slog.Any("uuid", n.uuid))

	return n, nil
}

