package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

// Config contains all application configuration
type Config struct {
	Cluster ScyllaClusterConfig `json:"cluster"`
	Repair  RepairConfig        `json:"repair"`
}

// LoadConfig loads configuration from command line and/or file
func LoadConfig() (*Config, error) {
	// Default values
	config := &Config{
		Cluster: ScyllaClusterConfig{
			Host:    "localhost",
			Port:    10000,
			Timeout: 30 * time.Second,
		},
		Repair: RepairConfig{
			Parallel:      2,
			IncludeSystem: false,
			// FIXME: take from config
			RepairTimeout: 30 * time.Minute,
			MaxRetries:    3,
			RetryDelay:    30 * time.Second,
		},
	}

	// Command line parameters
	var configFile string
	flag.StringVar(&configFile, "config", "", "Path to configuration file (JSON)")

	// Cluster parameters
	var hostFlag string
	var portFlag int
	flag.StringVar(&hostFlag, "host", "", "ScyllaDB host (default: localhost)")
	flag.IntVar(&portFlag, "port", 0, "ScyllaDB port (default: 10000)")

	// Repair parameters
	var parallelFlag int
	var includeSystemFlag bool
	var tokenRangesFlag int
	flag.IntVar(&parallelFlag, "parallel", 0, "Number of parallel repair workers (default: 2)")
	flag.BoolVar(&includeSystemFlag, "include-system", false, "Include system tables")
	flag.IntVar(&tokenRangesFlag, "ranges", 0, "Number of token ranges for repair (0 for auto-detection)")

	flag.Parse()

	// Load from configuration file if specified
	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("error reading configuration file: %w", err)
		}

		if err := json.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("error parsing configuration file: %w", err)
		}
	}

	// Override values from command line if specified
	if hostFlag != "" {
		config.Cluster.Host = hostFlag
	}

	if portFlag != 0 {
		config.Cluster.Port = portFlag
	}

	if parallelFlag != 0 {
		config.Repair.Parallel = parallelFlag
	}

	if includeSystemFlag {
		config.Repair.IncludeSystem = true
	}

	return config, nil
}
