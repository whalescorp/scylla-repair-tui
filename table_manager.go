package main

import (
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TableManager manages table state and their repair
type TableManager struct {
	tables    []TableRepairInfo
	nodes     []Node
	nodesGrid *[2][]Node // Two columns of nodes
	mutex     sync.Mutex
}

// NewTableManager creates a new table manager
func NewTableManager() *TableManager {
	return &TableManager{
		tables:    []TableRepairInfo{},
		nodes:     []Node{},
		nodesGrid: &[2][]Node{},
	}
}

// SetTables sets the list of tables
func (tm *TableManager) SetTables(tables []TableRepairInfo) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	tm.tables = tables
}

// GetTables returns a copy of the table list
func (tm *TableManager) GetTables() []TableRepairInfo {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	result := make([]TableRepairInfo, len(tm.tables))
	copy(result, tm.tables)
	return result
}

// GetTableCount returns the number of tables
func (tm *TableManager) GetTableCount() int {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	return len(tm.tables)
}

// StartTableRepair starts table repair
func (tm *TableManager) StartTableRepair(tableIdx int) bool {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if tableIdx < 0 || tableIdx >= len(tm.tables) {
		return false
	}

	table := &tm.tables[tableIdx]
	table.Status = RepairStatusRunning
	table.Progress = 0
	table.Completed = 0
	table.Retries = 0
	table.StartTime = time.Now()
	table.EndTime = time.Time{}

	// Reset status of all ranges
	for j := range table.Ranges {
		table.Ranges[j].Status = RepairStatusPending
		table.Ranges[j].StartTime = time.Time{}
		table.Ranges[j].EndTime = time.Time{}
		table.Ranges[j].Error = ""
		table.Ranges[j].SequenceNum = 0
		table.Ranges[j].RetryCount = 0
	}

	return true
}

// GetTableInfo returns table information
func (tm *TableManager) GetTableInfo(tableIdx int) (TableRepairInfo, bool) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if tableIdx < 0 || tableIdx >= len(tm.tables) {
		return TableRepairInfo{}, false
	}

	return tm.tables[tableIdx], true
}

// SetRangeStatus sets range status
func (tm *TableManager) SetRangeStatus(tableIdx, rangeIdx int, status RepairStatus, err string) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if tableIdx < 0 || tableIdx >= len(tm.tables) {
		return
	}

	table := &tm.tables[tableIdx]
	if rangeIdx < 0 || rangeIdx >= len(table.Ranges) {
		return
	}

	rangeInfo := &table.Ranges[rangeIdx]
	rangeInfo.Status = status
	rangeInfo.Error = err

	if status == RepairStatusRunning && rangeInfo.StartTime.IsZero() {
		rangeInfo.StartTime = time.Now()
	}

	if status == RepairStatusCompleted || status == RepairStatusFailed {
		rangeInfo.EndTime = time.Now()
		if status == RepairStatusCompleted {
			rangeInfo.Progress = 1.0
		}
	}
}

// IncrementRetryCount increments retry counter for range
func (tm *TableManager) IncrementRetryCount(tableIdx, rangeIdx int) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if tableIdx < 0 || tableIdx >= len(tm.tables) {
		return
	}

	table := &tm.tables[tableIdx]
	if rangeIdx < 0 || rangeIdx >= len(table.Ranges) {
		return
	}

	table.Ranges[rangeIdx].RetryCount++
}

// UpdateTableProgress updates overall table progress
func (tm *TableManager) UpdateTableProgress(tableIdx int) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if tableIdx < 0 || tableIdx >= len(tm.tables) {
		return
	}

	table := &tm.tables[tableIdx]
	if len(table.Ranges) == 0 {
		return
	}

	completed := 0
	totalProgress := 0.0
	totalRetries := 0

	for _, rangeInfo := range table.Ranges {
		if rangeInfo.Status == RepairStatusCompleted {
			completed++
			totalProgress += 1.0
		} else if rangeInfo.Status == RepairStatusRunning {
			totalProgress += rangeInfo.Progress
		}
		totalRetries += rangeInfo.RetryCount
	}

	table.Completed = completed
	table.Progress = totalProgress / float64(len(table.Ranges))
	table.Retries = totalRetries
}

