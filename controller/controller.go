package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
    "strconv"
    "syscall"
	"time"
    "os/signal"
)

type Service struct {
	Host     string `json:"host"`
	Status   string `json:"status"`
	Replicas int    `json:"replicas"`
}

var services = struct {
	mu       sync.Mutex
	registry map[string]*Service
}{registry: make(map[string]*Service)}


// Logs
type LogEntry struct {
    ID        int    `json:"id"`
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Host      string `json:"host"`
	Details   string `json:"details"`
}

var (
	logFile      = "controller_logs.json"
	logCounter   = 0
	logMu sync.Mutex
)

type ReplicaInfo struct {
    Host       string `json:"host"`
    StartTime  string `json:"start_time"`
    Status     string `json:"status"` // "running", "failed", etc.
}

var replicas = map[string][]ReplicaInfo{}


type AgentStatus struct {
    Host    string `json:"host"`
    Running bool   `json:"running"`
}

// incrementLogCounter safely increments and returns the log counter
func incrementLogCounter() int {
	logMu.Lock()
	defer logMu.Unlock()
	logCounter++

	return logCounter
}


// WriteLog writes an action to the log file
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

	// Записываем JSON с новой строкой
	file.Write(prettyJSON)
	file.WriteString("\n") // Добавляем перенос строки
}


func registerHost(w http.ResponseWriter, r *http.Request) {
	var service Service
	err := json.NewDecoder(r.Body).Decode(&service)
	if err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	services.mu.Lock()
	defer services.mu.Unlock()

	services.registry[service.Host] = &service
	WriteLog("RegisterHost", service.Host, "Host registered")
	log.Printf("Registered host: %s", service.Host)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Host registered"})

	// response := LogEntry{
	// 	Timestamp: time.Now().Format(time.RFC3339),
	// 	Action:    "RegisterService",
	// 	Host:      service.Host,
	// 	Details:   "Service registered",
	// }
}


func logStartupAndShutdown() {
	WriteLog("ControllerStart", "localhost", "Controller started")

	// Создаём канал для перехвата сигналов завершения
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// Блокируем выполнение до получения сигнала
	go func() {
		<-signalChan
		WriteLog("ControllerStop", "localhost", "Controller stopped")
		log.Println("Controller is shutting down...")
		os.Exit(0)
	}()
}

func monitorServices() {
	for {
		services.mu.Lock()
		for host, service := range services.registry {
			resp, err := http.Get("http://" + host + ":8081/status")
			if err != nil || resp.StatusCode != http.StatusOK {
				log.Printf("Host %s is down. Redistributing replicas...", host)
				service.Status = "down"
				redistributeReplicas(host, service.Replicas)
			} else {
				service.Status = "running"
			}
		}
		services.mu.Unlock()
		time.Sleep(30 * time.Second)
	}
}

func redistributeReplicas(failedHost string, replicas int) {
	services.mu.Lock()
	defer services.mu.Unlock()

	activeHosts := []string{}
	for host, serice := range services.registry {
		if host != failedHost && serice.Status == "running" {
			activeHosts = append(activeHosts, host)
		}
	}

	if len(activeHosts) == 0 {
		log.Printf("No active hosts available to redistribute replicas from %s", failedHost)
		return
	}

	replicasPerHost := replicas / len(activeHosts)
	remainingReplicas := replicas % len(activeHosts)

	for _, host := range activeHosts {
		newReplicas := replicasPerHost
		if remainingReplicas > 0 {
			newReplicas++
			remainingReplicas--
		}

		scaleURL := "http://" + host + ":8081/createReplica"
		body, _ := json.Marshal(map[string]interface{}{
			"count": newReplicas,
		})
		
		_, err := http.Post(scaleURL, "application/json", bytes.NewBuffer(body))
		if err != nil {
			log.Printf("Failed to scale replicas on host %s: %v", host, err)
		} else {
			log.Printf("Redistributed %d replicas to host %s", newReplicas, host)
		}
	}

	delete(services.registry, failedHost)
}

// Масштабирование реплик
func scaleService(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Host     string `json:"host"`
		Replicas int    `json:"replicas"`
	}

	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		http.Error(w, "Invalid scale payload", http.StatusBadRequest)
		return
	}

	services.mu.Lock()
	defer services.mu.Unlock()

	service, exists := services.registry[payload.Host]
	if exists {
		http.Error(w, "Host not found", http.StatusNotFound)
		return
	}

	service.Replicas = payload.Replicas
	WriteLog("ScaleService", payload.Host, "Scaled to "+strconv.Itoa(payload.Replicas))
	log.Printf("Scaled service for host %s to %d", payload.Host, payload.Replicas)
	
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Replicas scalled"})
}


func main() {
    logStartupAndShutdown()
	
	go monitorServices()

	http.HandleFunc("/register", registerHost)
	http.HandleFunc("/scale", scaleService)

	log.Println("Controller running on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
