package kitten

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func RunChild(configJSON string) error {

	var config KittenConfig
	err := json.Unmarshal([]byte(configJSON), &config)

	if err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if config.Namespaces.UTS {
		err = syscall.Sethostname([]byte(config.Hostname))
		if err != nil {
			return fmt.Errorf("failed to set hostname: %w", err)
		}
	}

	if config.Namespaces.Mount {

		err = syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, "")
		if err != nil {
			return fmt.Errorf("failed to make / private: %w", err)
		}

		err = pivotRoot(config.RootFS)
		if err != nil {
			log.Printf("pivot_root failed, falling back to chroot: %v", err)
			err = syscall.Chroot(config.RootFS)
			if err != nil {
				return fmt.Errorf("failed to chroot: %w, %s", err, config.RootFS)
			}
		}

		err = syscall.Chdir("/")
		if err != nil {
			return fmt.Errorf("failed to chdir to /: %w", err)
		}

		for _, mount := range config.Mounts {
			err = mountFilesystem(mount)
			if err != nil {
				log.Printf("Warning: failed to mount %s: %v", mount.Target, err)
			}
		}
		if err := ensureMinDev("/dev"); err != nil {
			return fmt.Errorf("failed to setup /dev: %w", err)
		}

	}

	if config.Namespaces.Net {
		err = execCommand("ip", "link", "set", "lo", "up")
		if err != nil {
			return fmt.Errorf("failed to bring up loopback: %w", err)
		}
		//*
		err = execCommand("ip", "addr", "add", config.Network.ContainerIP+"/24", "dev", "eth0")
		if err != nil {
			return fmt.Errorf("failed to add IP to eth0: %w", err)
		}
		log.Printf("added IP to eth0: %s, adding gateway: %s", config.Network.ContainerIP, config.Network.GatewayIP)
		//*/
		for i := 0; i < 5; i++ {
			err := execCommand("ip", "route", "add", "default", "via", config.Network.GatewayIP)
			if err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
			log.Printf("Ran one loop: %w", err)
		}

		//err = execCommand("ip", "route", "add", "default", "via", config.Network.GatewayIP)
		if err != nil {
			return fmt.Errorf("failed to add default route: %w", err)
		}

	}

	if config.Env != nil {
		for key, value := range config.Env {
			os.Setenv(key, value)
		}
	}

	if os.Getenv("PATH") == "" {
		os.Setenv("PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	}

	if os.Getenv("HOME") == "" {
		os.Setenv("HOME", "/root")
	}

	if config.WorkingDir != "" {
		err = syscall.Chdir(config.WorkingDir)
		if err != nil {
			return fmt.Errorf("failed to chdir to %s: %w", config.WorkingDir, err)
		}
	}

	args := config.Command
	if len(config.Args) > 0 {
		args = append(args, config.Args...)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	err = cmd.Run()
	log.Printf("Command finished with err: %w", err)
	for i := len(config.Mounts) - 1; i >= 0; i-- {
		syscall.Unmount(config.Mounts[i].Target, 0)
		// Ignore errors during cleanup
	}
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			os.Exit(exitError.ExitCode())
		} else {
			fmt.Printf("command execution failed: %w", err)
			os.Exit(1)
		}
	}

	log.Printf("About to exit")
	os.Exit(0)
	return nil

}

func pivotRoot(newRoot string) error {
	putOld := filepath.Join(newRoot, ".pivot_root")

	err := os.MkdirAll(putOld, 0700)
	if err != nil {
		return fmt.Errorf("failed to create pivot_root dir: %w", err)
	}

	err = syscall.Mount(newRoot, newRoot, "", syscall.MS_BIND|syscall.MS_REC, "")
	if err != nil {
		return fmt.Errorf("failed to bind mount new root: %w", err)
	}

	err = syscall.PivotRoot(newRoot, putOld)
	if err != nil {
		return fmt.Errorf("pivot_root failed in : %w", err)
	}

	err = syscall.Chdir("/")
	if err != nil {
		return fmt.Errorf("failed to chdir after pivot: %w", err)
	}

	putOld = "/.pivot_root"
	err = syscall.Unmount(putOld, syscall.MNT_DETACH)
	if err != nil {
		return fmt.Errorf("failed to unmount old root: %w", err)
	}

	// Remove old root directory
	err = os.RemoveAll(putOld)
	if err != nil {
		return fmt.Errorf("failed to remove old root dir: %w", err)
	}

	return nil
}

func mountFilesystem(mount MountConfig) error {
	// Ensure target directory exists
	if mount.Type != "proc" && mount.Type != "sysfs" {
		err := os.MkdirAll(mount.Target, 0755)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("failed to create mount target: %w", err)
		}
	}

	// Perform mount
	err := syscall.Mount(mount.Source, mount.Target, mount.Type, mount.Flags, mount.Data)
	if err != nil {
		return fmt.Errorf("mount failed: %w", err)
	}

	return nil
}

// execCommand executes a command and returns an error if it fails
func execCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w (output: %s)", err, string(output))
	}
	return nil
}

func ensureMinDev(devDir string) error {
	// Ensure /dev directory exists
	if err := os.MkdirAll(devDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", devDir, err)
	}

	devs := []struct {
		name string
		mode uint32
		dev  int
	}{
		{"null", syscall.S_IFCHR | 0666, makedev(1, 3)},
		{"zero", syscall.S_IFCHR | 0666, makedev(1, 5)},
		{"full", syscall.S_IFCHR | 0666, makedev(1, 7)},
		{"random", syscall.S_IFCHR | 0666, makedev(1, 8)},
		{"urandom", syscall.S_IFCHR | 0666, makedev(1, 9)},
	}

	for _, d := range devs {
		path := devDir + "/" + d.name
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := syscall.Mknod(path, d.mode, d.dev); err != nil {
				return fmt.Errorf("failed to create %s: %w", path, err)
			}
		}
	}

	// Ensure /dev/pts exists
	if err := os.MkdirAll(devDir+"/pts", 0755); err != nil {
		return fmt.Errorf("failed to create pts: %w", err)
	}

	// Ensure /dev/ptmx points to pts/ptmx
	ptmxPath := devDir + "/ptmx"
	if _, err := os.Lstat(ptmxPath); os.IsNotExist(err) {
		if err := os.Symlink("pts/ptmx", ptmxPath); err != nil {
			return fmt.Errorf("failed to symlink ptmx: %w", err)
		}
	}

	return nil
}

// makedev encodes major/minor into a single int (same as Linux makedev macro)
func makedev(major int, minor int) int {
	return int((major << 8) | (minor & 0xff) | ((minor & 0xfff00) << 12))
}
