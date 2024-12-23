package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
)

// const logFile = "loader_logs.json"

// LogEntry represents a log structure
type LogEntry struct {
	ID        int    `json:"id"`
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Host      string `json:"host"`
	Details   string `json:"details"`
}

var (
	logFile      = "loades_logs.json"
	logCounter   = 0
	logCounterMu sync.Mutex
	server       *http.Server
	startTime    time.Time
)

// incrementLogCounter safely increments and returns the log counter
func incrementLogCounter() int {
	logCounterMu.Lock()
	defer logCounterMu.Unlock()
	logCounter++
	return logCounter
}

// WriteLog writes an action to the log file with a unique ID
func WriteLog(action, host, details string) {
	if host == "::1" {
		host = "localhost"
	} else if host == "127.0.0.1" || host == "localhost" {
		host = "localhost"
	}

	logID := incrementLogCounter()
	logEntry := LogEntry{
		ID:        logID,
		Timestamp: time.Now().Format(time.RFC3339),
		Action:    action,
		Host:      host,
		Details:   details,
	}

	prettyJSON, err := json.MarshalIndent(logEntry, "","    ")
	if err != nil {
		log.Printf("Error formating log entry: %v", err)
	}

	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Error opening log file: %v", err)
		return
	}
	defer file.Close()

	file.Write(prettyJSON)
	file.WriteString("\n")
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	WriteLog(r.URL.Path, "localhost", "Service status requested")
	log.Println("Checking service status...")

	response := map[string]interface{}{
		"status": "running",
		"uptime": time.Since(startTime).String(),
		"message": "Service is running smothly",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func handleLoad(w http.ResponseWriter, r *http.Request) {
	WriteLog(r.URL.Path, "localhost", "Load processed")
	log.Println("Processing load...")

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	cpuPercents, _ := cpu.Percent(0, false)

	response := map[string]interface{}{
		"status": "success",
		// "message": "Load processed successfully",
		"cpu":map[string]interface{}{
			"load_percent": cpuPercents[0],
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	WriteLog(r.URL.Path, "localhost", "Service stop requested")
	log.Println("Stopping the service...")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status"}: "stopping", "message": "Service is shutting down"}`))

	go func ()  {
		if err := server.Close(); err != nil {
			log.Fatalf("Error shutting down server: %v", err)
		}
		os.Exit(0)
	}()
}


// func simulateWorkload(w http.ResponseWriter, r *http.Request) {
// 	load := rand.Intn(100) + 1 // Simulate random workload percentage
// 	log.Printf("Simulated workload: %d%%\n", load)
// 	WriteLog("SimulateWorkload", load)
// 	fmt.Fprintf(w, "Simulated workload: %d%%", load)
// }

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/load", handleLoad)
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/stop", handleStop)

	startTime = time.Now()

	server = &http.Server{
		Addr:   ":8082",
		Handler: mux,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Println("Loades running on port 8082")
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stopChan
	log.Println("Shutting down server gracefully...")

	// rand.Seed(time.Now().UnixNano())
	// http.HandleFunc("/load", handleLoad)

	// log.Println("Loader running on port 8082")
	// log.Fatal(http.ListenAndServe(":8082", nil))
}
