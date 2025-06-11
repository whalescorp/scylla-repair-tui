package main

import (
	"context"
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
	SequenceNum int64
	RetryCount  int
	Progress    float64 // Progress from 0 to 1
}

// TableRepairInfo contains information about repair of specific table
type TableRepairInfo struct {
	Keyspace    string
	Table       string
	Status      RepairStatus
	StartTime   time.Time
	EndTime     time.Time
	Progress    float64           // From 0 to 1
	TokenRanges int               // Total number of token ranges
	Completed   int               // Number of processed token ranges
	Retries     int               // Total number of retries for all ranges
	Ranges      []RangeRepairInfo // List of all token ranges for repair
}

// ScyllaClusterConfig contains configuration for connecting to ScyllaDB cluster
type ScyllaClusterConfig struct {
	Host    string
	Port    int
	Timeout time.Duration
}

// RepairConfig contains configuration for repair process
type RepairConfig struct {
	Parallel      int           // Number of parallel repairs
	IncludeSystem bool          // Whether to include system tables
	RepairTimeout time.Duration // Timeout for single repair operation (default 30 minutes)
	MaxRetries    int           // Maximum number of retries for failed repair (default 3)
	RetryDelay    time.Duration // Delay between retries (default 30 seconds)
}

type ScylladbRepairTaskStatus struct {
	TaskID         string `json:"task_id"`
	State          string `json:"state"`
	Type           string `json:"type"`
	Scope          string `json:"scope"`
	Keyspace       string `json:"keyspace"`
	Table          string `json:"table"`
	Entity         string `json:"entity"`
	SequenceNumber int64  `json:"sequence_number"`
}

// ScyllaRepairApp represents the main application
type ScyllaRepairApp struct {
	app         *tview.Application
	pages       *tview.Pages
	nodesGrid   *tview.Grid
	nodesTable1 *tview.Table
	separator   *tview.Table
	nodesTable2 *tview.Table
	tablesList  *tview.Table
	logView     *tview.TextView
	statusBar   *tview.TextView
	helpModal   *tview.Modal

	clusterConfig ScyllaClusterConfig
	repairConfig  RepairConfig

	tableManager   *TableManager
	repairActive   bool
	autoRepairMode bool
	cancel         context.CancelFunc
	scyllaVersion  string
}

// Node represents information about cluster node
type Node struct {
	Address    string
	Datacenter string
	Rack       string
	Status     string
	HostID     string
}
