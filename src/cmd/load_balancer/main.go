package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"Load-Balancer/pkg/load_balancer"
)

// ---------------- Proxy and main ---------------- //

var (
	activeWG sync.WaitGroup
	logger   = log.New(os.Stdout, "", log.LstdFlags)
)

// handle single client connection: pick backend, proxy bidirectionally, update policy when done
func handleClient(conn net.Conn, policy load_balancer.Policy) {
	defer conn.Close()
	activeWG.Add(1)
	defer activeWG.Done()

	remoteAddr := conn.RemoteAddr().String()

	backend := policy.SelectServer()
	logger.Printf("Selected backend %s for client %s", backend, remoteAddr)

	backendConn, err := net.Dial("tcp", backend)
	if err != nil {
		logger.Printf("ERROR connecting to backend %s: %v", backend, err)
		// If policy is LeastConnections we should decrement because selection incremented; Update handles decrement semantics
		policy.Update(backend)
		return
	}
	defer backendConn.Close()
	logger.Printf("Proxying %s <-> %s", remoteAddr, backend)

	// proxy bidirectionally, track when both sides complete
	var wg sync.WaitGroup
	wg.Add(2)

	// client -> backend
	go func() {
		defer wg.Done()
		_, err := io.Copy(backendConn, conn)
		if err != nil {
			logger.Printf("Copy client->backend error: %v", err)
		}
		// close write to backend so it knows EOF
		if tcp, ok := backendConn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()

	// backend -> client
	go func() {
		defer wg.Done()
		_, err := io.Copy(conn, backendConn)
		if err != nil {
			logger.Printf("Copy backend->client error: %v", err)
		}
		// close write to client
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()

	wg.Wait()

	// connection finished; update policy (decrement counters / measure RTT)
	policy.Update(backend)
	logger.Printf("Connection finished for client %s via backend %s", remoteAddr, backend)
}

func main() {
	// flags
	policyName := flag.String("a", "RoundRobin", "Policy: N2One, RoundRobin, LeastConnections, LeastResponseTime")
	port := flag.Int("p", 8080, "Load balancer port")
	var serversFlag string 
	flag.StringVar(&serversFlag, "s", "", "Backend server in host:port form; can be repeated. Example: -s localhost:5000 -s localhost:5001")
	flag.Parse()

	if len(serversFlag) == 0 {
		logger.Fatalf("No backend servers specified (-s).")
	}

	// prepare server list (strings)
	servers := strings.Fields(serversFlag) 

	// init chosen policy
	var policy load_balancer.Policy
	switch *policyName {
	case "N2One":
		policy = load_balancer.NewN2One(servers)
	case "RoundRobin":
		policy = load_balancer.NewRoundRobin(servers)
	case "LeastConnections":
		policy = load_balancer.NewLeastConnections(servers)
	case "LeastResponseTime":
		policy = load_balancer.NewLeastResponseTime(servers)
	default:
		logger.Fatalf("Unknown policy: %s", *policyName)
	}

	listenAddr := fmt.Sprintf("0.0.0.0:%d", *port)
	l, err := net.Listen("tcp", listenAddr)
	if err != nil {
		logger.Fatalf("Failed to listen on %s: %v", listenAddr, err)
	}
	logger.Printf("Listening on %s, policy=%s, backends=%v", listenAddr, *policyName, servers)

	// graceful shutdown setup
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	acceptDone := make(chan struct{})

	// accept loop in goroutine so we can interrupt
	go func() {
		defer close(acceptDone)
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			// handle connection concurrently
			go handleClient(conn, policy)
		}
	}()

	// wait for signal
	<-sig

	logger.Printf("Graceful shutdown requested. Stopping accepting new connections...")
	// close listener to stop accept loop
	_ = l.Close()
	// wait accept goroutine to finish
	<-acceptDone
	logger.Printf("Waiting for active connections to finish...")
	// wait for active handlers
	activeWG.Wait()
	logger.Printf("Shutdown complete.")
}
