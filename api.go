package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ScyllaAPI provides methods for interacting with ScyllaDB REST API
type ScyllaAPI struct {
	client         *http.Client
	baseURL        string
	requestTimeout time.Duration
}

// NewScyllaAPI creates new object for working with ScyllaDB API
func NewScyllaAPI(host string, port int, timeout time.Duration) *ScyllaAPI {
	client := &http.Client{}

	baseURL := fmt.Sprintf("http://%s:%d", host, port)

	return &ScyllaAPI{
		client:         client,
		baseURL:        baseURL,
		requestTimeout: timeout,
	}
}

// GetKeyspaces returns list of keyspaces in cluster
func (api *ScyllaAPI) GetKeyspaces(ctx context.Context) ([]string, error) {
	resp, err := api.makeRequest(ctx, "GET", "/storage_service/keyspaces", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get keyspaces: %w", err)
	}
	defer resp.Body.Close()

	var keyspaces []string
	if err := json.NewDecoder(resp.Body).Decode(&keyspaces); err != nil {
		return nil, fmt.Errorf("failed to decode keyspaces response: %w", err)
	}

	return keyspaces, nil
}

// ColumnFamilyInfo represents information about table (column family)
type ColumnFamilyInfo struct {
	Keyspace string `json:"ks"`
	Table    string `json:"cf"`
	Type     string `json:"type"`
}

// GetTables returns list of tables in specified keyspace
func (api *ScyllaAPI) GetTables(ctx context.Context, keyspace string) ([]string, error) {
	// Get list of all tables
	resp, err := api.makeRequest(ctx, "GET", "/column_family/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get column families: %w", err)
	}
	defer resp.Body.Close()

	var columnFamilies []ColumnFamilyInfo
	if err := json.NewDecoder(resp.Body).Decode(&columnFamilies); err != nil {
		return nil, fmt.Errorf("failed to decode column families response: %w", err)
	}

	// Filter tables by specified keyspace
	var tables []string
	for _, cf := range columnFamilies {
		if cf.Keyspace == keyspace {
			tables = append(tables, cf.Table)
		}
	}

	return tables, nil
}

// KeyValuePair represents key-value pair from ScyllaDB API
type KeyValuePair struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

