package main

import (
	"bufio"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/mohammadaminkoohi/GoGossip/src/internal/node"
)

func setupLogger(level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	logger := slog.New(handler)

	slog.SetDefault(logger)

	return logger
}

func main() {
	cfg := node.Config{}

	var debug bool
	flag.BoolVar(&debug, "debug", false, "Enable debug logs (ping/pong detail, etc.)")

	flag.IntVar(&cfg.Port, "port", 8000, "Listening port for this Node")
	flag.StringVar(&cfg.Bootstrap, "bootstrap", "", "Address of the seed Node")
	flag.IntVar(&cfg.Fanout, "fanout", 3, "Fanout value for this Node")
	flag.IntVar(&cfg.TTL, "ttl", 10, "TTL value for this Node")
	flag.IntVar(&cfg.PeerLimit, "peer-limit", 100, "Peer limit for this Node")
	flag.IntVar(&cfg.PingInterval, "ping-interval", 1000, "Ping interval in milliseconds for this Node")
	flag.IntVar(&cfg.PeerTimeout, "peer-timeout", 5000, "Peer timeout in milliseconds for this Node")
	flag.Int64Var(&cfg.Seed, "seed", 1234567890, "Seed value for this Node")
	flag.StringVar(&cfg.ExperimentLogPath, "experiment-log", "", "Path to append experiment metric lines (JSON)")
	flag.StringVar(&cfg.NeighborsPolicy, "neighbors-policy", "first", "Fanout selection: 'first' or 'random'")
	flag.IntVar(&cfg.PullInterval, "pull-interval", 0, "IHAVE send interval in ms (0=disabled)")
	flag.IntVar(&cfg.IHaveMaxIds, "ihave-max-ids", 32, "Max message IDs per IHAVE message")
	flag.IntVar(&cfg.GossipCacheMaxSize, "gossip-cache-max", 0, "Max cached gossip payloads for IWANT (0=default 2000)")

	flag.Parse()

	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	logger := setupLogger(level)

	logger.Info("configuration loaded", slog.Any("config", cfg))

	n, err := node.New(cfg)
	if err != nil {
		logger.Error("failed to initialize node", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if cfg.Bootstrap != "" {
		logger.Info("starting as peer", slog.String("bootstrap", cfg.Bootstrap), slog.Int("port", cfg.Port))
	} else {
		logger.Info("starting as seed node", slog.Int("port", cfg.Port))
	}

	if err := n.Start(); err != nil {
		logger.Error("node failed to start", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// CLI: each line of input is published as a gossip message
	fmt.Println("Node running. Type a message and press Enter to gossip it (Ctrl+C to exit).")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		n.PublishGossip("chat", line)
	}
	if err := scanner.Err(); err != nil {
		logger.Error("reading stdin", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
