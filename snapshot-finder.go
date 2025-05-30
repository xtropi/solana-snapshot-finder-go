package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"
)

const MEASURE_SECONDS int = 3
const MAINNET_CLUSTER_URL string = "https://api.mainnet-beta.solana.com"

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

func removeDuplicates(strings []string) []string {
	// Используем карту для отслеживания уникальных строк
	uniqueStrings := make(map[string]bool)
	result := []string{}

	for _, str := range strings {
		if _, exists := uniqueStrings[str]; !exists {
			uniqueStrings[str] = true
			result = append(result, str)
		}
	}

	return result
}

func getAllRpcIps(url string) ([]string, error) {

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

func measureSpeed(ip string, measureTime int) (float64, error) {

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

	speed := calculateSpeed(resp.Body, measureTime)
	return speed, nil
}

func calculateSpeed(body io.ReadCloser, measureTime int) float64 {
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

func checkAllNodesSpeed() error {
	numCores := runtime.NumCPU()
	// runtime.GOMAXPROCS(numCores)

	res, err := getAllRpcIps(MAINNET_CLUSTER_URL)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return nil
	}
	rpcIps := removeDuplicates(res)
	rpcIpsLen := len(rpcIps)
	fmt.Println(rpcIpsLen, " Nodes Total Founded")
	batchLength := numCores
	// Preparing input and output channels for workers
	ipsChannel := make(chan string, batchLength)
	messages := make(chan string, batchLength)

	// Initializing Goroutine workers
	var wg sync.WaitGroup
	for i := 0; i < batchLength; i++ {
		wg.Add(1)
		// Worker itself
		go func() {
			defer wg.Done()
			// Reading channel until it closed
			for ip := range ipsChannel {
				speed, err := measureSpeed(ip, MEASURE_SECONDS)
				if err != nil {
					messages <- fmt.Sprintf("IP %s: Error\n", ip)
				} else {
					messages <- fmt.Sprintf("IP %s: %.2f MB/s\n", ip, speed)
				}
			}
		}()
	}

	// Input loading goroutine
	go func() {
		defer close(ipsChannel) // close input channel afterwards
		for _, ip := range rpcIps {
			ipsChannel <- ip
		}
	}()

	// Reading from output channel until it closes
	i := 0
	for elem := range messages {
		i++
		fmt.Printf("%d) %s", i, elem)
	}
	wg.Wait()       // Wait until all goroutines are done
	close(messages) // Close output channel
	return nil
}

func main() {
	// Exit code ensuring
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// checkSavedNodesSpeed()
	checkAllNodesSpeed()
	// saveCheckedNodes()
	// downloadSnapshot()
	return nil
}