// GetNodes returns information about nodes in cluster
func (api *ScyllaAPI) GetNodes(ctx context.Context) ([]Node, error) {
	// Get Host ID of all nodes
	resp, err := api.makeRequest(ctx, "GET", "/storage_service/host_id", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get node IDs: %w", err)
	}
	defer resp.Body.Close()

	var hostIDs []KeyValuePair
	if err := json.NewDecoder(resp.Body).Decode(&hostIDs); err != nil {
		return nil, fmt.Errorf("failed to decode host IDs: %w", err)
	}

	// Get list of live nodes
	resp, err = api.makeRequest(ctx, "GET", "/gossiper/endpoint/live/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get live endpoints: %w", err)
	}
	defer resp.Body.Close()

	var liveEndpoints []string
	if err := json.NewDecoder(resp.Body).Decode(&liveEndpoints); err != nil {
		return nil, fmt.Errorf("failed to decode live endpoints: %w", err)
	}

	// Get list of down nodes
	resp, err = api.makeRequest(ctx, "GET", "/gossiper/endpoint/down/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get down endpoints: %w", err)
	}
	defer resp.Body.Close()

	var downEndpoints []string
	if err := json.NewDecoder(resp.Body).Decode(&downEndpoints); err != nil {
		return nil, fmt.Errorf("failed to decode down endpoints: %w", err)
	}

	// Create sets for fast status check
	liveSet := make(map[string]bool)
	for _, endpoint := range liveEndpoints {
		liveSet[endpoint] = true
	}

	downSet := make(map[string]bool)
	for _, endpoint := range downEndpoints {
		downSet[endpoint] = true
	}

	// Get simple states for additional information
	resp, err = api.makeRequest(ctx, "GET", "/failure_detector/simple_states", nil)
	if err != nil {
		// Not critical, continue without additional information
		fmt.Printf("Warning: failed to get simple states: %v\n", err)
	} else {
		defer resp.Body.Close()
		// Decode simple states if available
		var simpleStates []KeyValuePair
		if err := json.NewDecoder(resp.Body).Decode(&simpleStates); err == nil {
			// Can be used for additional information if needed
			_ = simpleStates
		}
	}

	var nodes []Node
	for _, hostIDPair := range hostIDs {
		address := hostIDPair.Key

		node := Node{
			Address: address,
			HostID:  fmt.Sprintf("%v", hostIDPair.Value),
			Status:  "UNKNOWN", // Default
		}

		// Determine status based on gossiper data
		if liveSet[address] {
			node.Status = "UP"
		} else if downSet[address] {
			node.Status = "DOWN"
		} else {
			// If not in live and not in down, consider UNKNOWN
			node.Status = "UNKNOWN"
		}

		// Get information about datacenter and rack for each node
		if node.Status == "UP" {
			// Try to get datacenter
			dcResp, err := api.makeRequest(ctx, "GET", fmt.Sprintf("/snitch/datacenter?host=%s", address), nil)
			if err == nil {
				defer dcResp.Body.Close()
				if dcBody, err := io.ReadAll(dcResp.Body); err == nil {
					node.Datacenter = strings.Trim(string(dcBody), `"`)
				}
			} else {
				node.Datacenter = "N/A"
			}

			// Try to get rack
			rackResp, err := api.makeRequest(ctx, "GET", fmt.Sprintf("/snitch/rack?host=%s", address), nil)
			if err == nil {
				defer rackResp.Body.Close()
				if rackBody, err := io.ReadAll(rackResp.Body); err == nil {
					node.Rack = strings.Trim(string(rackBody), `"`)
				}
			} else {
				node.Rack = "N/A"
			}
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// GetRepairStatus checks repair task status by sequence number with configurable timeout
func (api *ScyllaAPI) GetRepairStatus(ctx context.Context, sequenceNumber int64, timeout time.Duration) (map[string]any, error) {
	// Convert timeout to seconds for API
	timeoutSeconds := int(timeout.Seconds())
	path := fmt.Sprintf("/storage_service/repair_status?id=%d&timeout=%d", sequenceNumber, timeoutSeconds)
	resp, err := api.makeRequest(ctx, "GET", path, nil, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to get repair status for sequence %d: %w", sequenceNumber, err)
	}
	defer resp.Body.Close()

	// Read response as string
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read repair status response: %w", err)
	}

	statusStr := strings.TrimSpace(string(body))
	statusStr = strings.Trim(statusStr, `"`)

	// Create status map
	status := make(map[string]any)
	status["state"] = statusStr

	// Process only real statuses from ScyllaDB sources
	// For repair_status enum: RUNNING, SUCCESSFUL, FAILED
	// For task_state enum: created, running, done, failed
	switch strings.ToUpper(statusStr) {
	case "RUNNING":
		return nil, fmt.Errorf("repair timed out")
	case "SUCCESSFUL", "SUCCESS":
		status["progress"] = 1.0
		status["success"] = 1.0
	case "FAILED", "ERROR":
		status["progress"] = 0.0
		status["message"] = "Repair failed"
	default:
		// For unknown statuses return as is
		status["progress"] = 0.0
		status["message"] = fmt.Sprintf("Unknown repair status: %s", statusStr)
	}

	return status, nil
}

// CancelRangeRepair cancels repair for specific token range
func (api *ScyllaAPI) CancelRangeRepair(ctx context.Context, seqNum int64) error {
	resp, err := api.makeRequest(ctx, "GET", "/task_manager/list_module_tasks/repair", nil)
	if err != nil {
		return fmt.Errorf("failed to get repair tasks: %w", err)
	}
	defer resp.Body.Close()

	var tasksBefore []ScylladbRepairTaskStatus
	if err := json.NewDecoder(resp.Body).Decode(&tasksBefore); err != nil {
		return fmt.Errorf("failed to decode repair tasks: %w", err)
	}

	for _, task := range tasksBefore {
		if task.SequenceNumber == seqNum {
			path := fmt.Sprintf("/task_manager/abort_task/%s", task.TaskID)
			// Let's assume that task cleanup process is very costly, so we give it 5 minutes to complete
			resp, err := api.makeRequest(ctx, "POST", path, nil, 5*time.Minute)
			if err != nil {
				return fmt.Errorf("failed to cancel range repair: %w", err)
			}
			defer resp.Body.Close()
			return nil
		}
	}

	resp, err = api.makeRequest(ctx, "GET", "/task_manager/list_module_tasks/repair", nil)
	if err != nil {
		return fmt.Errorf("failed to get repair tasks: %w", err)
	}
	defer resp.Body.Close()

	var tasksAfter []ScylladbRepairTaskStatus
	if err := json.NewDecoder(resp.Body).Decode(&tasksAfter); err != nil {
		return fmt.Errorf("failed to decode repair tasks: %w", err)
	}

	for _, task := range tasksAfter {
		if task.SequenceNumber == seqNum {
			return fmt.Errorf("repair task is still running after cancellation")
		}
	}

	return fmt.Errorf("repair task not found")
}

// GetVersion returns ScyllaDB version in semver format
func (api *ScyllaAPI) GetVersion(ctx context.Context) (string, error) {
	resp, err := api.makeRequest(ctx, "GET", "/storage_service/scylla_release_version", nil)
	if err != nil {
		return "", fmt.Errorf("version request to ScyllaDB failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read version response: %w", err)
	}

	version := strings.TrimSpace(string(body))
	version = strings.Trim(version, `"`)

	// Extract semver part from version string
	if dashIndex := strings.Index(version, "-"); dashIndex != -1 {
		version = version[:dashIndex]
	}

	return version, nil
}

// makeRequest performs HTTP request to ScyllaDB API
func (api *ScyllaAPI) makeRequest(ctx context.Context, method, path string, body io.Reader, timeout ...time.Duration) (*http.Response, error) {
	timeoutLocal := api.requestTimeout
	ctxLocal := ctx
	if len(timeout) > 0 {
		timeoutLocal = timeout[0]
	}

	ctxLocal, _ = context.WithTimeout(ctx, timeoutLocal)

	url := api.baseURL + path
	req, err := http.NewRequestWithContext(ctxLocal, method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return api.client.Do(req)
}

// TokenRange represents token range for repair
type TokenRange struct {
	StartToken      string           `json:"start_token"`
	EndToken        string           `json:"end_token"`
	Endpoints       []string         `json:"endpoints"`
	RpcEndpoints    []string         `json:"rpc_endpoints"`
	EndpointDetails []EndpointDetail `json:"endpoint_details"`
}

// EndpointDetail contains detailed information about endpoint
type EndpointDetail struct {
	Host       string `json:"host"`
	Datacenter string `json:"datacenter"`
	Rack       string `json:"rack"`
}

// GetTableRanges returns token ranges for specified table
func (api *ScyllaAPI) GetTableRanges(ctx context.Context, keyspace, table string) ([]TokenRange, error) {
	path := fmt.Sprintf("/storage_service/describe_ring/%s?table=%s", keyspace, table)
	resp, err := api.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get ranges for %s.%s: %w", keyspace, table, err)
	}
	defer resp.Body.Close()

	var ranges []TokenRange
	if err := json.NewDecoder(resp.Body).Decode(&ranges); err != nil {
		return nil, fmt.Errorf("failed to decode ranges response: %w", err)
	}

	return ranges, nil
}

// StartRangeRepair starts repair for specific token range
func (api *ScyllaAPI) StartRangeRepair(ctx context.Context, keyspace, table, tokenRange string) (int64, error) {
	params := make(url.Values)
	params.Set("ranges", tokenRange)
	params.Set("columnFamilies", table)
	params.Set("jobThreads", "1")
	params.Set("primaryRange", "false")
	params.Set("pullRepair", "false")
	params.Set("incremental", "false")
	params.Set("parallelism", "sequential")
	params.Set("ignoreUnreplicatedKeyspaces", "false")
	params.Set("trace", "false")

	path := fmt.Sprintf("/storage_service/repair_async/%s?%s", keyspace, params.Encode())
	resp, err := api.makeRequest(ctx, "POST", path, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to start range repair: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read repair response: %w", err)
	}

	seqNumStr := strings.TrimSpace(string(body))
	seqNum := int64(0)
	if _, err := fmt.Sscanf(seqNumStr, "%d", &seqNum); err != nil {
		return 0, fmt.Errorf("failed to parse sequence number: %w", err)
	}

	return seqNum, nil
}