// FinishTableRepair finishes table repair
func (tm *TableManager) FinishTableRepair(tableIdx int) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if tableIdx < 0 || tableIdx >= len(tm.tables) {
		return
	}

	table := &tm.tables[tableIdx]
	allCompleted := true
	anyFailed := false

	for _, rangeInfo := range table.Ranges {
		if rangeInfo.Status == RepairStatusFailed {
			anyFailed = true
		} else if rangeInfo.Status != RepairStatusCompleted {
			allCompleted = false
		}
	}

	if anyFailed {
		table.Status = RepairStatusFailed
	} else if allCompleted {
		table.Status = RepairStatusCompleted
	}
	table.EndTime = time.Now()
}

// SetNodes sets the list of nodes and splits them into two columns
func (tm *TableManager) SetNodes(nodes []Node) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	tm.nodes = nodes

	// Split nodes between two columns
	halfNodes := (len(nodes) + 1) / 2

	// Clear existing grid
	tm.nodesGrid[0] = []Node{}
	tm.nodesGrid[1] = []Node{}

	// Fill first column
	for i := 0; i < halfNodes && i < len(nodes); i++ {
		tm.nodesGrid[0] = append(tm.nodesGrid[0], nodes[i])
	}

	// Fill second column
	for i := halfNodes; i < len(nodes); i++ {
		tm.nodesGrid[1] = append(tm.nodesGrid[1], nodes[i])
	}
}

// GetNodes returns a copy of the nodes list
func (tm *TableManager) GetNodes() []Node {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	result := make([]Node, len(tm.nodes))
	copy(result, tm.nodes)
	return result
}

// GetNodesGrid returns the nodes split into two columns
func (tm *TableManager) GetNodesGrid() [2][]Node {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	result := [2][]Node{}
	result[0] = make([]Node, len(tm.nodesGrid[0]))
	result[1] = make([]Node, len(tm.nodesGrid[1]))

	copy(result[0], tm.nodesGrid[0])
	copy(result[1], tm.nodesGrid[1])

	return result
}

// GetNodeCount returns the total number of nodes
func (tm *TableManager) GetNodeCount() int {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	return len(tm.nodes)
}

