package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// NewScyllaRepairApp creates new application for repair
func NewScyllaRepairApp(config *Config) *ScyllaRepairApp {
	app := &ScyllaRepairApp{
		app:           tview.NewApplication(),
		pages:         tview.NewPages(),
		clusterInfo:   tview.NewTextView(),
		tablesList:    tview.NewTable(),
		logView:       tview.NewTextView(),
		statusBar:     tview.NewTextView(),
		repairConfig:  config.Repair,
		clusterConfig: config.Cluster,
		tableManager:  NewTableManager(),
	}

	app.setupUI()
	return app
}

// setupUI sets up user interface
func (app *ScyllaRepairApp) setupUI() {
	// Setup interface
	app.clusterInfo.SetBorder(true).SetTitle("Cluster Info")
	app.tablesList.SetBorder(true).SetTitle("Tables")
	app.logView.SetBorder(true).SetTitle("Logs (press escape to view tables)")
	app.statusBar.SetBorder(true).SetTitle("Status")

	// Setup table with list of tables
	app.tablesList.SetSelectable(true, false)
	app.tablesList.SetFixed(1, 0)
	app.tablesList.SetCell(0, 0, tview.NewTableCell("Keyspace").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	app.tablesList.SetCell(0, 1, tview.NewTableCell("Table").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	app.tablesList.SetCell(0, 2, tview.NewTableCell("Status").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	app.tablesList.SetCell(0, 3, tview.NewTableCell("Progress").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	app.tablesList.SetCell(0, 4, tview.NewTableCell("Completed/Total").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	app.tablesList.SetCell(0, 5, tview.NewTableCell("Retries").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	app.tablesList.SetCell(0, 6, tview.NewTableCell("Start Time").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	app.tablesList.SetCell(0, 7, tview.NewTableCell("Duration").SetTextColor(tcell.ColorYellow).SetSelectable(false))

	// Setup main page
	mainGrid := tview.NewGrid().
		SetRows(8, 0, 3).
		SetColumns(0).
		AddItem(app.clusterInfo, 0, 0, 1, 1, 0, 0, false).
		AddItem(app.tablesList, 1, 0, 1, 1, 0, 0, true).
		AddItem(app.statusBar, 2, 0, 1, 1, 0, 0, false)

	// Setup page with logs
	logPage := tview.NewGrid().
		SetRows(0).
		SetColumns(0).
		AddItem(app.logView, 0, 0, 1, 1, 0, 0, true)

	// Add pages
	app.pages.AddPage("main", mainGrid, true, true)
	app.pages.AddPage("logs", logPage, true, false)

	// Setup key handling
	app.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			// Always return to main page by Escape
			app.pages.SwitchToPage("main")
			return nil
		} else if event.Key() == tcell.KeyCtrlL {
			// Switch to page with logs
			app.pages.SwitchToPage("logs")
			return nil
		} else if event.Key() == tcell.KeyCtrlR {
			// Start repair process only on main page
			currentPage, _ := app.pages.GetFrontPage()
			if currentPage == "main" {
				go app.startRepair()
			}
			return nil
		} else if event.Key() == tcell.KeyCtrlC {
			// Exit application
			app.app.Stop()
			return nil
		}
		return event
	})

	// Setup main component
	app.app.SetRoot(app.pages, true)
}

func (app *ScyllaRepairApp) Shutdown() {
	if app.cancel != nil {
		app.cancel()
	}
	app.app.Stop()
}

func (app *ScyllaRepairApp) Run() error {
	go app.initialize()
	return app.app.Run()
}

func (app *ScyllaRepairApp) log(format string, args ...any) {
	message := fmt.Sprintf("[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	app.app.QueueUpdateDraw(func() {
		fmt.Fprint(app.logView, message)
		app.logView.ScrollToEnd()
	})
}

func (app *ScyllaRepairApp) updateStatusBar(message string) {
	app.app.QueueUpdateDraw(func() {
		app.statusBar.Clear()
		fmt.Fprintf(app.statusBar, "%s | Ctrl+R: repair selected table | Ctrl+L: logs | Escape: main view | Ctrl+C: quit", message)
	})
}

func (app *ScyllaRepairApp) initialize() {
	app.updateStatusBar("Connecting to ScyllaDB...")
	app.log("Initializing application...")

	ctx, cancel := context.WithCancel(context.Background())
	app.cancel = cancel

	// Connect to ScyllaDB admin API
	api := NewScyllaAPI(
		app.clusterConfig.Host,
		app.clusterConfig.Port,
		app.clusterConfig.Timeout,
	)

	// Check connection
	err := api.TestConnection(ctx)
	if err != nil {
		app.log("Error connecting to ScyllaDB: %v", err)
		app.updateStatusBar("Error: Failed to connect to ScyllaDB")
		return
	}

	// Get cluster information
	nodes, err := api.GetNodes(ctx)
	if err != nil {
		app.log("Error getting cluster information: %v", err)
		app.updateStatusBar("Error: Failed to get cluster information")
		return
	}

	// Display cluster information
	app.app.QueueUpdateDraw(func() {
		app.clusterInfo.Clear()
		fmt.Fprintf(app.clusterInfo, "Connected to ScyllaDB at %s:%d\n", app.clusterConfig.Host, app.clusterConfig.Port)
		fmt.Fprintf(app.clusterInfo, "Cluster nodes: %d (displaying details for first 4)\n", len(nodes))

		for _, node := range nodes {
			fmt.Fprintf(app.clusterInfo, "- %s (%s): %s, DC: %s, Rack: %s\n",
				node.Address, node.HostID, node.Status, node.Datacenter, node.Rack)
		}
	})

	// Get list of keyspaces
	keyspaces, err := api.GetKeyspaces(ctx)
	if err != nil {
		app.log("Error getting keyspaces: %v", err)
		app.updateStatusBar("Error: Failed to get keyspaces")
		return
	}

	// Get list of tables for each keyspace
	var tables []TableRepairInfo
	for _, keyspace := range keyspaces {
		// Skip system keyspaces if not enabled
		if !app.repairConfig.IncludeSystem && (strings.HasPrefix(keyspace, "system") || keyspace == "dse_system") {
			continue
		}

		tableNames, err := api.GetTables(ctx, keyspace)
		if err != nil {
			app.log("Error getting tables for keyspace %s: %v", keyspace, err)
			continue
		}

		for _, tableName := range tableNames {
			// Get token ranges for table
			app.log("Getting token ranges for %s.%s...", keyspace, tableName)
			ranges, err := api.GetTableRanges(ctx, keyspace, tableName)
			if err != nil {
				app.log("Error getting ranges for %s.%s: %v", keyspace, tableName, err)
				// Add table even if ranges cannot be obtained
				tables = append(tables, TableRepairInfo{
					Keyspace: keyspace,
					Table:    tableName,
					Status:   RepairStatusPending,
					Ranges:   []RangeRepairInfo{},
				})
				continue
			}

			// Convert all token ranges to repair ranges
			var allRanges []RangeRepairInfo
			for _, tokenRange := range ranges {
				allRanges = append(allRanges, RangeRepairInfo{
					StartToken: tokenRange.StartToken,
					EndToken:   tokenRange.EndToken,
					Status:     RepairStatusPending,
				})
			}

			tables = append(tables, TableRepairInfo{
				Keyspace:    keyspace,
				Table:       tableName,
				Status:      RepairStatusPending,
				TokenRanges: len(allRanges),
				Ranges:      allRanges,
			})

			app.log("Found %d ranges for %s.%s", len(allRanges), keyspace, tableName)
		}
	}

	// Sort tables by keyspace and name
	sort.Slice(tables, func(i, j int) bool {
		if tables[i].Keyspace == tables[j].Keyspace {
			return tables[i].Table < tables[j].Table
		}
		return tables[i].Keyspace < tables[j].Keyspace
	})

	// Save list of tables
	app.tableManager.SetTables(tables)

	// Update UI with tables
	app.updateTablesList()
	app.updateStatusBar(fmt.Sprintf("Ready | %d tables found | Select a table", len(tables)))
	app.log("Application initialized, found %d tables in %d keyspaces", len(tables), len(keyspaces))
}

// updateTablesList updates list of tables in UI
func (app *ScyllaRepairApp) updateTablesList() {
	app.app.QueueUpdateDraw(func() {
		// Clear table, leaving headers
		for i := 1; i < app.tablesList.GetRowCount(); i++ {
			for j := 0; j < app.tablesList.GetColumnCount(); j++ {
				app.tablesList.SetCell(i, j, tview.NewTableCell(""))
			}
		}

		// Fill table
		for i, table := range app.tableManager.GetTables() {
			row := i + 1 // +1 because of headers
			app.tablesList.SetCell(row, 0, tview.NewTableCell(table.Keyspace))
			app.tablesList.SetCell(row, 1, tview.NewTableCell(table.Table))

			// Status
			statusCell := tview.NewTableCell(string(table.Status))
			switch table.Status {
			case RepairStatusPending:
				statusCell.SetTextColor(tcell.ColorWhite)
			case RepairStatusRunning:
				statusCell.SetTextColor(tcell.ColorYellow)
			case RepairStatusCompleted:
				statusCell.SetTextColor(tcell.ColorGreen)
			case RepairStatusFailed:
				statusCell.SetTextColor(tcell.ColorRed)
			}
			app.tablesList.SetCell(row, 2, statusCell)

			// Progress
			var progressText string
			if table.Progress > 0 {
				progressText = fmt.Sprintf("%.1f%%", table.Progress*100)
			} else {
				progressText = "-"
			}
			app.tablesList.SetCell(row, 3, tview.NewTableCell(progressText))

			// Number of token ranges
			var rangesText string
			if table.TokenRanges > 0 {
				rangesText = fmt.Sprintf("%d/%d", table.Completed, table.TokenRanges)
			} else {
				rangesText = "-"
			}
			app.tablesList.SetCell(row, 4, tview.NewTableCell(rangesText))

			// Number of attempts
			var retriesText string
			if table.Retries > 0 {
				retriesText = fmt.Sprintf("%d", table.Retries)
			} else {
				retriesText = "-"
			}
			app.tablesList.SetCell(row, 5, tview.NewTableCell(retriesText))

			// Start time
			var startTimeText string
			if !table.StartTime.IsZero() {
				startTimeText = table.StartTime.Format("15:04:05")
			} else {
				startTimeText = "-"
			}
			app.tablesList.SetCell(row, 6, tview.NewTableCell(startTimeText))

			// Duration
			var durationText string
			if !table.StartTime.IsZero() {
				var duration time.Duration
				if !table.EndTime.IsZero() {
					duration = table.EndTime.Sub(table.StartTime)
				} else {
					duration = time.Since(table.StartTime)
				}
				durationText = formatDuration(duration)
			} else {
				durationText = "-"
			}
			app.tablesList.SetCell(row, 7, tview.NewTableCell(durationText))
		}
	})
}

// startRepair starts repair process for selected table
func (app *ScyllaRepairApp) startRepair() {
	if app.repairActive {
		app.log("Repair is already running")
		return
	}

	// Get current selected row
	selectedRow, _ := app.tablesList.GetSelection()
	if selectedRow <= 0 || selectedRow > app.tableManager.GetTableCount() {
		app.log("No table selected")
		app.updateStatusBar("No table selected")
		return
	}

	selectedTableIdx := selectedRow - 1 // -1 because of headers
	selectedTable, ok := app.tableManager.GetTableInfo(selectedTableIdx)
	if !ok {
		app.log("Invalid table selected")
		return
	}

	if len(selectedTable.Ranges) == 0 {
		app.log("No ranges found for table %s.%s", selectedTable.Keyspace, selectedTable.Table)
		app.updateStatusBar("No ranges found for selected table")
		return
	}

	app.repairActive = true

	app.log("Starting repair for table %s.%s with %d ranges (max %d retries per range)",
		selectedTable.Keyspace, selectedTable.Table, len(selectedTable.Ranges), app.repairConfig.MaxRetries)
	app.updateStatusBar(fmt.Sprintf("Repairing %s.%s...", selectedTable.Keyspace, selectedTable.Table))

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	app.cancel = cancel

	// Reset status of selected table and its ranges
	app.tableManager.StartTableRepair(selectedTableIdx)
	app.updateTablesList()

	// Create API client
	api := NewScyllaAPI(
		app.clusterConfig.Host,
		app.clusterConfig.Port,
		app.clusterConfig.Timeout,
	)

	// Create channel for token ranges
	type rangeWork struct {
		tableIdx int
		rangeIdx int
	}

	rangeCh := make(chan rangeWork, len(selectedTable.Ranges))
	var wg sync.WaitGroup

	// Start workers for processing token ranges
	for i := 0; i < app.repairConfig.Parallel; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for work := range rangeCh {
				app.repairRangeWithRetries(ctx, api, work.tableIdx, work.rangeIdx, workerID)
			}
		}(i)
	}

	// Send all ranges of selected table for processing
	go func() {
		defer close(rangeCh)

		for rangeIdx := range selectedTable.Ranges {
			select {
			case <-ctx.Done():
				return
			case rangeCh <- rangeWork{tableIdx: selectedTableIdx, rangeIdx: rangeIdx}:
				// Range sent for processing
			}
		}
	}()

	// Wait for completion of all workers
	go func() {
		wg.Wait()

		// Update final status of table
		app.tableManager.FinishTableRepair(selectedTableIdx)
		app.repairActive = false

		app.tableManager.UpdateTableProgress(selectedTableIdx)
		app.updateTablesList()
		app.log("Repair completed for table %s.%s", selectedTable.Keyspace, selectedTable.Table)
		app.updateStatusBar(fmt.Sprintf("Repair completed for %s.%s", selectedTable.Keyspace, selectedTable.Table))
	}()

	// Simple UI update every 300ms
	go func() {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !app.repairActive {
					return
				}
				app.tableManager.UpdateTableProgress(selectedTableIdx)
				app.updateTablesList()
			}
		}
	}()
}

