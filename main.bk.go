package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"kitten/pkg"
)

func main() {
	// Check if we're being called as child process
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

	// Otherwise, we're being called from CLI
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		runCommand()
	case "version":
		fmt.Println("Kitten Container Runtime v0.1.0")
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Kitten - Lightweight Container Runtime")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  kitten run [options] <rootfs> <command> [args...]")
	fmt.Println("  kitten version")
	fmt.Println("  kitten help")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --hostname <name>     Set container hostname")
	fmt.Println("  --no-uts              Disable UTS namespace")
	fmt.Println("  --no-pid              Disable PID namespace")
	fmt.Println("  --no-mount            Disable mount namespace")
	fmt.Println("  --no-ipc              Disable IPC namespace")
	fmt.Println("  --network <mode>      Network mode: none, host, or bridge")
	fmt.Println("  --ip <ip>             Container IP address")
	fmt.Println("  --gateway <ip>        Gateway IP address")
	fmt.Println("  --subnet <cidr>       Network subnet")
	fmt.Println("  --port <host:container> Port mapping (e.g., 8080:80)")
	fmt.Println("  --env <key=value>     Set environment variable")
	fmt.Println("  --workdir <path>      Set working directory")
}

func runCommand() {
	// Check if running as root
	if err := kitten.EnsureRoot(); err != nil {
		log.Fatalf("Error: %v", err)
	}

	// Parse flags
	runFlags := flag.NewFlagSet("run", flag.ExitOnError)

	hostname := runFlags.String("hostname", "kitten", "Container hostname")
	noUTS := runFlags.Bool("no-uts", false, "Disable UTS namespace")
	noPID := runFlags.Bool("no-pid", false, "Disable PID namespace")
	noMount := runFlags.Bool("no-mount", false, "Disable mount namespace")
	noIPC := runFlags.Bool("no-ipc", false, "Disable IPC namespace")
	networkMode := runFlags.String("network", "none", "Network mode (none, host, bridge)")
	containerIP := runFlags.String("ip", "", "Container IP address")
	gatewayIP := runFlags.String("gateway", "10.0.0.1", "Gateway IP address")
	subnet := runFlags.String("subnet", "10.0.0.0/24", "Network subnet")
	workdir := runFlags.String("workdir", "/", "Working directory")

	var envVars arrayFlags
	var portMappings arrayFlags
	runFlags.Var(&envVars, "env", "Environment variable (can be specified multiple times)")
	runFlags.Var(&portMappings, "port", "Port mapping (can be specified multiple times)")

	// Parse arguments
	runFlags.Parse(os.Args[2:])
	args := runFlags.Args()

	if len(args) < 2 {
		fmt.Println("Error: rootfs and command are required")
		fmt.Println("Usage: kitten run [options] <rootfs> <command> [args...]")
		os.Exit(1)
	}

	rootfs := args[0]
	command := args[1:]

	// Build configuration
	config := kitten.NewDefaultConfig()
	config.RootFS = rootfs
	config.Command = command
	config.Hostname = *hostname
	config.WorkingDir = *workdir

	// Configure namespaces
	config.Namespaces.UTS = !*noUTS
	config.Namespaces.PID = !*noPID
	config.Namespaces.Mount = !*noMount
	config.Namespaces.IPC = !*noIPC

	// Configure network
	if *networkMode != "none" {
		config.Namespaces.Net = true
		config.Network = &kitten.NetworkConfig{
			Mode:      *networkMode,
			GatewayIP: *gatewayIP,
			Subnet:    *subnet,
		}

		if *containerIP != "" {
			config.Network.ContainerIP = *containerIP
		}

		// Parse port mappings
		for _, pm := range portMappings {
			var hostPort, containerPort int
			protocol := "tcp"

			n, err := fmt.Sscanf(pm, "%d:%d", &hostPort, &containerPort)
			if err != nil || n != 2 {
				log.Fatalf("Invalid port mapping format: %s (expected host:container)", pm)
			}

			config.Network.PortMappings = append(config.Network.PortMappings, kitten.PortMapping{
				HostPort:      hostPort,
				ContainerPort: containerPort,
				Protocol:      protocol,
			})
		}

		if *networkMode == "bridge" {
			config.Network.BridgeName = "kitten0"
		}
	}

	// Parse environment variables
	config.Env = make(map[string]string)
	for _, env := range envVars {
		var key, value string
		n, err := fmt.Sscanf(env, "%s=%s", &key, &value)
		if err != nil || n != 2 {
			log.Fatalf("Invalid environment variable format: %s (expected key=value)", env)
		}
		config.Env[key] = value
	}

	// Use default mounts
	config.Mounts = kitten.PrepareDefaultMounts()

	// Create and start kitten
	k, err := kitten.NewKitten(config)
	if err != nil {
		log.Fatalf("Failed to create kitten: %v", err)
	}

	fmt.Printf("Starting kitten %s...\n", k.ID)

	err = k.Start()
	if err != nil {
		log.Fatalf("Failed to start kitten: %v", err)
	}

	fmt.Printf("Kitten started with PID %d\n", k.PID())

	// Wait for kitten to exit
	exitCode, err := k.Wait()
	if err != nil {
		log.Printf("Kitten exited with error: %v", err)
	}

	fmt.Printf("Kitten exited with code %d\n", exitCode)
	os.Exit(exitCode)
}

// arrayFlags implements flag.Value for repeated flags
type arrayFlags []string

func (i *arrayFlags) String() string {
	return fmt.Sprintf("%v", *i)
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}