// UpdateNodesDisplay updates the display of nodes in the provided tables and separator
func (tm *TableManager) UpdateNodesDisplay(table1, table2 *tview.Table, separator *tview.Table) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Reset scroll position to top
	table1.SetOffset(0, 0)
	table2.SetOffset(0, 0)

	// Clear existing nodes (except headers)
	for i := 1; i < table1.GetRowCount(); i++ {
		for j := 0; j < table1.GetColumnCount(); j++ {
			table1.SetCell(i, j, tview.NewTableCell(""))
		}
	}
	for i := 1; i < table2.GetRowCount(); i++ {
		for j := 0; j < table2.GetColumnCount(); j++ {
			table2.SetCell(i, j, tview.NewTableCell(""))
		}
	}

	// Fill first table
	for i, node := range tm.nodesGrid[0] {
		row := i + 1 // +1 because of header
		table1.SetCell(row, 0, tview.NewTableCell(node.Address).SetExpansion(1))
		table1.SetCell(row, 1, tview.NewTableCell(node.HostID).SetExpansion(1))

		// Status with color
		statusCell := tview.NewTableCell(node.Status).SetExpansion(1)
		switch node.Status {
		case "UP":
			statusCell.SetTextColor(tcell.ColorGreen)
		case "DOWN":
			statusCell.SetTextColor(tcell.ColorRed)
		default:
			statusCell.SetTextColor(tcell.ColorYellow)
		}
		table1.SetCell(row, 2, statusCell)

		table1.SetCell(row, 3, tview.NewTableCell(node.Datacenter).SetExpansion(1))
		table1.SetCell(row, 4, tview.NewTableCell(node.Rack).SetExpansion(1))
	}

	// Fill remaining rows in table1 with empty cells to match table2 row count
	for i := len(tm.nodesGrid[0]); i < len(tm.nodesGrid[1]); i++ {
		row := i + 1 // +1 because of header
		table1.SetCell(row, 0, tview.NewTableCell("").SetExpansion(1))
		table1.SetCell(row, 1, tview.NewTableCell("").SetExpansion(1))
		table1.SetCell(row, 2, tview.NewTableCell("").SetExpansion(1))
		table1.SetCell(row, 3, tview.NewTableCell("").SetExpansion(1))
		table1.SetCell(row, 4, tview.NewTableCell("").SetExpansion(1))
	}

	// Fill second table
	for i, node := range tm.nodesGrid[1] {
		row := i + 1 // +1 because of header
		table2.SetCell(row, 0, tview.NewTableCell(node.Address).SetExpansion(1))
		table2.SetCell(row, 1, tview.NewTableCell(node.HostID).SetExpansion(1))

		// Status with color
		statusCell := tview.NewTableCell(node.Status).SetExpansion(1)
		switch node.Status {
		case "UP":
			statusCell.SetTextColor(tcell.ColorGreen)
		case "DOWN":
			statusCell.SetTextColor(tcell.ColorRed)
		default:
			statusCell.SetTextColor(tcell.ColorYellow)
		}
		table2.SetCell(row, 2, statusCell)

		table2.SetCell(row, 3, tview.NewTableCell(node.Datacenter).SetExpansion(1))
		table2.SetCell(row, 4, tview.NewTableCell(node.Rack).SetExpansion(1))
	}

	// Fill remaining rows in table2 with empty cells to match table1 row count
	for i := len(tm.nodesGrid[1]); i < len(tm.nodesGrid[0]); i++ {
		row := i + 1 // +1 because of header
		table2.SetCell(row, 0, tview.NewTableCell("").SetExpansion(1))
		table2.SetCell(row, 1, tview.NewTableCell("").SetExpansion(1))
		table2.SetCell(row, 2, tview.NewTableCell("").SetExpansion(1))
		table2.SetCell(row, 3, tview.NewTableCell("").SetExpansion(1))
		table2.SetCell(row, 4, tview.NewTableCell("").SetExpansion(1))
	}

	// Update separator with vertical lines
	// Clear existing separator cells
	for i := range separator.GetRowCount() {
		separator.SetCell(i, 0, tview.NewTableCell(""))
	}

	// Calculate maximum rows needed
	separatorHeight := max(len(tm.nodesGrid[0]), len(tm.nodesGrid[1]))
	separatorHeight += 1 // +1 for header

	if separatorHeight <= 5 {
		separatorHeight = 5
	}

	// Fill separator with vertical lines
	for i := range separatorHeight + 1 { // +20 extra to ensure full height
		separator.SetCell(i, 0, tview.NewTableCell("│").SetTextColor(tcell.ColorWhite).SetAlign(tview.AlignCenter).SetSelectable(false))
	}
}

func InitNodeDetailsTable(table *tview.Table) {
	table.SetBorder(false)
	table.SetSelectable(false, false)
	table.SetFixed(1, 0)
	table.SetCell(0, 0, tview.NewTableCell("Address").SetTextColor(tcell.ColorYellow).SetSelectable(false).SetExpansion(1))
	table.SetCell(0, 1, tview.NewTableCell("Host ID").SetTextColor(tcell.ColorYellow).SetSelectable(false).SetExpansion(1))
	table.SetCell(0, 2, tview.NewTableCell("Status").SetTextColor(tcell.ColorYellow).SetSelectable(false).SetExpansion(1))
	table.SetCell(0, 3, tview.NewTableCell("DC").SetTextColor(tcell.ColorYellow).SetSelectable(false).SetExpansion(1))
	table.SetCell(0, 4, tview.NewTableCell("Rack").SetTextColor(tcell.ColorYellow).SetSelectable(false).SetExpansion(1))
}
