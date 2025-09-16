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
	"time"
)

// Policy interface
type Policy interface {
	SelectServer() string
	Update(server string)
}

// ---------------- Policies ---------------- //

// N2One: always first server
type N2One struct {
	servers []string
}

func NewN2One(servers []string) *N2One { return &N2One{servers: servers} }

func (p *N2One) SelectServer() string { return p.servers[0] }
func (p *N2One) Update(server string) {}

// RoundRobin
type RoundRobin struct {
	servers []string
	idx     int
	mu      sync.Mutex
}

func NewRoundRobin(servers []string) *RoundRobin { return &RoundRobin{servers: servers} }

func (p *RoundRobin) SelectServer() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.servers[p.idx]
	p.idx = (p.idx + 1) % len(p.servers)
	return s
}
func (p *RoundRobin) Update(server string) {}

// LeastConnections
type LeastConnections struct {
	servers     []string
	connections map[string]int
	mu          sync.Mutex
}

func NewLeastConnections(servers []string) *LeastConnections {
	conn := make(map[string]int, len(servers))
	for _, s := range servers {
		conn[s] = 0
	}
	return &LeastConnections{servers: servers, connections: conn}
}

func (p *LeastConnections) SelectServer() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	// choose min
	min := int(^uint(0) >> 1) // max int
	var selected string
	for _, s := range p.servers {
		if p.connections[s] < min {
			min = p.connections[s]
			selected = s
		}
	}
	// increment
	p.connections[selected]++
	return selected
}

func (p *LeastConnections) Update(server string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.connections[server]; ok && p.connections[server] > 0 {
		p.connections[server]--
	}
}

// LeastResponseTime
type LeastResponseTime struct {
	servers    []string
	avgTime    map[string]float64
	startTimes map[string]chan time.Time // FIFO of start times per server
	pastTimes  map[string][]float64
	mu         sync.Mutex
}

func NewLeastResponseTime(servers []string) *LeastResponseTime {
	avg := make(map[string]float64, len(servers))
	starts := make(map[string]chan time.Time, len(servers))
	past := make(map[string][]float64, len(servers))
	for _, s := range servers {
		avg[s] = 0.0
		// buffered channel to queue start times. buffer large enough for typical concurrency.
		starts[s] = make(chan time.Time, 10000)
		past[s] = []float64{}
	}
	return &LeastResponseTime{
		servers:    servers,
		avgTime:    avg,
		startTimes: starts,
		pastTimes:  past,
	}
}

func (p *LeastResponseTime) SelectServer() string {
	p.mu.Lock()
	// pick server with minimal avgTime (if tie: first occurrence)
	selected := p.servers[0]
	min := p.avgTime[selected]
	for _, s := range p.servers {
		if p.avgTime[s] < min {
			min = p.avgTime[s]
			selected = s
		}
	}
	// push start time into its FIFO channel
	now := time.Now()
	select {
	case p.startTimes[selected] <- now:
		// ok
	default:
		// in unlikely event channel full, use non-blocking fallback (drop oldest)
		// try to drain one and then push
		select {
		case <-p.startTimes[selected]:
		default:
		}
		p.startTimes[selected] <- now
	}
	p.mu.Unlock()
	return selected
}

func (p *LeastResponseTime) Update(server string) {
	// pop a start time, compute elapsed, append to pastTimes, recompute avg
	p.mu.Lock()
	defer p.mu.Unlock()
	ch, ok := p.startTimes[server]
	if !ok {
		return
	}
	var start time.Time
	select {
	case start = <-ch:
		// got start
	default:
		// no start recorded; cannot compute
		return
	}
	elapsed := time.Since(start).Seconds()
	p.pastTimes[server] = append(p.pastTimes[server], elapsed)
	// recompute avg
	sum := 0.0
	for _, v := range p.pastTimes[server] {
		sum += v
	}
	p.avgTime[server] = sum / float64(len(p.pastTimes[server]))
}

// ---------------- Proxy and main ---------------- //

var (
	activeWG sync.WaitGroup
	logger   = log.New(os.Stdout, "", log.LstdFlags)
)

// handle single client connection: pick backend, proxy bidirectionally, update policy when done
func handleClient(conn net.Conn, policy Policy) {
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
	fmt.Println(servers)

	// init chosen policy
	var policy Policy
	switch *policyName {
	case "N2One":
		policy = NewN2One(servers)
	case "RoundRobin":
		policy = NewRoundRobin(servers)
	case "LeastConnections":
		policy = NewLeastConnections(servers)
	case "LeastResponseTime":
		policy = NewLeastResponseTime(servers)
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
