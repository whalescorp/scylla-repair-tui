# ScyllaDB Repair TUI

🔧 **Terminal User Interface for ScyllaDB Table Repair Management**

nodetool utility shipped with scylladb can ONLY start repairs in all-parallel mode. In such case, for large and loaded clusters, repair can allocate most of the host memory, cpu and IO for this tasks. To prevent it, i've wrote this utility. You can pass parallel parameter to choose desired parallelism and track progress of subsequent repair tasks in this utility.

## How does it looks like:

```bash
┌────────────────────────────────────────────────Connected to ScyllaDB v6.1.1 at localhost:10000 | Nodes: 1────────────────────────────────────────────────┐
│Address      Host ID                              Status DC          Rack   │Address          Host ID          Status          DC          Rack           │
│192.168.0.32 55262dbc-f14c-4eae-bbd5-75adf08d6949 UP     datacenter1 rack1  │                                                                             │
│                                                                            │                                                                             │
│                                                                            │                                                                             │
│                                                                            │                                                                             │
└──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
╔══════════════════════════════════════════════════════════════════════════Tables══════════════════════════════════════════════════════════════════════════╗
║Keyspace      Table                                           Status       Progress      Completed/Total      Retries      Start Time      Duration       ║
║some-kayspace users                                           PENDING      -             0/32                 -            -               -              ║
║some-keyspace accounts                                        PENDING      -             0/32                 -            -               -              ║
║some-keyspace account_states_index                            PENDING      -             0/32                 -            -               -              ║
║                                                                                                                                                          ║
║                                                                                                                                                          ║
╚══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╝
┌──────────────────────────────────────────────────────────────────────────Status──────────────────────────────────────────────────────────────────────────┐
│Ready | 14 tables found | Select a table | Ctrl+H: help                                                                                                   │
└──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

## ✨ Features

- **Variable parallelism** - You can set parallelism via arguments or via config
- **Interactive Terminal UI** - Beautiful text-based interface built with `tview`
- **Table View** - Browse keyspaces and tables with detailed repair status
- **Manual & Automated Repairs** - Run repairs on individual tables or all tables automatically
- **Progress Tracking** - Real-time progress bars, completion status, and timing information
- **Retry Logic** - Configurable retry attempts for failed repairs
- **Live Logs** - Dedicated log view with timestamped events

## 🚀 Quick Start

### Prerequisites

- **Go 1.24+** (for building from source)
- **ScyllaDB cluster** with REST API enabled (default port 10000)
- **Network access** to ScyllaDB nodes

### Installation

#### Option 1: Download Binary
```bash
# Download the latest release
wget https://github.com/whalescorp/scylla-repair-tui/releases/latest/download/scylla-repair-tui
chmod +x scylla-repair-tui
```

#### Option 2: Build from Source
```bash
git clone https://github.com/whalescorp/scylla-repair-tui.git
cd scylla-repair-tui
go build -o scylla-repair-tui
```

### Basic Usage

#### Connect to Local ScyllaDB
```bash
./scylla-repair-tui
```

#### Connect to Remote ScyllaDB
```bash
./scylla-repair-tui -host 192.168.1.100 -port 10000
```

#### Use Configuration File
```bash
./scylla-repair-tui -config config.json
```

## ⚙️ Configuration

### Command Line Options

| Option | Default | Description |
|--------|---------|-------------|
| `-host` | `localhost` | ScyllaDB host address |
| `-port` | `10000` | ScyllaDB REST API port |
| `-parallel` | `2` | Number of parallel repair workers |
| `-include-system` | `false` | Include system tables in repair |
| `-config` | - | Path to JSON configuration file |


## 🎮 Keyboard Controls

### Global Controls
| Key | Action |
|-----|--------|
| `Ctrl+H` | Show help modal |
| `Ctrl+C` | Exit application |
| `Escape` | Return to main view / Close modals |

### Main View
| Key | Action |
|-----|--------|
| `Ctrl+R` | Start repair of selected table |
| `Ctrl+A` | Start automatic repair of all tables |
| `Ctrl+L` | Switch to logs view |
| `Alt+PgUp` | Scroll nodes display up |
| `Alt+PgDn` | Scroll nodes display down |
| `↑/↓` | Navigate table list |
| `Enter` | Select table for repair |

### Auto Repair Mode
| Key | Action |
|-----|--------|
| `Ctrl+L` | Switch to logs view |
| `Ctrl+C` | Exit application |

## 📊 Interface Overview

### Header Section
- **Cluster Info**: ScyllaDB version, host, port, and node count
- **Node Status**: Real-time display of cluster nodes with DC/Rack information

### Main Table View
- **Keyspace**: Database keyspace name
- **Table**: Table name within keyspace
- **Status**: Current repair status (`PENDING`, `RUNNING`, `COMPLETED`, `FAILED`)
- **Progress**: Repair completion percentage
- **Completed/Total**: Number of completed vs total token ranges
- **Retries**: Number of retry attempts for failed ranges
- **Start Time**: When the repair began
- **Duration**: Total time elapsed

### Status Bar
- **Current Status**: Real-time application status
- **Quick Help**: `Ctrl+H: help` reminder

## 🔄 Repair Workflow

### Manual Repair
1. Navigate to desired table using arrow keys
2. Press `Ctrl+R` to start repair
3. Monitor progress in real-time
4. View detailed logs with `Ctrl+L`

### Automatic Repair
1. Press `Ctrl+A` to start auto repair mode
2. Application will repair all tables sequentially
3. Failed repairs are automatically retried
4. Use `Escape` to stop auto repair mode

### Retry Logic
- Failed repairs are automatically retried up to `max_retries` times
- Configurable delay between retry attempts
- Retry count displayed in table view

## 🏗️ Development

### Building
```bash
git clone https://github.com/your-org/scylla-repair-tui.git
cd scylla-repair-tui
go mod download
go build -o scylla-repair-tui
```

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request