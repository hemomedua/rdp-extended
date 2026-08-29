package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

var (
	db              *Database
	rdpExecutor     *RDPExecutor
	currentHosts    []string
	currentProfiles []RDPProfile
)

// Simple GUI using Windows MessageBox for now (will be expanded)
func CreateMainWindow(database *Database, executor *RDPExecutor) error {
	db = database
	rdpExecutor = executor

	// Load system and database hosts
	hosts, _ := GetSystemHostsWithDB(db)
	sort.Strings(hosts)
	currentHosts = hosts

	// For now, show a simple menu-driven interface in CLI
	showMainMenu()
	return nil
}

func showMainMenu() {
	reader := bufio.NewReader(os.Stdin)
	
	fmt.Println("\n=== RDP+ Extended ===")
	fmt.Println("1. Connect to host")
	fmt.Println("2. Add new profile")
	fmt.Println("3. Edit profile")
	fmt.Println("4. List profiles")
	fmt.Println("5. Settings")
	fmt.Println("6. Exit")
	fmt.Print("Select option: ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	
	var choice int
	fmt.Sscanf(input, "%d", &choice)

	switch choice {
	case 1:
		menuConnect()
	case 2:
		menuAddProfile()
	case 3:
		menuEditProfile()
	case 4:
		menuList()
	case 5:
		menuSettings()
	case 6:
		return
	default:
		fmt.Println("Invalid option")
		showMainMenu()
	}
}

func menuConnect() {
	fmt.Print("Enter host (or leave empty to select): ")
	var host string
	fmt.Scanln(&host)

	if host == "" {
		fmt.Println("Available hosts:")
		for i, h := range currentHosts {
			fmt.Printf("%d. %s\n", i+1, h)
		}
		fmt.Print("Select host number: ")
		var idx int
		fmt.Scanln(&idx)
		if idx > 0 && idx <= len(currentHosts) {
			host = currentHosts[idx-1]
		}
	}

	profiles, _ := db.GetProfilesByHost(host)
	if len(profiles) == 0 {
		fmt.Printf("No profiles found for %s\n", host)
		showMainMenu()
		return
	}

	if len(profiles) == 1 {
		if err := rdpExecutor.ExecuteRDP(&profiles[0]); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	} else {
		fmt.Println("Select username:")
		for i, p := range profiles {
			fmt.Printf("%d. %s\n", i+1, p.Username)
		}
		fmt.Print("Select: ")
		var idx int
		fmt.Scanln(&idx)
		if idx > 0 && idx <= len(profiles) {
			if err := rdpExecutor.ExecuteRDP(&profiles[idx-1]); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		}
	}
	showMainMenu()
}

func menuAddProfile() {
	var profile RDPProfile

	fmt.Print("Host: ")
	fmt.Scanln(&profile.Host)
	fmt.Print("Username: ")
	fmt.Scanln(&profile.Username)
	fmt.Print("Password: ")
	fmt.Scanln(&profile.Password)

	profile.Port = 3389
	profile.Resolution = "1920x1080"
	profile.ClipboardEnabled = true
	profile.DisksEnabled = true
	profile.DisksRedirect = "all"
	profile.DisksDynamic = true
	profile.ProxyMode = "direct"

	if err := db.AddProfile(&profile); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println("Profile saved successfully")
	}

	// Reload hosts
	currentHosts, _ = GetSystemHostsWithDB(db)
	sort.Strings(currentHosts)

	showMainMenu()
}

func menuEditProfile() {
	fmt.Print("Host: ")
	var host string
	fmt.Scanln(&host)

	profiles, _ := db.GetProfilesByHost(host)
	if len(profiles) == 0 {
		fmt.Printf("No profiles found for %s\n", host)
		showMainMenu()
		return
	}

	if len(profiles) > 1 {
		fmt.Println("Select profile:")
		for i, p := range profiles {
			fmt.Printf("%d. %s\n", i+1, p.Username)
		}
		var idx int
		fmt.Print("Select: ")
		fmt.Scanln(&idx)
		if idx < 1 || idx > len(profiles) {
			showMainMenu()
			return
		}
		profiles = []RDPProfile{profiles[idx-1]}
	}

	profile := profiles[0]

	fmt.Printf("Host [%s]: ", profile.Host)
	var input string
	fmt.Scanln(&input)
	if input != "" {
		profile.Host = input
	}

	fmt.Printf("Username [%s]: ", profile.Username)
	fmt.Scanln(&input)
	if input != "" {
		profile.Username = input
	}

	fmt.Printf("Password [***]: ")
	fmt.Scanln(&input)
	if input != "" {
		profile.Password = input
	}

	if err := db.AddProfile(&profile); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println("Profile updated successfully")
	}

	showMainMenu()
}

func menuList() {
	hosts, _ := db.GetAllHosts()
	if len(hosts) == 0 {
		fmt.Println("No profiles found")
		showMainMenu()
		return
	}

	sort.Strings(hosts)
	fmt.Println("\n=== Profiles ===")
	for _, host := range hosts {
		profiles, _ := db.GetProfilesByHost(host)
		fmt.Printf("\n%s:\n", host)
		for _, p := range profiles {
			fmt.Printf("  - %s (port %d)\n", p.Username, p.Port)
		}
	}
	fmt.Println()

	showMainMenu()
}

func menuSettings() {
	fmt.Println("\n=== Settings ===")
	fmt.Println("1. Configure global SOCKS5 proxy")
	fmt.Println("2. Back")
	fmt.Print("Select: ")

	var choice int
	fmt.Scanln(&choice)

	if choice == 1 {
		fmt.Println("\nGlobal SOCKS5 Proxy")
		fmt.Println("1. Enable")
		fmt.Println("2. Disable")
		fmt.Print("Select: ")

		var enable int
		fmt.Scanln(&enable)

		if enable == 1 {
			fmt.Print("Proxy address (host:port): ")
			var proxy string
			fmt.Scanln(&proxy)

			newSettings := &GlobalSettings{
				GlobalProxyMode:    "enabled",
				GlobalProxyAddress: proxy,
			}
			db.SaveGlobalSettings(newSettings)
			fmt.Println("Proxy enabled")
		} else {
			newSettings := &GlobalSettings{
				GlobalProxyMode: "disabled",
			}
			db.SaveGlobalSettings(newSettings)
			fmt.Println("Proxy disabled")
		}
	}

	showMainMenu()
}
