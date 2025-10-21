package kitten

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

func WaitForProcess(pid int, timeout time.Duration) error {
	done := make(chan error, 1)

	go func() {
		process, err := os.FindProcess(pid)
		if err != nil {
			done <- err
			return
		}
		_, err = process.Wait()
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for process %d", pid)
	}
}

func KillProcess(pid int, graceful bool) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	if graceful {
		err = process.Signal(syscall.SIGTERM)
		if err != nil {
			return fmt.Errorf("failed to send SIGTERM: %w", err)
		}

		err = WaitForProcess(pid, 5*time.Second)
		if err == nil {
			return nil
		}
	}

	err = process.Kill()
	if err != nil {
		return fmt.Errorf("failed to kill process: %w", err)
	}
	return nil
}
