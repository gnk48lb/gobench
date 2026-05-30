package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
	"gobench/internal/model"
)

type FunctionPayload struct {
	BatchSize     int `json:"batch_size"`
	SimulateCount int `json:"simulate_count"`
}

// ExecuteTask routes the execution based on task type
func ExecuteTask(ctx context.Context, task *model.Task) (string, error) {
	if task.TaskType == "function" {
		return simulateRevenueCalc(ctx, task.Payload)
	}
	// Stub for other types like shell or http
	return "", fmt.Errorf("unsupported task_type: %s", task.TaskType)
}

// simulateRevenueCalc simulates high-concurrency calculation of million-user revenue
func simulateRevenueCalc(ctx context.Context, payloadStr string) (string, error) {
	var payload FunctionPayload
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		return "", fmt.Errorf("invalid payload: %v", err)
	}
	if payload.BatchSize <= 0 {
		payload.BatchSize = 1000 // default value
	}
	if payload.SimulateCount <= 0 {
		payload.SimulateCount = 100000 // default value
	}

	batches := payload.SimulateCount / payload.BatchSize
	if payload.SimulateCount%payload.BatchSize != 0 {
		batches++
	}

	var totalRevenue int64 // Atomic accumulator for total revenue in cents
	var wg sync.WaitGroup
	// Concurrency limiter for batches (max 10 concurrent batch processing goroutines)
	concurrencyLimit := make(chan struct{}, 10)

	start := time.Now()

	for i := 0; i < batches; i++ {
		wg.Add(1)
		concurrencyLimit <- struct{}{} // Acquire token
		go func(batchIndex int) {
			defer wg.Done()
			defer func() { <-concurrencyLimit }() // Release token

			// Simulate processing time (10ms - 50ms)
			time.Sleep(time.Duration(rand.Intn(40)+10) * time.Millisecond)

			// Simulate revenue calculation result for this batch
			batchRevenue := int64(payload.BatchSize * rand.Intn(100))
			atomic.AddInt64(&totalRevenue, batchRevenue)
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	output := fmt.Sprintf("Processed %d users in %d batches. Total Revenue: %.2f RMB. Time taken: %s",
		payload.SimulateCount, batches, float64(totalRevenue)/100.0, duration.String())
	return output, nil
}