// repairRange starts repair for specific token range (one attempt)
func (app *ScyllaRepairApp) repairRange(ctx context.Context, api *ScyllaAPI, tableIdx, rangeIdx, workerID int) error {
	table, ok := app.tableManager.GetTableInfo(tableIdx)
	if !ok {
		return fmt.Errorf("invalid table index")
	}

	if rangeIdx >= len(table.Ranges) {
		return fmt.Errorf("invalid range index")
	}

	rangeInfo := table.Ranges[rangeIdx]
	app.tableManager.SetRangeStatus(tableIdx, rangeIdx, RepairStatusRunning, "")

	tokenRange := fmt.Sprintf("%s:%s", rangeInfo.StartToken, rangeInfo.EndToken)
	app.log("Worker %d: Starting repair for %s.%s range %s", workerID, table.Keyspace, table.Table, tokenRange)

	// Start repair
	seqNum, err := api.StartRangeRepair(ctx, table.Keyspace, table.Table, tokenRange)
	if err != nil {
		app.log("Worker %d: Failed to start repair: %v", workerID, err)
		app.tableManager.SetRangeStatus(tableIdx, rangeIdx, RepairStatusFailed, fmt.Sprintf("Failed to start repair: %v", err))
		return err
	}

	app.log("Worker %d: Repair started with sequence number: %d", workerID, seqNum)

	// Wait for completion synchronously with timeout from config
	status, err := api.GetRepairStatus(ctx, seqNum, app.repairConfig.RepairTimeout)
	if err != nil {
		if strings.Contains(err.Error(), "repair timed out") {
			err = api.CancelRangeRepair(ctx, seqNum)
			if err != nil {
				app.log("Worker %d: Failed to cancel repair: %v", workerID, err)
				app.tableManager.SetRangeStatus(tableIdx, rangeIdx, RepairStatusFailed, fmt.Sprintf("Failed to cancel repair: %v", err))
				return err
			}
			app.log("Worker %d: Repair cancelled", workerID)
		}
		app.log("Worker %d: Error getting repair status: %v", workerID, err)
		app.tableManager.SetRangeStatus(tableIdx, rangeIdx, RepairStatusFailed, fmt.Sprintf("Status check failed: %v", err))
		return err
	}

	state, ok := status["state"].(string)
	if !ok {
		app.log("Worker %d: Invalid status response", workerID)
		app.tableManager.SetRangeStatus(tableIdx, rangeIdx, RepairStatusFailed, "Invalid status response")
		return fmt.Errorf("invalid status response")
	}

	app.log("Worker %d: Repair finished with status: %s", workerID, state)

	// Update status
	switch strings.ToUpper(state) {
	case "SUCCESSFUL", "SUCCESS":
		app.tableManager.SetRangeStatus(tableIdx, rangeIdx, RepairStatusCompleted, "")
		return nil // Success

	case "FAILED", "ERROR":
		errMsg := fmt.Sprintf("Repair failed with status: %s", state)
		if message, ok := status["message"].(string); ok {
			errMsg = message
		}
		app.tableManager.SetRangeStatus(tableIdx, rangeIdx, RepairStatusFailed, errMsg)
		return fmt.Errorf("repair failed: %s", state)

	case "RUNNING":
		errMsg := fmt.Sprintf("Repair returned unexpected RUNNING status: %s", state)
		app.tableManager.SetRangeStatus(tableIdx, rangeIdx, RepairStatusFailed, errMsg)
		return fmt.Errorf("unexpected RUNNING status")

	default:
		errMsg := fmt.Sprintf("Unknown repair status: %s", state)
		app.tableManager.SetRangeStatus(tableIdx, rangeIdx, RepairStatusFailed, errMsg)
		return fmt.Errorf("unknown status: %s", state)
	}
}

