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

// const logFile = "controller_logs.json"

// LogEntry represents a log structure
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
	logCounterMu sync.Mutex
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
	logCounterMu.Lock()
	defer logCounterMu.Unlock()
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

	// Открываем файл для записи
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Error opening log file: %v", err)
		return
	}
	defer file.Close()

	// Форматируем JSON-лог с отступами
	prettyJSON, err := json.MarshalIndent(logEntry, "", "    ")
	if err != nil {
		log.Printf("Error formatting log entry: %v", err)
		return
	}

	// Записываем JSON с новой строкой
	file.Write(prettyJSON)
	file.WriteString("\n") // Добавляем перенос строки
}


func logStartupAndShutdown() {
	// Логируем запуск контроллера
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

func monitorAgents() {
    for {
        for host, service := range services.registry {
            resp, err := http.Get("http://" + host + ":8081/status")
            if err != nil || resp.StatusCode != http.StatusOK {
                log.Printf("Agent %s is down, redistributing replicas...", host)
                redistributeReplicas(host, service.Replicas)
				service.Status = "down"
            } else {
				service.Status = "running"
			}
        }
		services.mu.Lock()
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
		newReplicaCount := replicasPerHost
		if remainingReplicas > 0 {
			newReplicaCount++
			remainingReplicas--
		}

		scaleURL := "http://" + host + ":8081/createReplica"
		body, _ := json.Marshal(map[string]interface{}{
			"serviceName": "loader",
			"count":       newReplicaCount,
		})
		_, err := http.Post(scaleURL, "application/json", bytes.NewBuffer(body))
		if err != nil {
			log.Printf("Failed to scale replicas on host %s: %v", host, err)
		} else {
			log.Printf("Redistributed %d replicas to host %s", newReplicaCount, host)
		}
	}

	delete(services.registry, failedHost)
}

func postToGetHandler(w http.ResponseWriter, r *http.Request) {
	// Структура для обработки данных из POST-запроса
	var payload struct {
		Message string `json:"message"`
		Author  string `json:"author"`
	}

	// Декодируем тело POST-запроса
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Логируем и готовим данные для ответа
    WriteLog("PostToGet", "localhost", "Received message: "+payload.Message+" by "+payload.Author)
	response := map[string]string{
		"message": payload.Message,
		"author":  payload.Author,
		"status":  "Received and processed",
	}

	// Отправляем ответ в формате JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}


// Register a new service
func registerService(w http.ResponseWriter, r *http.Request) {
	var service Service

	err := json.NewDecoder(r.Body).Decode(&service)
	if err != nil {
		http.Error(w, "Invalid service payload", http.StatusBadRequest)
		return
	}

	services.mu.Lock()
	services.registry[service.Host] = &service
	services.mu.Unlock()

	// Логируем событие
	WriteLog("RegisterService", service.Host, "Service registered")
	// log.Printf("Registered service: %+v\n", service)

	// response := LogEntry{
	// 	Timestamp: time.Now().Format(time.RFC3339),
	// 	Action:    "RegisterService",
	// 	Host:      service.Host,
	// 	Details:   "Service registered",
	// }

	response := map[string]string{
		"status": "success",
		"message": "Service registered successfully",
	}

	// Отправляем JSON в ответ
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}


// Scale service replicas
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
	service, exists := services.registry[payload.Host]
	if exists {
		service.Replicas = payload.Replicas

		// Call agent API to create/delete replicas
		scaleURL := "http://" + payload.Host + ":8081/createReplica"
		body, _ := json.Marshal(map[string]interface{}{
			"serviceName": "loader",
			"count":       payload.Replicas,
		})
		resp, err := http.Post(scaleURL, "application/json", bytes.NewBuffer(body))
		if err != nil || resp.StatusCode != http.StatusCreated {
			log.Printf("Failed to scale service: %v", err)
			http.Error(w, "Failed to scale service", http.StatusInternalServerError)
		} else {
			WriteLog("ScaleService", payload.Host, "Scaled to "+strconv.Itoa(payload.Replicas))
			log.Printf("Scaled service on %s to %d replicas\n", payload.Host, payload.Replicas)
			w.WriteHeader(http.StatusOK)
		}
	}
	services.mu.Unlock()

	if !exists {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}
}


func main() {
    logStartupAndShutdown()
	
	go monitorAgents()

	http.HandleFunc("/register", registerService)
	http.HandleFunc("/scale", scaleService)
	http.HandleFunc("/post-to-get", postToGetHandler) // Новый маршрут

	log.Println("Controller running on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
