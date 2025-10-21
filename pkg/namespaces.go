package kitten

import (
	"fmt"
	"os"
	"syscall"
)

func buildCloneFlags(ns NamespaceConfig) uintptr {
	var flags uintptr = 0

	if ns.UTS {
		flags |= syscall.CLONE_NEWUTS
	}

	if ns.PID {
		flags |= syscall.CLONE_NEWPID
	}

	if ns.Mount {
		flags |= syscall.CLONE_NEWNS
	}

	if ns.Net {
		flags |= syscall.CLONE_NEWNET
	}

	if ns.IPC {
		flags |= syscall.CLONE_NEWIPC
	}

	if ns.User {
		flags |= syscall.CLONE_NEWUSER
	}

	return flags
}

func getNamespacePath(pid int, nsType string) string {
	return fmt.Sprintf("/proc/%d/ns/%s", pid, nsType)
}

func namespaceExists(pid int, nsType string) bool {
	path := getNamespacePath(pid, nsType)
	_, err := os.Stat(path) //whats stat??
	return err == nil
}
