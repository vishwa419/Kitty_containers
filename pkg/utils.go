package kitten

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"syscall"
)

func GenerateID(prefix string) string {
	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)
	randomStr := hex.EncodeToString(randomBytes) //explain gen
	return prefix + "_" + randomStr
}

func shortID(fullID string) string {
	parts := strings.Split(fullID, "_")
	if len(parts) < 2 {
		return fullID
	}
	if len(parts[1]) > 6 {
		return parts[1][:6]
	}
	return parts[1]
}

func EnsureRoot() error {
	if os.Getuid() != 0 {
		return fmt.Errorf("must run as root babe (current UID: %d)", os.Getuid())
	}
	return nil
}

func CheckCapability(cap string) bool {
	return os.Getuid() == 0
} // why is this different from ensureRoot

func ProcessExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	return err == nil
}
