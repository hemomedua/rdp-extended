package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	// Initialize database
	dbPath := getDBPath()
	database, err := NewDatabase(dbPath)
	if err != nil {
		fmt.Printf("Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// Create RDP executor
	executor := NewRDPExecutor(database)

	// Check if CLI mode or GUI mode
	if len(os.Args) > 1 {
		// CLI mode
		HandleCLI(os.Args, database, executor)
	} else {
		// GUI mode
		if err := CreateMainWindow(database, executor); err != nil {
			fmt.Printf("Failed to create GUI: %v\n", err)
			os.Exit(1)
		}
	}
}

func getDBPath() string {
	// Use AppData\Local\RDPExtended\rdp-manager.db on Windows
	appDataDir := os.Getenv("LOCALAPPDATA")
	if appDataDir == "" {
		// Fallback to home directory
		homeDir, _ := os.UserHomeDir()
		appDataDir = filepath.Join(homeDir, "AppData", "Local")
	}

	dbDir := filepath.Join(appDataDir, "RDPExtended")
	return filepath.Join(dbDir, "rdp-manager.db")
}
