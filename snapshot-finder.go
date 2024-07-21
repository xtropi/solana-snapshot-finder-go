package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

type ClusterNodesRequest struct {
	Jsonrpc string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
}

type Node struct {
	RPC string `json:"rpc"`
}

type ClusterNodesResponse struct {
	Result []Node `json:"result"`
}

func get_all_rpc_ips(url string) ([]string, error) {

	requestBody := ClusterNodesRequest{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "getClusterNodes",
	}

	jsonRequestBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}

	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonRequestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to send POST request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var response ClusterNodesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %v", err)
	}

	var rpcIps []string
	for _, node := range response.Result {
		if node.RPC != "" {
			rpcIps = append(rpcIps, node.RPC)
		}
	}

	return rpcIps, nil
}

func measure_speed(ip string, measureTime int) (float64, error) {

	url := fmt.Sprintf("http://%s/snapshot.tar.bz2", ip)
	client := &http.Client{
		Timeout: time.Duration(measureTime+2) * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("Failed to get URL %s: %v\n", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("Failed to get a valid response from %s, status code: %d\n", url, resp.StatusCode)
	}

	speed := calculate_speed(resp.Body, measureTime)
	return speed, nil
}

func calculate_speed(body io.ReadCloser, measureTime int) float64 {
	var loaded int64
	buf := make([]byte, 81920) // 80KB
	startTime := time.Now()
	for {
		n, err := body.Read(buf)
		if err != nil && err != io.EOF {
			fmt.Printf("Failed to read response body: %v\n", err)
			return 0
		}
		if n == 0 || time.Since(startTime).Seconds() >= float64(measureTime) {
			break
		}
		loaded += int64(n)
	}

	elapsed := time.Since(startTime).Seconds()
	speed := float64(loaded) / elapsed / (1024 * 1024) // Convert to MB/s
	return speed
}

func main() {
	// numCores := runtime.NumCPU()
	// runtime.GOMAXPROCS(numCores)
	// semaphore := make(chan struct{}, numCores) // ограничиваем количество одновременно запущенных горутин до числа ядер процессора
	url := "https://api.mainnet-beta.solana.com"
	rpcIps, err := get_all_rpc_ips(url)
	rpcIpsLen := len(rpcIps)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println(rpcIpsLen, " Nodes Total Founded")

	cycleRange := rpcIpsLen
	messages := make(chan string, cycleRange)
	var wg sync.WaitGroup
	for i := 0; i < cycleRange; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			speed, err := measure_speed(rpcIps[i], 5)
			if err != nil {
				messages <- fmt.Sprintf("IP %s: Error\n", rpcIps[i])
			} else {
				messages <- fmt.Sprintf("IP %s: %.2f MB/s\n", rpcIps[i], speed)
			}
		}()
	}

	wg.Wait()
	close(messages)
	i := 0
	for elem := range messages {
		fmt.Printf("%d) %s", i, elem)
		i++
	}

}
