package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/takutakahashi/capps/internal/config"
	"github.com/takutakahashi/capps/internal/server"
)

var (
	flagGatewayURL        string
	flagToken             string
	flagPassword          string
	flagListenAddr        string
	flagLogLevel          string
	flagRequestTimeout    time.Duration
	flagReconnectInterval time.Duration
	flagReconnectMaxRetry int
)

var rootCmd = &cobra.Command{
	Use:   "capps",
	Short: "WebSocket gateway to REST API bridge for openclaw",
	Long: `capps bridges the openclaw WebSocket gateway protocol to a REST API.

Any openclaw gateway RPC method can be called via:

  POST /call/<method>

where <method> uses slash separators that are mapped to dots:

  POST /call/sessions/list   →  sessions.list
  POST /call/config/get      →  config.get
  POST /call/health          →  health

Environment variables:
  OPENCLAW_GATEWAY_URL      WebSocket URL of the openclaw gateway (required)
  OPENCLAW_GATEWAY_TOKEN    Token for authentication (required unless --password)
  OPENCLAW_GATEWAY_PASSWORD Password for authentication (required unless --token)
  CAPPS_LISTEN_ADDR         HTTP listen address (default ":8080")
  CAPPS_LOG_LEVEL           Log level: debug/info/warn/error (default "info")
  CAPPS_REQUEST_TIMEOUT     Per-call timeout (default "30s")
  CAPPS_RECONNECT_INTERVAL  Reconnect backoff interval (default "5s")
  CAPPS_RECONNECT_MAX_RETRY Max reconnect attempts, 0=unlimited (default 0)
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadWithOverrides(
			flagGatewayURL,
			flagToken,
			flagPassword,
			flagListenAddr,
			flagLogLevel,
			flagRequestTimeout,
			flagReconnectInterval,
			flagReconnectMaxRetry,
		)
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		srv, err := server.New(cfg)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		return srv.Start(ctx)
	},
}

func init() {
	rootCmd.Flags().StringVar(&flagGatewayURL, "gateway-url", "", "WebSocket URL of the openclaw gateway (overrides OPENCLAW_GATEWAY_URL)")
	rootCmd.Flags().StringVar(&flagToken, "token", "", "Token for gateway authentication (overrides OPENCLAW_GATEWAY_TOKEN)")
	rootCmd.Flags().StringVar(&flagPassword, "password", "", "Password for gateway authentication (overrides OPENCLAW_GATEWAY_PASSWORD)")
	rootCmd.Flags().StringVar(&flagListenAddr, "listen", "", "HTTP server listen address (default \":8080\")")
	rootCmd.Flags().StringVar(&flagLogLevel, "log-level", "", "Log level: debug, info, warn, error (default \"info\")")
	rootCmd.Flags().DurationVar(&flagRequestTimeout, "request-timeout", 0, "Per-call RPC timeout (default 30s)")
	rootCmd.Flags().DurationVar(&flagReconnectInterval, "reconnect-interval", 0, "Reconnect backoff interval (default 5s)")
	rootCmd.Flags().IntVar(&flagReconnectMaxRetry, "reconnect-max-retry", 0, "Max reconnect attempts, 0=unlimited (default 0)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
