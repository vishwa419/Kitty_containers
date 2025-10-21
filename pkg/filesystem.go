package kitten

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"
)

func ValidateRootFs(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("rootfs does not exist babe: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("rootfs must be a directory darling")
	}

	essentialDirs := []string{"/bin", "/etc", "/lib", "/usr"}
	for _, dir := range essentialDirs {
		fullPath := filepath.Join(path, dir)
		if !directoryExists(fullPath) {
			log.Printf("Warning: rootfs missing %s (might not be a valid root filesystem)", dir)
		}
	}
	return nil
}

func PrepareDefaultMounts() []MountConfig {
	return []MountConfig{
		{
			Source: "proc",
			Target: "/proc",
			Type:   "proc",
			Flags:  0,
		},
		{
			Source: "tmpfs",
			Target: "/dev",
			Type:   "tmpfs",
			Flags:  syscall.MS_NOSUID | syscall.MS_STRICTATIME,
			Data:   "mode=755",
		},
		{
			Source: "devpts",
			Target: "/dev/pts",
			Type:   "devpts",
			Flags:  syscall.MS_NOSUID | syscall.MS_NOEXEC,
			Data:   "newinstance,ptmxmode=0666,mode=0620",
		},
		{
			Source: "sysfs",
			Target: "/sys",
			Type:   "sysfs",
			Flags:  syscall.MS_NOSUID | syscall.MS_NOEXEC | syscall.MS_NODEV | syscall.MS_RDONLY,
		},
		{
			Source: "tmpfs",
			Target: "/run",
			Type:   "tmpfs",
			Flags:  syscall.MS_NOSUID | syscall.MS_NODEV,
			Data:   "mode=755",
		},
	}
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
