package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "connect":
		// TODO: Implement WebSocket connection to server
		fmt.Println("Connecting to Catnip server...")
	case "status":
		// TODO: Implement server status check
		fmt.Println("Checking server status...")
	case "help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Catnip CLI - Interact with Catnip container server")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  catnip-cli <command> [arguments]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  connect    Connect to a Catnip server PTY session")
	fmt.Println("  status     Check server status")
	fmt.Println("  help       Show this help message")
}