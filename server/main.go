package main

import (
	"encoding/json"
	"fmt"
	"io"
	"kitten/pkg"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// API server that spawns containers without blocking
type APIServer struct {
	managers map[string]*kitten.Manager
	mu       sync.RWMutex
	port     string
}

// Request/Response types
type SpawnRequest struct {
	Config kitten.ManagerConfig `json:"config"`
}

type SpawnResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

type StatusResponse struct {
	ID         string            `json:"id"`
	Containers map[string]string `json:"containers"`
	Status     string            `json:"status"`
}

type ListResponse struct {
	Deployments []DeploymentInfo `json:"deployments"`
}

type DeploymentInfo struct {
	ID         string            `json:"id"`
	Containers map[string]string `json:"containers"`
	StartTime  string            `json:"start_time"`
}

func NewAPIServer(port string) *APIServer {
	return &APIServer{
		managers: make(map[string]*kitten.Manager),
		port:     port,
	}
}

func (s *APIServer) Start() error {
	http.HandleFunc("/spawn", s.handleSpawn)
	http.HandleFunc("/status/", s.handleStatus)
	http.HandleFunc("/stop/", s.handleStop)
	http.HandleFunc("/list", s.handleList)
	http.HandleFunc("/health", s.handleHealth)

	log.Printf("Starting Kitten API server on port %s", s.port)
	return http.ListenAndServe(":"+s.port, nil)
}

// POST /spawn - Spawn containers from JSON config
func (s *APIServer) handleSpawn(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received spawn request from %s", r.RemoteAddr)

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	log.Printf("Received config: %s", string(body))

	// Create manager from JSON
	manager, err := kitten.NewManager(string(body))
	if err != nil {
		log.Printf("Invalid config: %v", err)
		http.Error(w, fmt.Sprintf("Invalid config: %v", err), http.StatusBadRequest)
		return
	}

	// Generate deployment ID
	deploymentID := kitten.GenerateID("deploy")
	log.Printf("Generated deployment ID: %s", deploymentID)

	// Store manager
	s.mu.Lock()
	s.managers[deploymentID] = manager
	s.mu.Unlock()

	// Respond immediately before starting containers
	response := SpawnResponse{
		ID:      deploymentID,
		Message: "Containers are being spawned",
		Status:  "starting",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)

	log.Printf("[%s] Response sent to client, now starting containers in background", deploymentID)

	// Start containers asynchronously (non-blocking!)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[%s] PANIC in goroutine: %v", deploymentID, r)
			}
		}()

		log.Printf("[%s] Starting containers...", deploymentID)
		if err := manager.Start(); err != nil {
			log.Printf("[%s] Failed to start containers: %v", deploymentID, err)
			return
		}
		log.Printf("[%s] All containers started successfully", deploymentID)

		// Wait for containers in background
		if err := manager.Wait(); err != nil {
			log.Printf("[%s] Container error: %v", deploymentID, err)
		}
		log.Printf("[%s] All containers exited", deploymentID)

		// Cleanup
		s.mu.Lock()
		delete(s.managers, deploymentID)
		s.mu.Unlock()
	}()

	// Return immediately
	response = SpawnResponse{
		ID:      deploymentID,
		Message: "Containers are being spawned",
		Status:  "starting",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GET /status/{id} - Get status of a deployment
func (s *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deploymentID := r.URL.Path[len("/status/"):]
	if deploymentID == "" {
		http.Error(w, "Deployment ID required", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	manager, exists := s.managers[deploymentID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	status := manager.Status()
	response := StatusResponse{
		ID:         deploymentID,
		Containers: status,
		Status:     "running",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// POST /stop/{id} - Stop a deployment
func (s *APIServer) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deploymentID := r.URL.Path[len("/stop/"):]
	if deploymentID == "" {
		http.Error(w, "Deployment ID required", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	manager, exists := s.managers[deploymentID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	// Stop containers asynchronously
	go func() {
		log.Printf("[%s] Stopping containers...", deploymentID)
		if err := manager.Stop(); err != nil {
			log.Printf("[%s] Error stopping: %v", deploymentID, err)
		}

		s.mu.Lock()
		delete(s.managers, deploymentID)
		s.mu.Unlock()

		log.Printf("[%s] Stopped and cleaned up", deploymentID)
	}()

	response := map[string]string{
		"id":      deploymentID,
		"message": "Stopping containers",
		"status":  "stopping",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GET /list - List all deployments
func (s *APIServer) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	deployments := make([]DeploymentInfo, 0, len(s.managers))
	for id, manager := range s.managers {
		deployments = append(deployments, DeploymentInfo{
			ID:         id,
			Containers: manager.Status(),
			StartTime:  time.Now().Format(time.RFC3339),
		})
	}

	response := ListResponse{
		Deployments: deployments,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GET /health - Health check
func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func main() {
	// CRITICAL: Handle child process re-execution
	// When kitten spawns a container, it re-executes itself with __kitten_child__
	if len(os.Args) >= 2 && os.Args[1] == "__kitten_child__" {
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "missing config JSON\n")
			os.Exit(1)
		}

		configJSON := os.Args[2]
		err := kitten.RunChild(configJSON)
		if err != nil {
			fmt.Fprintf(os.Stderr, "child error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Check root permissions
	if err := kitten.EnsureRoot(); err != nil {
		log.Fatalf("Error: %v", err)
	}

	port := "8080"
	server := NewAPIServer(port)

	log.Printf("Kitten API Server starting on http://localhost:%s", port)
	log.Printf("Endpoints:")
	log.Printf("  POST   /spawn       - Spawn containers from JSON")
	log.Printf("  GET    /status/{id} - Get deployment status")
	log.Printf("  POST   /stop/{id}   - Stop deployment")
	log.Printf("  GET    /list        - List all deployments")
	log.Printf("  GET    /health      - Health check")

	if err := server.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
