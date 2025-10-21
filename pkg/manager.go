package kitten

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// ManagerConfig defines the configuration for multiple containers
type ManagerConfig struct {
	Version    string                   `json:"version"`
	Containers map[string]ContainerSpec `json:"containers"`
	Networks   map[string]NetworkSpec   `json:"networks,omitempty"`
}

// ContainerSpec defines a single container configuration
type ContainerSpec struct {
	Image       string            `json:"image"`
	Command     []string          `json:"command,omitempty"`
	Hostname    string            `json:"hostname,omitempty"`
	WorkingDir  string            `json:"workdir,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	Ports       []string          `json:"ports,omitempty"`
	Network     string            `json:"network,omitempty"`
	IP          string            `json:"ip,omitempty"`
	DependsOn   []string          `json:"depends_on,omitempty"`
	Restart     string            `json:"restart,omitempty"` // no, always, on-failure
	Namespaces  *NamespaceConfig  `json:"namespaces,omitempty"`
}

// NetworkSpec defines network configuration
type NetworkSpec struct {
	Driver  string `json:"driver"` // bridge, host, none
	Subnet  string `json:"subnet,omitempty"`
	Gateway string `json:"gateway,omitempty"`
}

// Manager orchestrates multiple containers
type Manager struct {
	config     *ManagerConfig
	containers map[string]*Kitten
	networks   map[string]*NetworkConfig
	mu         sync.RWMutex
}

// NewManager creates a new container manager from JSON
func NewManager(configJSON string) (*Manager, error) {
	var config ManagerConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &Manager{
		config:     &config,
		containers: make(map[string]*Kitten),
		networks:   make(map[string]*NetworkConfig),
	}, nil
}

// NewManagerFromFile creates a new container manager from a JSON file
func NewManagerFromFile(configPath string) (*Manager, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	return NewManager(string(data))
}

// Start starts all containers according to their dependencies
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// First, create networks
	if err := m.createNetworks(); err != nil {
		return fmt.Errorf("failed to create networks: %w", err)
	}

	// Start containers in dependency order
	started := make(map[string]bool)

	for len(started) < len(m.config.Containers) {
		progress := false

		for name, spec := range m.config.Containers {
			if started[name] {
				continue
			}

			// Check if all dependencies are started
			canStart := true
			for _, dep := range spec.DependsOn {
				if !started[dep] {
					canStart = false
					break
				}
			}

			if canStart {
				fmt.Printf("Starting container: %s\n", name)
				if err := m.startContainer(name, spec); err != nil {
					return fmt.Errorf("failed to start container %s: %w", name, err)
				}
				started[name] = true
				progress = true

				// Brief pause to avoid overwhelming the system
				time.Sleep(100 * time.Millisecond)
			}
		}

		if !progress {
			return fmt.Errorf("circular dependency detected or missing dependency")
		}
	}

	fmt.Printf("All %d containers started successfully\n", len(started))
	return nil
}

// Stop stops all containers
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fmt.Println("Stopping all containers...")

	// Stop in reverse dependency order
	deps := m.buildDependencyGraph()
	order := m.getStopOrder(deps)

	for _, name := range order {
		if k, exists := m.containers[name]; exists {
			fmt.Printf("Stopping container: %s\n", name)
			if err := k.Stop(); err != nil {
				fmt.Printf("Warning: failed to stop %s: %v\n", name, err)
			}
		}
	}

	// Cleanup networks
	m.cleanupNetworks()

	fmt.Println("All containers stopped")
	return nil
}

// Wait waits for all containers to exit
func (m *Manager) Wait() error {
	m.mu.RLock()
	containers := make([]*Kitten, 0, len(m.containers))
	for _, k := range m.containers {
		containers = append(containers, k)
	}
	m.mu.RUnlock()

	// Wait for all containers
	results := make(chan error, len(containers))

	for _, k := range containers {
		go func(kitten *Kitten) {
			_, err := kitten.Wait()
			results <- err
		}(k)
	}

	// Collect results
	var firstErr error
	for i := 0; i < len(containers); i++ {
		if err := <-results; err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// Restart restarts a specific container
func (m *Manager) Restart(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	k, exists := m.containers[name]
	if !exists {
		return fmt.Errorf("container %s not found", name)
	}

	fmt.Printf("Restarting container: %s\n", name)

	if err := k.Stop(); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	spec := m.config.Containers[name]
	return m.startContainer(name, spec)
}

// Status returns the status of all containers
func (m *Manager) Status() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]string)
	for name, k := range m.containers {
		// Check if container process is still running
		if k.PID() > 0 {
			status[name] = "running"
		} else {
			status[name] = "stopped"
		}
	}

	return status
}

// GetContainer returns a specific container by name
func (m *Manager) GetContainer(name string) (*Kitten, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	k, exists := m.containers[name]
	return k, exists
}

// ListContainers returns all container names
func (m *Manager) ListContainers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.containers))
	for name := range m.containers {
		names = append(names, name)
	}
	return names
}

// startContainer starts a single container
func (m *Manager) startContainer(name string, spec ContainerSpec) error {
	config := NewDefaultConfig()
	config.RootFS = spec.Image
	config.Command = spec.Command
	config.Hostname = spec.Hostname
	config.WorkingDir = spec.WorkingDir
	config.Env = spec.Environment

	if config.Hostname == "" {
		config.Hostname = name
	}

	if config.WorkingDir == "" {
		config.WorkingDir = "/"
	}

	// Configure namespaces
	if spec.Namespaces != nil {
		config.Namespaces = *spec.Namespaces
	}

	// Configure networking
	if spec.Network != "" {
		if netConfig, exists := m.networks[spec.Network]; exists {
			config.Namespaces.Net = true
			config.Network = &NetworkConfig{
				Mode:      netConfig.Mode,
				Subnet:    netConfig.Subnet,
				GatewayIP: netConfig.GatewayIP,
			}

			if spec.IP != "" {
				config.Network.ContainerIP = spec.IP
			}

			// Parse port mappings
			for _, pm := range spec.Ports {
				var hostPort, containerPort int
				n, err := fmt.Sscanf(pm, "%d:%d", &hostPort, &containerPort)
				if err != nil || n != 2 {
					return fmt.Errorf("invalid port mapping: %s", pm)
				}

				config.Network.PortMappings = append(config.Network.PortMappings, PortMapping{
					HostPort:      hostPort,
					ContainerPort: containerPort,
					Protocol:      "tcp",
				})
			}

			if netConfig.Mode == "bridge" {
				config.Network.BridgeName = "kitten0"
			}
		}
	}

	// Default mounts
	config.Mounts = PrepareDefaultMounts()

	// Create and start container
	k, err := NewKitten(config)
	if err != nil {
		return err
	}

	if err := k.Start(); err != nil {
		return err
	}

	m.containers[name] = k

	// Handle restart policy
	if spec.Restart == "always" || spec.Restart == "on-failure" {
		go m.handleRestart(name, spec)
	}

	return nil
}

// handleRestart monitors and restarts a container based on restart policy
func (m *Manager) handleRestart(name string, spec ContainerSpec) {
	for {
		m.mu.RLock()
		k, exists := m.containers[name]
		m.mu.RUnlock()

		if !exists {
			return
		}

		exitCode, err := k.Wait()

		shouldRestart := false
		if spec.Restart == "always" {
			shouldRestart = true
		} else if spec.Restart == "on-failure" && (exitCode != 0 || err != nil) {
			shouldRestart = true
		}

		if shouldRestart {
			fmt.Printf("Container %s exited (code=%d), restarting...\n", name, exitCode)
			time.Sleep(1 * time.Second)

			m.mu.Lock()
			if err := m.startContainer(name, spec); err != nil {
				fmt.Printf("Failed to restart %s: %v\n", name, err)
				m.mu.Unlock()
				return
			}
			m.mu.Unlock()
		} else {
			return
		}
	}
}

// createNetworks creates all defined networks
func (m *Manager) createNetworks() error {
	for name, spec := range m.config.Networks {
		netConfig := &NetworkConfig{
			Mode:   spec.Driver,
			Subnet: spec.Subnet,
		}

		if spec.Gateway != "" {
			netConfig.GatewayIP = spec.Gateway
		} else if spec.Subnet != "" {
			netConfig.GatewayIP = "10.0.0.1" // default
		}

		// Create bridge if driver is bridge
		if spec.Driver == "bridge" {
			bridgeName := "kitten0"
			netConfig.BridgeName = bridgeName

			// Create the bridge interface
			if err := CreateBridge(bridgeName, spec.Subnet, spec.Gateway); err != nil {
				return fmt.Errorf("failed to create bridge %s: %w", bridgeName, err)
			}
			fmt.Printf("Created bridge: %s\n", bridgeName)
		}

		m.networks[name] = netConfig
		fmt.Printf("Created network: %s (%s)\n", name, spec.Driver)
	}

	return nil
}

// cleanupNetworks cleans up all networks
func (m *Manager) cleanupNetworks() {
	for name, netConfig := range m.networks {
		if netConfig.Mode == "bridge" && netConfig.BridgeName != "" {
			// Delete the bridge
			if err := DeleteBridge(netConfig.BridgeName); err != nil {
				fmt.Printf("Warning: failed to delete bridge %s: %v\n", netConfig.BridgeName, err)
			} else {
				fmt.Printf("Deleted bridge: %s\n", netConfig.BridgeName)
			}
		}
		fmt.Printf("Cleaned up network: %s\n", name)
	}
	m.networks = make(map[string]*NetworkConfig)
}

// buildDependencyGraph builds the dependency graph
func (m *Manager) buildDependencyGraph() map[string][]string {
	deps := make(map[string][]string)

	for name, spec := range m.config.Containers {
		deps[name] = spec.DependsOn
	}

	return deps
}

// getStopOrder returns the order to stop containers (reverse of start order)
func (m *Manager) getStopOrder(deps map[string][]string) []string {
	order := make([]string, 0, len(deps))
	visited := make(map[string]bool)

	var visit func(string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true

		// Visit dependencies first
		for _, dep := range deps[name] {
			visit(dep)
		}

		order = append(order, name)
	}

	for name := range deps {
		visit(name)
	}

	// Reverse the order
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	return order
}
