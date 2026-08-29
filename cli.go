package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

func HandleCLI(args []string, db *Database, executor *RDPExecutor) {
	if len(args) < 2 {
		printHelp()
		return
	}

	command := args[1]

	switch command {
	case "list":
		cmdList(db)
	case "connect":
		cmdConnect(args[2:], db, executor)
	case "add":
		cmdAdd(args[2:], db)
	case "delete":
		cmdDelete(args[2:], db)
	case "proxy":
		cmdProxy(args[2:], db)
	case "help":
		printHelp()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printHelp()
	}
}

func cmdList(db *Database) {
	hosts, err := db.GetAllHosts()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if len(hosts) == 0 {
		fmt.Println("No saved profiles found")
		return
	}

	sort.Strings(hosts)
	fmt.Println("=== Saved Profiles ===")
	for _, host := range hosts {
		profiles, err := db.GetProfilesByHost(host)
		if err != nil {
			continue
		}

		fmt.Printf("\nHost: %s\n", host)
		for _, p := range profiles {
			fmt.Printf("  - %s (port %d)\n", p.Username, p.Port)
		}
	}
}

func cmdConnect(args []string, db *Database, executor *RDPExecutor) {
	if len(args) == 0 {
		fmt.Println("Usage: rdp-manager connect <host> [<username>]")
		return
	}

	host := args[0]
	profiles, err := db.GetProfilesByHost(host)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if len(profiles) == 0 {
		fmt.Printf("No profiles found for host: %s\n", host)
		return
	}

	var profile *RDPProfile

	if len(args) > 1 {
		// Specific username provided
		username := args[1]
		for i, p := range profiles {
			if p.Username == username {
				profile = &profiles[i]
				break
			}
		}
		if profile == nil {
			fmt.Printf("Profile not found: %s@%s\n", username, host)
			return
		}
	} else if len(profiles) == 1 {
		// Only one profile
		profile = &profiles[0]
	} else {
		// Multiple profiles - let user choose
		fmt.Printf("Multiple profiles for %s:\n", host)
		for i, p := range profiles {
			fmt.Printf("%d. %s\n", i+1, p.Username)
		}

		fmt.Print("Select profile (1-" + fmt.Sprintf("%d", len(profiles)) + "): ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		var choice int
		fmt.Sscanf(input, "%d", &choice)
		if choice < 1 || choice > len(profiles) {
			fmt.Println("Invalid choice")
			return
		}
		profile = &profiles[choice-1]
	}

	if err := executor.ExecuteRDP(profile); err != nil {
		fmt.Printf("Error connecting: %v\n", err)
		return
	}

	fmt.Printf("Connected to %s@%s\n", profile.Username, profile.Host)
}

func cmdAdd(args []string, db *Database) {
	if len(args) < 3 {
		fmt.Println("Usage: rdp-manager add <host> <username> <password>")
		return
	}

	profile := &RDPProfile{
		Host:              args[0],
		Username:          args[1],
		Password:          args[2],
		Port:              3389,
		Resolution:        "1920x1080",
		ClipboardEnabled:  true,
		DisksEnabled:      true,
		DisksRedirect:     "all",
		DisksDynamic:      true,
		ProxyMode:         "direct",
	}

	if err := db.AddProfile(profile); err != nil {
		fmt.Printf("Error adding profile: %v\n", err)
		return
	}

	fmt.Printf("Profile added: %s@%s\n", profile.Username, profile.Host)
}

func cmdDelete(args []string, db *Database) {
	if len(args) < 2 {
		fmt.Println("Usage: rdp-manager delete <host> <username>")
		return
	}

	host := args[0]
	username := args[1]

	if err := db.DeleteProfile(host, username); err != nil {
		fmt.Printf("Error deleting profile: %v\n", err)
		return
	}

	fmt.Printf("Profile deleted: %s@%s\n", username, host)
}

func cmdProxy(args []string, db *Database) {
	if len(args) == 0 {
		fmt.Println("Usage: rdp-manager proxy <set|get> [<host:port>]")
		return
	}

	action := args[0]

	switch action {
	case "set":
		if len(args) < 2 {
			fmt.Println("Usage: rdp-manager proxy set <host:port>")
			return
		}

		settings := &GlobalSettings{
			GlobalProxyMode:    "enabled",
			GlobalProxyAddress: args[1],
		}

		if err := db.SaveGlobalSettings(settings); err != nil {
			fmt.Printf("Error setting proxy: %v\n", err)
			return
		}

		fmt.Printf("Global proxy set to: %s\n", args[1])

	case "get":
		settings, err := db.GetGlobalSettings()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		fmt.Printf("Global proxy mode: %s\n", settings.GlobalProxyMode)
		if settings.GlobalProxyMode == "enabled" {
			fmt.Printf("Proxy address: %s\n", settings.GlobalProxyAddress)
		}

	case "disable":
		settings := &GlobalSettings{
			GlobalProxyMode: "disabled",
		}

		if err := db.SaveGlobalSettings(settings); err != nil {
			fmt.Printf("Error disabling proxy: %v\n", err)
			return
		}

		fmt.Println("Global proxy disabled")

	default:
		fmt.Printf("Unknown proxy action: %s\n", action)
	}
}

func printHelp() {
	fmt.Println(`
RDP+ Extended - Remote Desktop Manager

Usage:
  rdp-manager [command] [options]

Commands:
  list                              List all saved profiles
  connect <host> [<username>]       Connect to a host
  add <host> <username> <password>  Add a new profile
  delete <host> <username>          Delete a profile
  proxy set <host:port>             Set global SOCKS5 proxy
  proxy get                         Get current proxy settings
  proxy disable                     Disable global proxy
  help                              Show this help message

Examples:
  rdp-manager list
  rdp-manager connect 192.168.1.100
  rdp-manager connect 192.168.1.100 admin
  rdp-manager add 192.168.1.100 admin MyP@ssw0rd
  rdp-manager proxy set 127.0.0.1:1080

For GUI mode, run: rdp-manager (without arguments)
	`)
}
