package cmd

import(
	"flag"
	"github.com/mohammadaminkoohi/GoGossip/src/internal/node"
	"log/slog"
	"os"
)

func setupLogger() *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo, 
	})

	logger := slog.New(handler)

	slog.SetDefault(logger)

	return logger
}

func main() {
	cfg := node.Config{}

	logger := setupLogger()

	flag.IntVar(&cfg.Port, "port", 8000, "Listening port for this Node")
	flag.StringVar(&cfg.Bootstrap, "bootstrap","", "Address of the seed Node")
	flag.IntVar(&cfg.Fanout, "fanout", 3, "Fanout value for this Node")
	flag.IntVar(&cfg.TTL, "ttl", 10, "TTL value for this Node")
	flag.IntVar(&cfg.PeerLimit, "peer-limit", 100, "Peer limit for this Node")
	flag.IntVar(&cfg.PingInterval, "ping-interval", 1000, "Ping interval in milliseconds for this Node")
	flag.IntVar(&cfg.PeerTimeout, "peer-timeout", 5000, "Peer timeout in milliseconds for this Node")
	flag.Int64Var(&cfg.Seed, "seed", 1234567890, "Seed value for this Node")

	flag.Parse()

	logger.Info("Configuration loaded", slog.Any("config", cfg))

	_, err := node.New(cfg)
	if err != nil {
		logger.Error("failed to initialize node", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("Node initialized")

}