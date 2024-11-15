package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Add this struct at package level
type ContainerConfig struct {
	Image string `json:"Image"`
}

var (
	listenAddr string
	targetAddr string
	debug      bool
)

func init() {
	flag.StringVar(&listenAddr, "listen", "localhost:8080", "Address to listen on (format: host:port or unix:/path/to/socket)")
	flag.StringVar(&targetAddr, "target", "localhost:8081", "Address to forward to (format: host:port or unix:/path/to/socket)")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
}

// determineNetwork returns the network type and actual address based on the input
func determineNetwork(addr string) (network, address string) {
	if strings.HasPrefix(addr, "unix:") {
		return "unix", strings.TrimPrefix(addr, "unix:")
	}
	return "tcp", addr
}

func createListener(addr string) (net.Listener, error) {
	network, address := determineNetwork(addr)

	// If it's a unix socket, try to clean up any existing socket file
	if network == "unix" {
		cleanupUnixSocket(address)
	}

	return net.Listen(network, address)
}

func dialTarget(addr string) (net.Conn, error) {
	network, address := determineNetwork(addr)
	return net.Dial(network, address)
}

func cleanupUnixSocket(path string) error {
	return nil // Placeholder for actual cleanup if needed
}

func recordNewContainerCreation(image string) {
	log.Printf("Container creation: %s\n", image)
}

func recordImagePull(image string) {
	log.Printf("Image pull: %s\n", image)
}

func findContentLength(accumulator []byte) (int, int64) {
	// Find Content-Length header
	var headerEnd int = -1
	var expectedLength int64 = -1
	if i := bytes.Index(accumulator, []byte("\r\n\r\n")); i >= 0 {
		headerEnd = i + 4
		// Parse headers to get content length
		req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(accumulator[:headerEnd])))
		if err == nil {
			expectedLength = req.ContentLength
		}
	}

	return headerEnd, expectedLength
}

func findImageFromRequest(accumulator []byte) string {
	var requestedImage string

	// Parse complete request
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(accumulator)))
	if err == nil {
		// Read and parse body
		body, err := io.ReadAll(req.Body)
		if err == nil {
			var config ContainerConfig
			if err := json.Unmarshal(body, &config); err == nil {
				requestedImage = config.Image
			}
		}
		req.Body.Close()
	}

	return requestedImage
}

