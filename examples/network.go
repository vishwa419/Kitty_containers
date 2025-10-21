package main

import (
	"fmt"
	"kitten/pkg"
	"log"
	"os"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "__kitten_child__" {
		// We're inside the container
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

	if err := kitten.EnsureRoot(); err != nil {
		log.Fatalf("Error: %v", err)
	}

	config := kitten.NewDefaultConfig()

	config.RootFS = "/opt/rootfs/ubuntu-fs/"
	config.Command = []string{"/bin/sh", "-c", `
		echo "Network Configuration:"
		ip addr show
		echo ""
		echo "Routes:"
		ip route show
		echo ""
		echo "Testing connectivity:"
		ping -c 3 8.8.8.8 || echo "No external conncection"
		`}
	config.Hostname = "network-test"
	config.WorkingDir = "/"

	config.Namespaces.UTS = true
	config.Namespaces.PID = true
	config.Namespaces.Mount = true
	config.Namespaces.IPC = true
	config.Namespaces.Net = true

	config.Network = &kitten.NetworkConfig{
		Mode:       "bridge",
		Subnet:     "10.0.0.0/24",
		GatewayIP:  "10.0.0.1",
		BridgeName: "kitten0",
		PortMappings: []kitten.PortMapping{
			{
				HostPort:      8080,
				ContainerPort: 80,
				Protocol:      "tcp",
			},
		},
	}

	config.Env = map[string]string{
		"NETWORK_TEST": "true",
	}

	config.Mounts = kitten.PrepareDefaultMounts()

	log.Println("Creating network-enabled kitty....")

	k, err := kitten.NewKitten(config)
	if err != nil {
		log.Fatalf("failed to start kitten config error: %v", err)
	}

	err = k.Start()
	if err != nil {
		log.Fatalf("Failed to start kitten: %v", err)
	}

	log.Printf("Kitten started successfully with PID %d", k.PID())
	log.Printf("Container state: %s", k.State())

	info := k.Info()

	log.Printf("kitty started successfully")
	log.Printf(" PID: %d", k.PID())
	log.Printf(" container IP: %s", info.ContainerIP)
	log.Printf(" state: %s", k.State())

	log.Printf("waiting for kitty to sleep...")
	exiCode, err := k.Wait()

	if err != nil {
		log.Printf("container exited with error: %v", err)
	}

	log.Printf("Container exited with code %d", exiCode)

	finalInfo := k.Info()

	log.Printf("Runtime: %v", finalInfo.StopTime.Sub(finalInfo.StartTime))

	os.Exit(exiCode)
}
