package load_balancer_test

import (
	"Load-Balancer/pkg/load_balancer"
	"testing"
	"time"
)

var servers = []string{
	"localhost:5000",
	"localhost:5001",
	"localhost:5002",
	"localhost:5003",
}

// releaseSocket yields a fixed sequence of finished servers.
func releaseSocket() func() string {
	peers := []string{"localhost:5000", "localhost:5003", "localhost:5002"}
	i := 0
	return func() string {
		val := peers[i%len(peers)]
		i++
		return val
	}
}

func TestN2One(t *testing.T) {
	p := load_balancer.NewN2One(servers)

	var res []string
	for range 8 {
		res = append(res, p.SelectServer())
	}

	expected := []string{
		"localhost:5000", "localhost:5000", "localhost:5000", "localhost:5000",
		"localhost:5000", "localhost:5000", "localhost:5000", "localhost:5000",
	}
	if !equal(res, expected) {
		t.Errorf("got %v, want %v", res, expected)
	}
}

func TestRoundRobin(t *testing.T) {
	p := load_balancer.NewRoundRobin(servers)

	var res []string
	for range 8 {
		res = append(res, p.SelectServer())
	}

	expected := []string{
		"localhost:5000", "localhost:5001", "localhost:5002", "localhost:5003",
		"localhost:5000", "localhost:5001", "localhost:5002", "localhost:5003",
	}
	if !equal(res, expected) {
		t.Errorf("got %v, want %v", res, expected)
	}
}

func TestRoundRobinUpdate(t *testing.T) {
	p := load_balancer.NewRoundRobin(servers)

	var res []string
	next := releaseSocket()
	for i := range 8 {
		res = append(res, p.SelectServer())
		if i > 3 {
			p.Update(next())
		}
	}

	expected := []string{
		"localhost:5000", "localhost:5001", "localhost:5002", "localhost:5003",
		"localhost:5000", "localhost:5001", "localhost:5002", "localhost:5003",
	}
	if !equal(res, expected) {
		t.Errorf("got %v, want %v", res, expected)
	}
}

func TestLeastConnections(t *testing.T) {
	p := load_balancer.NewLeastConnections(servers)

	var res []string
	for range 8 {
		res = append(res, p.SelectServer())
	}

	expected := []string{
		"localhost:5000", "localhost:5001", "localhost:5002", "localhost:5003",
		"localhost:5000", "localhost:5001", "localhost:5002", "localhost:5003",
	}
	if !equal(res, expected) {
		t.Errorf("got %v, want %v", res, expected)
	}
}

func TestLeastConnectionsUpdate(t *testing.T) {
	p := load_balancer.NewLeastConnections(servers)

	var res []string
	next := releaseSocket()
	for i := range 8 {
		res = append(res, p.SelectServer())
		if i > 3 {
			p.Update(next())
		}
	}

	expected := []string{
		"localhost:5000", "localhost:5001", "localhost:5002", "localhost:5003",
		"localhost:5000", "localhost:5000", "localhost:5003", "localhost:5002",
	}
	if !equal(res, expected) {
		t.Errorf("got %v, want %v", res, expected)
	}
}

func TestLeastResponseTime(t *testing.T) {
	p := load_balancer.NewLeastResponseTime(servers)

	var res []string
	for range 8 {
		res = append(res, p.SelectServer())
	}

	expected := []string{
		"localhost:5000", "localhost:5001", "localhost:5002", "localhost:5003",
		"localhost:5000", "localhost:5001", "localhost:5002", "localhost:5003",
	}
	if !equal(res, expected) {
		t.Errorf("got %v, want %v", res, expected)
	}
}

func TestLeastResponseTimeUpdate(t *testing.T) {
	p := load_balancer.NewLeastResponseTime(servers)

	var res []string
	next := releaseSocket()
	for i := range 8 {
		res = append(res, p.SelectServer())
		if i > 3 {
			p.Update(next())
		}
		time.Sleep(100 * time.Millisecond) // simulate elapsed time
	}

	expected := []string{
		"localhost:5000", "localhost:5001", "localhost:5002", "localhost:5003",
		"localhost:5000", "localhost:5001", "localhost:5002", "localhost:5001",
	}
	if !equal(res, expected) {
		t.Errorf("got %v, want %v", res, expected)
	}
}

// helper to compare two string slices
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
