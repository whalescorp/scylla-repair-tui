package main

import (
	"context"
	"net/http"
	"time"

	"github.com/rivo/tview"
)

// RepairStatus represents the status of repair process
type RepairStatus string

const (
	RepairStatusPending   RepairStatus = "PENDING"
	RepairStatusRunning   RepairStatus = "RUNNING"
	RepairStatusCompleted RepairStatus = "COMPLETED"
	RepairStatusFailed    RepairStatus = "FAILED"
)

// RangeRepairInfo contains information about repair of specific token range
type RangeRepairInfo struct {
	StartToken  string
	EndToken    string
	Status      RepairStatus
	StartTime   time.Time
	EndTime     time.Time
	Error       string
	TaskID      string
	SequenceNum int64
	RetryCount  int
	RepairID    string  // Repair task identifier
	Progress    float64 // Progress from 0 to 1
	TokenRanges int     // Total number of sub-ranges (usually 1 for individual range)
	Completed   int     // Number of completed sub-ranges
}

// TableRepairInfo contains information about repair of specific table
type TableRepairInfo struct {
	Keyspace    string
	Table       string
	Status      RepairStatus
	StartTime   time.Time
	EndTime     time.Time
	Error       string
	Progress    float64           // From 0 to 1
	TokenRanges int               // Total number of token ranges
	Completed   int               // Number of processed token ranges
	Retries     int               // Total number of retries for all ranges
	RepairID    string            // Repair task identifier (not used in new logic)
	Ranges      []RangeRepairInfo // List of all token ranges for repair
}

// ScyllaClusterConfig contains configuration for connecting to ScyllaDB cluster
type ScyllaClusterConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	Timeout  time.Duration
}

// RepairConfig contains configuration for repair process
type RepairConfig struct {
	Parallel       int           // Number of parallel repairs
	IncludeSystem  bool          // Whether to include system tables
	TokenRanges    int           // Number of token ranges for repair (0 for auto-detection)
	TableBlacklist []string      // List of tables that should not be repaired
	RepairTimeout  time.Duration // Timeout for single repair operation (default 30 minutes)
	MaxRetries     int           // Maximum number of retries for failed repair (default 3)
	RetryDelay     time.Duration // Delay between retries (default 30 seconds)
}

// ScyllaRepairApp represents the main application
type ScyllaRepairApp struct {
	app          *tview.Application
	pages        *tview.Pages
	clusterInfo  *tview.TextView
	tablesList   *tview.Table
	logView      *tview.TextView
	progressView *tview.TextView
	statusBar    *tview.TextView

	httpClient    *http.Client
	clusterConfig ScyllaClusterConfig
	repairConfig  RepairConfig

	tableManager *TableManager
	repairActive bool
	cancel       context.CancelFunc
}

// Node represents information about cluster node
type Node struct {
	Address    string
	Datacenter string
	Rack       string
	Status     string
	State      string
	Load       string
	TokenCount int
	HostID     string
}