func handleConnection(client net.Conn, targetAddr string, connId int) {
	defer client.Close()

	// Connect to target server
	target, err := dialTarget(targetAddr)
	if err != nil {
		slog.Error(fmt.Sprintf("[%d] Failed to connect to target: %v", connId, err))
		return
	}
	defer target.Close()

	// Create channels to synchronize the goroutines
	done := make(chan bool, 2)

	var requestedImage string
	var isCreateContainer = false
	var isPullImage = false

	// Forward client -> target with inspection
	go func() {
		var accumulator []byte
		buffer := make([]byte, 4096)
		var headerEnd int = -1
		var expectedLength int64 = -1

		for {
			n, err := client.Read(buffer)
			if err != nil {
				if err != io.EOF {
					slog.Debug(fmt.Sprintf("[%d:REQ] Error reading from client: %s\n", connId, err))
				}
				done <- true
				return
			}

			// Get first line until \r\n
			data := buffer[:n]
			firstLineEnd := bytes.Index(data, []byte("\r\n"))
			if firstLineEnd == -1 {
				firstLineEnd = n
			}
			firstLine := string(data[:firstLineEnd])

			// Parse POST requests
			if strings.HasPrefix(firstLine, "POST") {
				slog.Debug(fmt.Sprintf("[%d:REQ] Request: %s\n", connId, firstLine))
				req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(data[:n])))
				if err == nil {
					// Check if it's a container creation request
					if strings.HasSuffix(req.URL.Path, "/containers/create") {
						isCreateContainer = true
						slog.Debug(fmt.Sprintf("[%d:REQ] Detected container creation request\n", connId))
					}

					if strings.HasSuffix(req.URL.Path, "/images/create") {
						var image = req.URL.Query().Get("fromImage")
						if image != "" {
							image, err = url.QueryUnescape(image)
						}
						var tag = req.URL.Query().Get("tag")
						isPullImage = true

						requestedImage = fmt.Sprintf("%s:%s", image, tag)
						slog.Debug(fmt.Sprintf("[%d:REQ] Detected image pull request: %s \n", connId, requestedImage))
					}
				}
			}

			if isCreateContainer {
				// Append new data to accumulator
				accumulator = append(accumulator, buffer[:n]...)

				// Find headers end if not found yet
				if headerEnd == -1 {
					headerEnd, expectedLength = findContentLength(accumulator)
				}

				// Check if we have complete request
				if headerEnd != -1 && expectedLength != -1 {
					if int64(len(accumulator)-headerEnd) >= expectedLength {
						requestedImage = findImageFromRequest(accumulator)

						// Reset for next request
						accumulator = nil
						headerEnd = -1
						expectedLength = -1
					}
				}
			}

			// Forward the data to target
			_, err = target.Write(buffer[:n])
			if err != nil {
				slog.Debug(fmt.Sprintf("[%d:REQ] Error writing to target: %v", connId, err))
				done <- true
				return
			}
		}
	}()

	// Forward target -> client
	go func() {
		buffer := make([]byte, 4096)
		for {

			n, err := target.Read(buffer)
			if err != nil {
				if err != io.EOF {
					slog.Debug(fmt.Sprintf("[%d:RES] Error reading from target: %v", connId, err))
				}
				done <- true
				return
			}

			data := buffer[:n]
			//log.Printf("[%d:RES] Read %d bytes from response", connId, len(string(data)))

			if strings.HasPrefix(string(data), "HTTP") {
				firstLineEnd := bytes.Index(data, []byte("\r\n"))
				if firstLineEnd == -1 {
					firstLineEnd = n
				}
				firstLine := string(data[:firstLineEnd])

				if isCreateContainer || isPullImage {
					slog.Debug(fmt.Sprintf("[%d:RES] HTTP Response: %s", connId, firstLine))

					statusCode, err := strconv.Atoi(firstLine[9:12])
					if err == nil && statusCode >= 200 && statusCode < 300 {
						slog.Debug(fmt.Sprintf("[%d:RES] Status Code: %d", connId, statusCode))

						if isCreateContainer {
							recordNewContainerCreation(requestedImage)
						}

						if isPullImage {
							recordImagePull(requestedImage)
						}
					}
				}

				isCreateContainer = false
				isPullImage = false
				requestedImage = ""
			}

			_, err = client.Write(buffer[:n])
			if err != nil {
				slog.Debug(fmt.Sprintf("[%d:RES] Error writing to client: %v", connId, err))
				done <- true
				return
			}
		}
	}()

	// Wait for either goroutine to finish
	<-done

	slog.Debug(fmt.Sprintf("[%d] Connection from %s closed", connId, client.RemoteAddr()))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
	listener, err := createListener(listenAddr)
	if err != nil {
		log.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	// Log the type of connections being used
	listenNetwork, listenAddress := determineNetwork(listenAddr)
	targetNetwork, targetAddress := determineNetwork(targetAddr)

	log.Printf("Proxy listening on %s://%s", listenNetwork, listenAddress)
	log.Printf("Forwarding to %s://%s", targetNetwork, targetAddress)
	log.Printf("Use Ctrl+C to stop the proxy")

	var connectionCount = 0

	// Accept connections
	for {
		client, err := listener.Accept()
		if err != nil {
			slog.Error(fmt.Sprintf("Failed to accept connection: %v", err))
			continue
		}

		connectionCount += 1

		slog.Debug(fmt.Sprintf("[%d] New connection from %s", connectionCount, client.RemoteAddr()))

		go handleConnection(client, targetAddr, connectionCount)
	}
}
