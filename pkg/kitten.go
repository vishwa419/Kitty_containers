package kitten

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type KittenState int

const (
	StateCreated KittenState = iota
	StateRunning
	StateStopped
	StateError
) //enti idi?

func (s KittenState) String() string {
	switch s {
	case StateCreated:
		return "created"
	case StateRunning:
		return "running"
	case StateStopped:
		return "stopped"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

type Kitten struct {
	ID            string
	config        KittenConfig
	state         KittenState
	pid           int
	cmd           *exec.Cmd
	exitCode      int
	vethHost      string
	vethContainer string
	containerIP   string
	mutex         sync.Mutex
	startTime     time.Time
	stopTime      time.Time
}

type KittenInfo struct {
	ID          string
	State       KittenState
	PID         int
	ExitCode    int
	StartTime   time.Time
	StopTime    time.Time
	ContainerIP string
	Config      KittenConfig
}

func NewKitten(config KittenConfig) (*Kitten, error) {

	err := ValidateConfig(config)
	if err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Generate ID if not provided
	if config.ID == "" {
		config.ID = GenerateID("kitten")
	}

	kitty := &Kitten{
		ID:     config.ID,
		config: config,
		state:  StateCreated,
	}

	return kitty, nil
}

func (k *Kitten) Start() error {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	if k.state != StateCreated {
		return fmt.Errorf("kitten already started (state: %s)", k.state)
	}

	if k.config.Namespaces.Net {
		vethHost, vethContainer, err := createVethPair(k.ID)
		if err != nil {
			return fmt.Errorf("failed to create veth pair: %w", err)
		}

		k.vethHost = vethHost
		k.vethContainer = vethContainer

		k.containerIP = allocateIP(k.config.Network.Subnet)
		k.config.Network.ContainerIP = k.containerIP

		log.Printf("created veth pair")

	}

	configJSON, err := json.Marshal(k.config)
	if err != nil {
		k.cleanup()
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	cmd := exec.Command("/proc/self/exe", "__kitten_child__", string(configJSON))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cloneFlags := buildCloneFlags(k.config.Namespaces)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:   cloneFlags,
		Unshareflags: syscall.CLONE_NEWNS,
	}
	if _, err = os.Stat(k.config.RootFS); os.IsNotExist(err) {
		return fmt.Errorf("rootfs does not exist: %s", k.config.RootFS)
	}

	err = cmd.Start()
	if err != nil {
		k.cleanup()
		return fmt.Errorf("failed to start child process: %w", err)
	}

	k.pid = cmd.Process.Pid
	k.cmd = cmd
	k.state = StateRunning
	k.startTime = time.Now()

	if k.config.Namespaces.Net {
		err = moveVethToNamespace(k.vethContainer, k.pid)
		if err != nil {
			k.Stop()
			return fmt.Errorf("failed to move veth to namespace: %w", err)
		}
		log.Printf("Moved veth to ns")
		err = renameVethInNamespace(k.pid, k.vethContainer, "eth0")
		if err != nil {
			k.Stop()
			return fmt.Errorf("failed to rename veth: %w", err)
		}
		log.Printf("renamed veth")
		cmd := exec.Command("nsenter", "-t", strconv.Itoa(k.pid), "-n", "ip", "link", "set", "eth0", "up")
		output, err := cmd.CombinedOutput()
		if err != nil {
			k.Stop()
			return fmt.Errorf("failed to bring up eth0 in container: %w (output: %s)", err, output)
		}
		log.Printf("Brought up eth0 in container namespace")
		err = configureHostVeth(k.vethHost, k.config.Network)
		if err != nil {
			k.Stop()
			return fmt.Errorf("failed to configure host veth: %w", err)
		}
		log.Printf("configured host veth")
		cmd = exec.Command("nsenter", "-t", strconv.Itoa(k.pid), "-n", "ip", "addr", "add", k.containerIP+"/24", "dev", "eth0")
		if err := cmd.Run(); err != nil {
			k.Stop()
			return fmt.Errorf("failed to add IP to eth0: %w", err)
		}

		// 6. Add default route (last step)
		cmd = exec.Command("nsenter", "-t", strconv.Itoa(k.pid), "-n", "ip", "route", "add", "default", "via", k.config.Network.GatewayIP)
		if err := cmd.Run(); err != nil {
			k.Stop()
			return fmt.Errorf("failed to add default route: %w", err)
		}

		for _, portMapping := range k.config.Network.PortMappings {
			err = addPortForward(portMapping, k.containerIP)
			if err != nil {
				k.Stop()
				return fmt.Errorf("failed to add port forward: %w", err)
			}
		}
	}

	return nil

}

func (k *Kitten) Wait() (int, error) {
	if k.cmd == nil {
		return 0, fmt.Errorf("kitten not started")
	}

	err := k.cmd.Wait()

	k.mutex.Lock()
	k.state = StateStopped
	k.stopTime = time.Now()

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			k.exitCode = exitError.ExitCode()
		} else {
			k.exitCode = 1
		}
	} else {
		k.exitCode = 0
	}
	k.mutex.Unlock()

	k.cleanup()

	return k.exitCode, err
}

func (k *Kitten) Stop() error {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	if k.state != StateRunning {
		return fmt.Errorf("kitten not running (state: %s)", k.state)
	}

	if k.cmd == nil || k.cmd.Process == nil {
		return fmt.Errorf("no process to stop")
	}

	err := k.cmd.Process.Signal(syscall.SIGTERM)
	if err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	done := make(chan bool, 1)
	go func() {
		k.cmd.Wait()
		done <- true
	}()

	select {
	case <-done:
		// process exited gracefully
	case <-time.After(5 * time.Second):
		k.cmd.Process.Kill()
	}

	k.state = StateStopped
	k.stopTime = time.Now()

	k.cleanup()

	return nil
}

func (k *Kitten) cleanup() error {
	if k.config.Namespaces.Net {
		// Remove port forwarding rules
		for _, portMapping := range k.config.Network.PortMappings {
			removePortForward(portMapping, k.containerIP)
		}

		// Delete veth host-side (container side deleted automatically)
		deleteVethInterface(k.vethHost)
	}

	return nil
}

// PID returns the process ID of the kitten
func (k *Kitten) PID() int {
	k.mutex.Lock()
	defer k.mutex.Unlock()
	return k.pid
}

// State returns the current state of the kitten
func (k *Kitten) State() KittenState {
	k.mutex.Lock()
	defer k.mutex.Unlock()
	return k.state
}

// ExitCode returns the exit code of the kitten
func (k *Kitten) ExitCode() int {
	k.mutex.Lock()
	defer k.mutex.Unlock()
	return k.exitCode
}

// Info returns information about the kitten
func (k *Kitten) Info() KittenInfo {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	return KittenInfo{
		ID:          k.ID,
		State:       k.state,
		PID:         k.pid,
		ExitCode:    k.exitCode,
		StartTime:   k.startTime,
		StopTime:    k.stopTime,
		ContainerIP: k.containerIP,
		Config:      k.config,
	}
}
