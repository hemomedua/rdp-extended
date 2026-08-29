# RDP+ Extended

Powerful Remote Desktop Manager for Windows 7+ with profile management, global settings, and SOCKS5 proxy support.

## Features

- **Profile Management**: Save multiple credentials per host
- **System Integration**: Reads from Windows RDP history
- **SOCKS5 Proxy**: Per-host or global proxy configuration
- **Display Settings**: Configurable resolution, clipboard, disk redirection
- **Dual Interface**: Both GUI and CLI support
- **Zero Dependencies**: Single standalone executable, no .NET Framework required

## Requirements

- Windows 7 or later
- No additional dependencies (built with Go)

## Installation

Download the latest `rdp-extended.exe` from releases and run it.

Database and settings are stored in: `%LOCALAPPDATA%\RDPExtended\`

## GUI Usage

1. **Select Host**: Choose from dropdown (auto-populated from Windows RDP history + saved profiles)
2. **Select Username**: Lists all credentials for selected host
3. **Connect**: Launch RDP session
4. **Edit Profile**: Modify connection settings
5. **New Profile**: Add new credentials
6. **Settings**: Configure global proxy

## CLI Usage

```bash
# List all profiles
rdp-extended list

# Connect to host
rdp-extended connect 192.168.1.100
rdp-extended connect 192.168.1.100 admin

# Add new profile
rdp-extended add 192.168.1.100 admin MyPassword123

# Delete profile
rdp-extended delete 192.168.1.100 admin

# Configure global SOCKS5 proxy
rdp-extended proxy set 127.0.0.1:1080
rdp-extended proxy get
rdp-extended proxy disable

# Show help
rdp-extended help
```

## Configuration

### Display Settings per Profile
- Resolution: 1024x768, 1280x1024, 1366x768, 1440x900, 1600x1200, 1920x1080, 2560x1440, fullscreen, fit
- Clipboard: Enable/Disable
- Disk Redirection: All/None/Custom with dynamic drive support

### Proxy Configuration
- **Direct**: No proxy
- **Global**: Use global SOCKS5 proxy (configured in Settings)
- **Custom**: Per-profile proxy override

## Building from Source

```bash
# Download dependencies
go mod download
go mod tidy

# Build for Windows
GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o rdp-extended.exe

# Build for Linux (for testing)
go build -o rdp-extended
```

## Architecture

- `database.go`: SQLite profile storage
- `system_hosts.go`: Windows registry RDP history reader
- `rdp_executor.go`: mstsc.exe wrapper and command builder
- `gui.go`: Windows Forms UI (Walk library)
- `cli.go`: Command-line interface
- `main.go`: Entry point

## Future Enhancements

- Encrypted password storage (AES-256)
- Advanced proxy routing (per-domain rules)
- RDP session history and logging
- Profile import/export
- Custom RDP file support

## License

Free to use for personal and commercial purposes.

## Author

Hemom Dikal
