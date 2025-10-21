package main

import (
	"kitten/pkg"
	"log"
	"os"
)

func main() {

	if len(os.Args) >= 2 && os.Args[1] == "__kitten_child__" {
		if len(os.Args) < 3 {
			log.Fatal("Missing container config JSON")
		}

		if err := kitten.RunChild(os.Args[2]); err != nil {
			log.Fatalf("child error: %v", err)
		}
		os.Exit(0)
	}
	if err := kitten.EnsureRoot(); err != nil {
		log.Fatalf("Error: %v", err)
	}

	config := kitten.NewDefaultConfig()
	config.RootFS = "/opt/rootfs/ubuntu-fs/"
	config.Command = []string{"/bin/sh", "-c", "echo 'Hello from container!' && hostname && ps aux"}
	config.Hostname = "example-container"
	config.WorkingDir = "/"

	config.Namespaces.UTS = true
	config.Namespaces.PID = true
	config.Namespaces.Mount = true
	config.Namespaces.IPC = true
	config.Namespaces.Net = false

	config.Env = map[string]string{
		"GREETING": "Hello from Kitten!",
		"EXAMPLE":  "true",
	}

	config.Mounts = kitten.PrepareDefaultMounts()

	log.Println("creating a kitty")

	k, err := kitten.NewKitten(config)
	if err != nil {
		log.Fatalf("Failed to create kitten: %v", err)
	}

	log.Printf("Starting kitty %s...", k.ID)

	err = k.Start()
	if err != nil {
		log.Fatalf("failed to start kitten: %v", err)
	}

	log.Printf("Kitten started successfully with PID %d", k.PID())
	log.Printf("container state: %s", k.State())

	log.Println("Waiting for container to exit....")
	exitCode, err := k.Wait()

	if err != nil {
		log.Printf("Container exited with error: %v", err)
	}

	log.Printf("Container exited with code %d", exitCode)

	info := k.Info()

	log.Printf("Final state: %s", info.State)
	log.Printf("Runtime: %v", info.StopTime.Sub(info.StartTime))

	os.Exit(exitCode)
}
