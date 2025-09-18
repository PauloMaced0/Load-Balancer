package load_balancer

import (
	"sync"
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
	servers		[]string
	avgTime		map[string]float64
	startTimes	map[string]chan time.Time // FIFO of start times per server
	pastTimes	map[string][]float64
	current		int
	mu			sync.Mutex
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
		current: -1,
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

	for range len(p.servers) {
		p.current = (p.current + 1) % len(p.servers)
		if p.avgTime[p.servers[p.current]] == p.avgTime[selected] {
			break
		}
	}

	// push start time into its FIFO channel
	now := time.Now()
	select {
	case p.startTimes[p.servers[p.current]] <- now:
		// ok
	default:
		// in unlikely event channel full, use non-blocking fallback (drop oldest)
		// try to drain one and then push
		select {
		case <-p.startTimes[p.servers[p.current]]:
		default:
		}
		p.startTimes[p.servers[p.current]] <- now
	}
	p.mu.Unlock()
	return p.servers[p.current]
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

