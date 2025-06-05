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
	client  *http.Client
	baseURL string
	auth    string
}

// NewScyllaAPI creates new object for working with ScyllaDB API
func NewScyllaAPI(host string, port int, username, password string, timeout time.Duration) *ScyllaAPI {
	client := &http.Client{
		Timeout: timeout,
	}

	baseURL := fmt.Sprintf("http://%s:%d", host, port)
	var auth string
	if username != "" && password != "" {
		auth = fmt.Sprintf("%s:%s", username, password)
	}

	return &ScyllaAPI{
		client:  client,
		baseURL: baseURL,
		auth:    auth,
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

// LoadMapEntry represents record in load map
type LoadMapEntry struct {
	Key   string  `json:"key"`
	Value float64 `json:"value"`
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
			}

			// Try to get rack
			rackResp, err := api.makeRequest(ctx, "GET", fmt.Sprintf("/snitch/rack?host=%s", address), nil)
			if err == nil {
				defer rackResp.Body.Close()
				if rackBody, err := io.ReadAll(rackResp.Body); err == nil {
					node.Rack = strings.Trim(string(rackBody), `"`)
				}
			}

			// Get node load
			loadResp, err := api.makeRequest(ctx, "GET", "/storage_service/load_map", nil)
			if err == nil {
				defer loadResp.Body.Close()
				var loadMap []LoadMapEntry
				if err := json.NewDecoder(loadResp.Body).Decode(&loadMap); err == nil {
					for _, loadEntry := range loadMap {
						if loadEntry.Key == address {
							node.Load = fmt.Sprintf("%.2f MB", loadEntry.Value/(1024*1024)) // Convert bytes to MB
							break
						}
					}
				}
			}
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// StartRepair starts repair for specified table
func (api *ScyllaAPI) StartRepair(ctx context.Context, keyspace, table string, options map[string]string) (string, error) {
	params := make(url.Values)
	params.Set("columnFamilies", table)

	// Add additional options
	for k, v := range options {
		params.Set(k, v)
	}

	path := fmt.Sprintf("/storage_service/repair_async/%s", keyspace)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := api.makeRequest(ctx, "POST", path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to start repair for %s.%s: %w", keyspace, table, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read repair response: %w", err)
	}

	// Response should contain repair task ID
	repairID := string(body)
	repairID = strings.TrimSpace(repairID)
	repairID = strings.Trim(repairID, `"`) // Remove quotes if any
	return repairID, nil
}

// GetRepairStatus checks repair task status by sequence number with configurable timeout
func (api *ScyllaAPI) GetRepairStatus(ctx context.Context, sequenceNumber int64, timeout time.Duration) (map[string]interface{}, error) {
	// Convert timeout to seconds for API
	timeoutSeconds := int(timeout.Seconds())
	path := fmt.Sprintf("/storage_service/repair_status?id=%d&timeout=%d", sequenceNumber, timeoutSeconds)
	resp, err := api.makeRequest(ctx, "GET", path, nil)
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
	status := make(map[string]interface{})
	status["state"] = statusStr

	// Process only real statuses from ScyllaDB sources
	// For repair_status enum: RUNNING, SUCCESSFUL, FAILED
	// For task_state enum: created, running, done, failed
	switch strings.ToUpper(statusStr) {
	case "RUNNING":
		status["progress"] = 0.5 // Approximate value for running repair
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

// TestConnection checks connection to ScyllaDB
func (api *ScyllaAPI) TestConnection(ctx context.Context) error {
	resp, err := api.makeRequest(ctx, "GET", "/storage_service/release_version", nil)
	if err != nil {
		return fmt.Errorf("failed to connect to ScyllaDB: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ScyllaDB returned status code: %d", resp.StatusCode)
	}

	return nil
}

// makeRequest performs HTTP request to ScyllaDB API
func (api *ScyllaAPI) makeRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := api.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if api.auth != "" {
		req.SetBasicAuth(strings.Split(api.auth, ":")[0], strings.Split(api.auth, ":")[1])
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

// GetLocalEndpoint returns address of local node
func (api *ScyllaAPI) GetLocalEndpoint(ctx context.Context) (string, error) {
	// Try to get from different sources
	endpoints := []string{
		"/storage_service/hostid/local",
		"/snitch/datacenter", // this will return information about local node
	}

	for _, endpoint := range endpoints {
		resp, err := api.makeRequest(ctx, "GET", endpoint, nil)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		result := strings.Trim(string(body), `"`)
		if result != "" {
			return result, nil
		}
	}

	return "", fmt.Errorf("failed to determine local endpoint")
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
