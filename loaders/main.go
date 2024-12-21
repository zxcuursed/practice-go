package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
	"sync"
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
	logFile      = "loader_logs.json"
	logCounter   = 0
	logCounterMu sync.Mutex
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


func handleLoad(w http.ResponseWriter, r *http.Request) {
	WriteLog("HandleLoad", "localhost", "Load processed")
	log.Println("Processing load...")
	w.WriteHeader(http.StatusOK)
}


// func simulateWorkload(w http.ResponseWriter, r *http.Request) {
// 	load := rand.Intn(100) + 1 // Simulate random workload percentage
// 	log.Printf("Simulated workload: %d%%\n", load)
// 	WriteLog("SimulateWorkload", load)
// 	fmt.Fprintf(w, "Simulated workload: %d%%", load)
// }

func main() {
	rand.Seed(time.Now().UnixNano())
	http.HandleFunc("/load", handleLoad)

	log.Println("Loader running on port 8082")
	log.Fatal(http.ListenAndServe(":8082", nil))
}
