package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"

	"onigoetz.ch/docker-proxy/lib"
)

var (
	listenAddr string
	targetAddr string
	debug      bool

	influxHost   string
	influxToken  string
	influxOrg    string
	influxBucket string
)

func init() {
	flag.StringVar(&listenAddr, "listen", "localhost:8080", "Address to listen on (format: host:port or unix:/path/to/socket)")
	flag.StringVar(&targetAddr, "target", "unix:/var/run/docker.sock", "Address to forward to (format: host:port or unix:/path/to/socket)")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")

	flag.StringVar(&influxHost, "influx-host", "http://localhost:8086", "InfluxDB host")
	flag.StringVar(&influxToken, "influx-token", "admin-token", "InfluxDB token")
	flag.StringVar(&influxOrg, "influx-org", "my-org", "InfluxDB organization")
	flag.StringVar(&influxBucket, "influx-bucket", "my-bucket", "InfluxDB bucket")
}

func main() {
	// Parse command line flags
	flag.Parse()

	// Validate addresses
	if listenAddr == "" || targetAddr == "" {
		log.Fatal("Both listen and target addresses must be specified")
	}

	if debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("Debug mode enabled")
	}

	// Create listener
	listener, err := lib.CreateListener(listenAddr)
	if err != nil {
		log.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	// Log the type of connections being used
	listenNetwork, listenAddress := lib.DetermineNetwork(listenAddr)
	targetNetwork, targetAddress := lib.DetermineNetwork(targetAddr)

	log.Printf("Proxy listening on %s://%s", listenNetwork, listenAddress)
	log.Printf("Forwarding to %s://%s", targetNetwork, targetAddress)
	log.Printf("Use Ctrl+C to stop the proxy")

	var connectionCount = 0

	config := lib.InfluxConfig{Url: "http://localhost:8086", Token: "admin-token", Org: "my-org", Bucket: "my-bucket"}

	// Accept connections
	for {
		client, err := listener.Accept()
		if err != nil {
			slog.Error(fmt.Sprintf("Failed to accept connection: %v", err))
			continue
		}

		connectionCount += 1

		slog.Debug(fmt.Sprintf("[%d] New connection from %s", connectionCount, client.RemoteAddr()))

		go lib.HandleConnection(client, targetAddr, connectionCount, config)
	}
}
