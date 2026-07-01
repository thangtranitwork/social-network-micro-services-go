package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	targetURL := flag.String("url", "http://localhost:11111/health", "Target URL to spam")
	concurrency := flag.Int("c", 10, "Number of concurrent workers")
	totalRequests := flag.Int("n", 200, "Total requests to send (0 for infinite)")
	delayMs := flag.Int("delay", 10, "Delay in milliseconds between requests per worker")
	flag.Parse()

	fmt.Printf("=============================================\n")
	fmt.Printf("   Microservices API Spammer (Load Tester)\n")
	fmt.Printf("=============================================\n")
	fmt.Printf("Target URL:      %s\n", *targetURL)
	fmt.Printf("Concurrency:     %d workers\n", *concurrency)
	fmt.Printf("Total Requests:  %d\n", *totalRequests)
	fmt.Printf("Worker Delay:    %d ms\n", *delayMs)
	fmt.Printf("=============================================\n\n")

	var (
		wg           sync.WaitGroup
		success      int64
		tooManyReqs  int64 // 429
		banned       int64 // 403
		otherErrors  int64
		reqCompleted int64
	)

	startTime := time.Now()

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{
				Timeout: 2 * time.Second,
			}

			for {
				// Check limit
				if *totalRequests > 0 {
					current := atomic.AddInt64(&reqCompleted, 1)
					if current > int64(*totalRequests) {
						break
					}
				}

				req, err := http.NewRequest("GET", *targetURL, nil)
				if err != nil {
					atomic.AddInt64(&otherErrors, 1)
					continue
				}

				// Generate randomized client IP headers to simulate different users
				// if testing from a single machine without auth
				simulatedIP := fmt.Sprintf("192.168.10.%d", (workerID%250)+1)
				req.Header.Set("X-Forwarded-For", simulatedIP)
				req.Header.Set("X-Real-IP", simulatedIP)

				resp, err := client.Do(req)
				if err != nil {
					atomic.AddInt64(&otherErrors, 1)
				} else {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()

					switch resp.StatusCode {
					case http.StatusOK:
						atomic.AddInt64(&success, 1)
					case http.StatusTooManyRequests:
						atomic.AddInt64(&tooManyReqs, 1)
					case http.StatusForbidden:
						atomic.AddInt64(&banned, 1)
					default:
						atomic.AddInt64(&otherErrors, 1)
					}
				}

				if *delayMs > 0 {
					time.Sleep(time.Duration(*delayMs) * time.Millisecond)
				}
			}
		}(i)
	}

	// Progress reporter goroutine
	stopReporter := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s := atomic.LoadInt64(&success)
				tm := atomic.LoadInt64(&tooManyReqs)
				b := atomic.LoadInt64(&banned)
				e := atomic.LoadInt64(&otherErrors)
				fmt.Printf("\rProgress - Success (200): %d | Too Many Requests (429): %d | Banned (403): %d | Errors: %d", s, tm, b, e)
			case <-stopReporter:
				return
			}
		}
	}()

	wg.Wait()
	close(stopReporter)

	duration := time.Since(startTime)
	fmt.Printf("\n\n================- RESULTS -================\n")
	fmt.Printf("Duration:               %v\n", duration)
	fmt.Printf("Success (200 OK):       %d\n", success)
	fmt.Printf("Rate Limited (429):     %d\n", tooManyReqs)
	fmt.Printf("Spam Banned (403):      %d\n", banned)
	fmt.Printf("Network/Other Errors:   %d\n", otherErrors)
	fmt.Printf("Total Requests Sent:    %d\n", success+tooManyReqs+banned+otherErrors)
	fmt.Printf("===========================================\n")
}
