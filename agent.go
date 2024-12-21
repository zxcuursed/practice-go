package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
	"syscall"
	"sync"
	"strconv"
)

// const logFile = "agent_logs.json"

// LogEntry represents a log structure

type LogEntry struct {
	ID        int    `json:"id"`
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Host      string `json:"host"`
	Details   string `json:"details"`
}

var (
	logFile      = "agent_logs.json"
	logCounter   = 0
	logCounterMu sync.Mutex
)

type Replica struct {
	ID          int `json:"id"`
	ServiceName string `json:"serviceName"`
	Host        string `json:"host"`
}
var replicas = make(map[int]Replica)
var replicaIDCounter = 1

// incrementLogCounter safely increments and returns the log counter
func incrementLogCounter() int {
	logCounterMu.Lock()
	defer logCounterMu.Unlock()
	logCounter++
	return logCounter
}

// // WriteLog writes an action to the log file
// func WriteLog(action, service string, count int) {
// 	logEntry := LogEntry{
// 		Timestamp: time.Now().Format(time.RFC3339),
// 		Action:    action,
// 		Service:   service,
// 		Count:     count,
// 	}
// 	file, _ := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
// 	defer file.Close()
// 	json.NewEncoder(file).Encode(logEntry)
// }
func WriteLog(action, host, details string) {
	logID := incrementLogCounter()
	logEntry := LogEntry{
		ID:        logID,
		Timestamp: time.Now().Format(time.RFC3339),
		Action:    action,
		Host:      host,
		Details:   details,
	}

	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Error opening log file: %v", err)
		return
	}
	defer file.Close()

	prettyJSON, err := json.MarshalIndent(logEntry, "", "    ")
	if err != nil {
		log.Printf("Error formatting log entry: %v", err)
		return
	}

	file.Write(prettyJSON)
	file.WriteString("\n")
}

func logStartupAndShutdown() {
	// Логируем запуск агента
	WriteLog("AgentStart", "localhost", "Agent started")

	// Создаём канал для перехвата сигналов завершения
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signalChan
		WriteLog("AgentStop", "localhost", "Agent stopped")
		log.Println("Agent is shutting down...")
		os.Exit(0)
	}()
}

func createReplicaHandler(w http.ResponseWriter, r *http.Request) {
	WriteLog("CreateReplicaRequest", "localhost", "Received create replica request")

	var payload struct {
		ServiceName string `json:"serviceName"`
		Count       int    `json:"count"`
	}

	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		WriteLog("CreateReplicaError", "localhost", "Invalid request payload")
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Создание реплик и добавление их в карту
	createReplicas := []Replica{}
	for i := 0; i < payload.Count; i++ {
		replica := Replica{
			ID:          replicaIDCounter,
			ServiceName: payload.ServiceName,
			Host:        "localhost",
		}
		replicas[replicaIDCounter] = replica
		createReplicas = append(createReplicas, replica)

		replicaIDCounter++
	}

	WriteLog("CreateReplica", "localhost", "Replicas created: "+strconv.Itoa(payload.Count))
	log.Printf("Creating %d replicas for service %s\n", payload.Count, payload.ServiceName)

	response := map[string]interface{}{
		"serviceName": payload.ServiceName,
		"replicas":    payload.Count,
		"status":      "succes",
		"message":     "Replicas created successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func getReplicaByIDHandler(w http.ResponseWriter, r *http.Request) {
	// Извлекаем ID реплики из URL
	replicaIDStr := r.URL.Path[len("/replica/"):]

	replicaID, err := strconv.Atoi(replicaIDStr)
	if err != nil {
		http.Error(w, "Invalid replica ID", http.StatusBadRequest)
		return
	}

	replica, found := replicas[replicaID]
	if !found {
		http.Error(w, "Replica not fount", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(replica)
}

func deleteReplicaHandler(w http.ResponseWriter, r *http.Request) {
	// replicaIDStr := r.URL.Path[len("/deleteReplica")]

	WriteLog("DeleteReplicaRequest", "localhost", "Received delete replica request")
	log.Println("Deleting replicas...")
	w.WriteHeader(http.StatusOK)
}

func main() {
	http.HandleFunc("/createReplica", createReplicaHandler)
	http.HandleFunc("/deleteReplica", deleteReplicaHandler)
	http.HandleFunc("/replica/", getReplicaByIDHandler)

	log.Println("Agent running on port 8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