// repairRangeWithRetries performs repair with retries
func (app *ScyllaRepairApp) repairRangeWithRetries(ctx context.Context, api *ScyllaAPI, tableIdx, rangeIdx, workerID int) {
	table, ok := app.tableManager.GetTableInfo(tableIdx)
	if !ok {
		return
	}

	if rangeIdx >= len(table.Ranges) {
		return
	}

	rangeInfo := table.Ranges[rangeIdx]
	tokenRange := fmt.Sprintf("%s:%s", rangeInfo.StartToken, rangeInfo.EndToken)

	for attempt := 0; attempt <= app.repairConfig.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			// Context cancelled
			app.tableManager.SetRangeStatus(tableIdx, rangeIdx, RepairStatusFailed, "Repair cancelled")
			return
		}

		if attempt > 0 {
			app.log("Worker %d: Retry %d/%d for %s.%s range %s", workerID, attempt, app.repairConfig.MaxRetries, table.Keyspace, table.Table, tokenRange)
			app.tableManager.IncrementRetryCount(tableIdx, rangeIdx)
			time.Sleep(app.repairConfig.RetryDelay)
		}

		// Attempt to perform repair
		err := app.repairRange(ctx, api, tableIdx, rangeIdx, workerID)
		if err == nil {
			// Success!
			if attempt > 0 {
				app.log("Worker %d: Repair succeeded after %d retries for %s.%s range %s", workerID, attempt, table.Keyspace, table.Table, tokenRange)
			}
			return
		}

		if strings.Contains(err.Error(), "repair task is still running after cancellation") {
			app.log("Worker %d: Repair task is still running after cancellation for %s.%s range %s", workerID, table.Keyspace, table.Table, tokenRange)
			app.tableManager.SetRangeStatus(tableIdx, rangeIdx, RepairStatusFailed, fmt.Sprintf("Repair task is still running after cancellation: %v", err))
			return
		}

		// Failure
		if attempt == app.repairConfig.MaxRetries {
			// Exhausted all attempts
			app.log("Worker %d: Repair failed after %d attempts for %s.%s range %s: %v", workerID, attempt+1, table.Keyspace, table.Table, tokenRange, err)
			app.tableManager.SetRangeStatus(tableIdx, rangeIdx, RepairStatusFailed, fmt.Sprintf("Failed after %d attempts: %v", attempt+1, err))
			return
		} else {
			// Try again
			app.log("Worker %d: Repair attempt %d failed for %s.%s range %s: %v, will retry", workerID, attempt+1, table.Keyspace, table.Table, tokenRange, err)
		}
	}
}

// formatDuration formats duration in human readable format
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}
