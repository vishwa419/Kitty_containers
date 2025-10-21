package kitten

import (
	"fmt"
	"os"
	"syscall"
)

type KittenConfig struct {
	ID         string
	RootFS     string
	Command    []string
	Args       []string
	Hostname   string
	Namespaces NamespaceConfig
	Network    *NetworkConfig
	Mounts     []MountConfig
	WorkingDir string
	Env        map[string]string
}

type NamespaceConfig struct {
	UTS   bool
	PID   bool
	Mount bool
	Net   bool
	IPC   bool
	User  bool
}

type NetworkConfig struct {
	Mode         string
	BridgeName   string
	ContainerIP  string
	GatewayIP    string
	Subnet       string
	PortMappings []PortMapping
}

type PortMapping struct {
	HostPort      int
	ContainerPort int
	Protocol      string
}

type MountConfig struct {
	Source string
	Target string
	Type   string
	Flags  uintptr
	Data   string
}

func NewDefaultConfig() KittenConfig {
	return KittenConfig{
		Namespaces: NamespaceConfig{
			UTS:   true,
			PID:   true,
			Mount: true,
			Net:   false,
			IPC:   true,
			User:  false,
		},
		Mounts: []MountConfig{
			{
				Source: "proc",
				Target: "/proc",
				Type:   "proc",
				Flags:  0,
			},
			{
				Source: "sysfs",
				Target: "/sys",
				Type:   "sysfs",
				Flags:  syscall.MS_NOSUID | syscall.MS_NOEXEC | syscall.MS_NODEV | syscall.MS_RDONLY,
			},
		},
		Hostname:   "kitten",
		WorkingDir: "/",
		Env:        make(map[string]string),
	}
}

func ValidateConfig(config KittenConfig) error {
	if config.RootFS == "" {
		return fmt.Errorf("rootfs is required")
	}

	// Check if rootfs exists
	info, err := os.Stat(config.RootFS)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("rootfs path does not exist: %s", config.RootFS)
		}
		return fmt.Errorf("error checking rootfs: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("rootfs must be a directory")
	}

	if len(config.Command) == 0 {
		return fmt.Errorf("command is required")
	}

	if config.Namespaces.Net && config.Network == nil {
		return fmt.Errorf("network config required when net namespace enabled")
	}

	for i, mount := range config.Mounts {
		if mount.Target == "" {
			return fmt.Errorf("mount target required for mount %d", i)
		}
	}

	return nil
}
