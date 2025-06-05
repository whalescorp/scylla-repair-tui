package main

import (
	"sync"
	"time"
)

// TableManager manages table state and their repair
type TableManager struct {
	tables []TableRepairInfo
	mutex  sync.Mutex
}

// NewTableManager creates a new table manager
func NewTableManager() *TableManager {
	return &TableManager{
		tables: []TableRepairInfo{},
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
	table.Error = ""

	// Reset status of all ranges
	for j := range table.Ranges {
		table.Ranges[j].Status = RepairStatusPending
		table.Ranges[j].StartTime = time.Time{}
		table.Ranges[j].EndTime = time.Time{}
		table.Ranges[j].Error = ""
		table.Ranges[j].TaskID = ""
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
