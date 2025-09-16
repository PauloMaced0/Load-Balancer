package main

import (
	"flag"
	"net/http"
	"strconv"
	"sync"
	"time"
	"github.com/gin-gonic/gin"
)

func leibnizPiPrecision(precision int) float64 {
	rv := 1.0
	for i := range precision{
		time.Sleep(1 * time.Millisecond) // simulate slow computation
		if i%2 == 0 {
			rv -= 1.0 / (2*float64(i+1) + 1)
		} else {
			rv += 1.0 / (2*float64(i+1) + 1)
		}
	}
	return rv * 4.0
}

// Global mutex for single-threaded behavior
var mu sync.Mutex

// Middleware: make Gin single-threaded
func singleThreaded() gin.HandlerFunc {
	return func(c *gin.Context) {
		mu.Lock()
		defer mu.Unlock()
		c.Next()
	}
}

func piHandler(c *gin.Context) {
	// Get precision value
	precisionStr := c.Params.ByName("precision")
	precision, err := strconv.Atoi(precisionStr)

	if err != nil {
		c.String(http.StatusBadRequest, "Invalid precision")
		return
	}

	c.HTML(http.StatusOK, "index.html", gin.H{
		"precision": precision,
		"pi": leibnizPiPrecision(precision),
	})
}

func main() {
	port := flag.Int("p", 8080, "HTTP port")
	flag.Parse()

	// Initialize Gin
	r := gin.Default()
	r.StaticFile("/favicon.ico", "../resources/favicon.ico")
	r.LoadHTMLGlob("../templates/*")

	// Apply single-threaded middleware
	r.Use(singleThreaded())
	r.GET("/:precision", piHandler)

	// Listen and Server in 0.0.0.0:8080
	addr := ":" + strconv.Itoa(*port)
	r.Run(addr)
}
