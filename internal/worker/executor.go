package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gobench/internal/model"
)

// ExecuteTask routes execution to the correct handler based on task_type.
func ExecuteTask(ctx context.Context, task *model.Task) (string, error) {
	switch task.TaskType {
	case "function":
		return executeFunction(ctx, task.Payload)
	case "shell":
		return executeShell(ctx, task.Payload)
	case "http":
		return executeHTTP(ctx, task.Payload)
	default:
		return "", fmt.Errorf("unsupported task_type: %s", task.TaskType)
	}
}

// =============================================================================
// task_type: "function"
// Dispatches to a named sub-function via payload["function"].
// Falls back to the legacy revenue simulation when the field is absent.
// =============================================================================

type functionBase struct {
	Function string `json:"function"`
}

func executeFunction(ctx context.Context, payloadStr string) (string, error) {
	var base functionBase
	// Intentionally ignore the error — a missing "function" key just means legacy behavior.
	_ = json.Unmarshal([]byte(payloadStr), &base)

	switch base.Function {
	case "batch_ping":
		return executeBatchPing(ctx, payloadStr)
	default:
		// Backward-compatible: existing tasks without a "function" field land here.
		return simulateRevenueCalc(ctx, payloadStr)
	}
}

// =============================================================================
// task_type: "shell"
// Runs a single shell command. Uses PowerShell on Windows, bash elsewhere.
//
// Payload example:
//
//	{
//	  "command": "Get-CimInstance Win32_LogicalDisk | Select-Object DeviceID, @{Name='FreeGB';Expression={[math]::round($_.FreeSpace/1GB,2)}} | Format-Table"
//	}
// =============================================================================

type shellPayload struct {
	Command string `json:"command"`
}

func executeShell(ctx context.Context, payloadStr string) (string, error) {
	var p shellPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil {
		return "", fmt.Errorf("invalid shell payload: %w", err)
	}
	if strings.TrimSpace(p.Command) == "" {
		return "", fmt.Errorf("shell command is empty")
	}

	// Use the native shell for the current OS so the server works on both
	// Windows (PowerShell) and Linux/macOS (bash).
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", p.Command)
	default:
		cmd = exec.CommandContext(ctx, "bash", "-c", p.Command)
	}

	// CombinedOutput captures both stdout and stderr, so error details always
	// show up in the task log even when the command fails.
	out, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if err != nil {
		return result, fmt.Errorf("command exited with error: %w", err)
	}
	return result, nil
}

// =============================================================================
// task_type: "http"
// Fires a single HTTP request and records status code, latency, and body.
//
// Payload example:
//
//	{
//	  "url":    "https://v1.hitokoto.cn/",
//	  "method": "GET"
//	}
//
// Optional fields: "headers" (object), "body" (string for POST/PUT payloads).
// The task-level timeout (set in the Task model) already wraps this in a
// context deadline, so no extra timeout is needed here.
// =============================================================================

type httpPayload struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func executeHTTP(ctx context.Context, payloadStr string) (string, error) {
	var p httpPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil {
		return "", fmt.Errorf("invalid http payload: %w", err)
	}
	if p.URL == "" {
		return "", fmt.Errorf("url is required")
	}
	if p.Method == "" {
		p.Method = "GET"
	}

	var bodyReader io.Reader
	if p.Body != "" {
		bodyReader = strings.NewReader(p.Body)
	}

	req, err := http.NewRequestWithContext(ctx, p.Method, p.URL, bodyReader)
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := (&http.Client{}).Do(req)
	latency := time.Since(start)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	body := string(raw)
	if len(body) > 2000 {
		body = body[:2000] + "\n...[truncated]"
	}

	return fmt.Sprintf("Status: %d | Latency: %s\n%s",
		resp.StatusCode, latency.Round(time.Millisecond), body), nil
}

// =============================================================================
// function: "batch_ping"
// Concurrently sends GET requests to every URL in the list and reports
// reachability + latency for each.
//
// Payload example:
//
//	{
//	  "function":        "batch_ping",
//	  "urls":            ["https://github.com", "https://google.com", "https://baidu.com"],
//	  "timeout_seconds": 5
//	}
//
// Each URL gets its own goroutine — total wall-clock time ≈ slowest single
// request, regardless of list length. This is the clearest demo of why
// goroutines exist.
// =============================================================================

type batchPingPayload struct {
	Function       string   `json:"function"`
	URLs           []string `json:"urls"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

type pingResult struct {
	url        string
	ok         bool
	statusCode int
	latencyMs  int64
	errMsg     string
}

func executeBatchPing(ctx context.Context, payloadStr string) (string, error) {
	var p batchPingPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil {
		return "", fmt.Errorf("invalid batch_ping payload: %w", err)
	}
	if len(p.URLs) == 0 {
		return "", fmt.Errorf("urls list must not be empty")
	}
	if p.TimeoutSeconds <= 0 {
		p.TimeoutSeconds = 5
	}

	results := make([]pingResult, len(p.URLs))
	var wg sync.WaitGroup

	// One shared client. Its Timeout caps each individual request; the outer
	// context (from task.Timeout) caps the whole batch.
	client := &http.Client{
		Timeout: time.Duration(p.TimeoutSeconds) * time.Second,
	}

	wallStart := time.Now()

	for i, u := range p.URLs {
		wg.Add(1)
		// Capture loop variables explicitly — classic Go gotcha.
		go func(idx int, target string) {
			defer wg.Done()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
			if err != nil {
				results[idx] = pingResult{url: target, errMsg: err.Error()}
				return
			}
			req.Header.Set("User-Agent", "GoBench-Ping/1.0")

			t0 := time.Now()
			resp, err := client.Do(req)
			lat := time.Since(t0).Milliseconds()

			if err != nil {
				// Trim the error string — it can be very long for timeouts.
				msg := err.Error()
				if len(msg) > 80 {
					msg = msg[:80] + "..."
				}
				results[idx] = pingResult{url: target, latencyMs: lat, errMsg: msg}
				return
			}
			resp.Body.Close() // We only care about headers, not the body.

			results[idx] = pingResult{
				url:        target,
				ok:         resp.StatusCode < 400,
				statusCode: resp.StatusCode,
				latencyMs:  lat,
			}
		}(i, u)
	}

	wg.Wait()
	elapsed := time.Since(wallStart)

	// Build the report string that ends up in the task_log.output column.
	var sb strings.Builder
	ok, fail := 0, 0

	// 🟢 优化 1：改用 fmt.Fprintf 直接写入 Builder
	fmt.Fprintf(&sb, "Batch Ping — %d URLs (per-request timeout: %ds)\n", len(p.URLs), p.TimeoutSeconds)

	// 🟢 优化 2：去掉 '+' 拼接，改用两步写入（特别注意用单引号 '\n' 写入字节）
	sb.WriteString(strings.Repeat("─", 64))
	sb.WriteByte('\n')

	for _, r := range results {
		switch {
		case r.ok:
			ok++
			// 🟢 优化 3：循环内部改用 Fprintf，省下海量临时字符串内存
			fmt.Fprintf(&sb, "  ✓ %3d  %-48s  %dms\n", r.statusCode, r.url, r.latencyMs)
		case r.statusCode > 0:
			fail++
			fmt.Fprintf(&sb, "  ✗ %3d  %-48s  %dms\n", r.statusCode, r.url, r.latencyMs)
		default:
			fail++
			fmt.Fprintf(&sb, "  ✗ ERR  %-48s  %s\n", r.url, r.errMsg)
		}
	}

	// 🟢 优化 4：同理清理尾部
	sb.WriteString(strings.Repeat("─", 64))
	sb.WriteByte('\n')

	fmt.Fprintf(&sb, "  Result: %d/%d reachable  |  Wall clock: %s\n",
		ok, len(p.URLs), elapsed.Round(time.Millisecond))

	return sb.String(), nil
}

// =============================================================================
// function: legacy "simulate_revenue_calc"
// Kept for backward compatibility with tasks already in the database.
// Simulates batched concurrent processing with artificial sleep to show
// goroutine fan-out behaviour.
// =============================================================================

// FunctionPayload is exported so existing code that references it keeps compiling.
type FunctionPayload struct {
	BatchSize     int `json:"batch_size"`
	SimulateCount int `json:"simulate_count"`
}

func simulateRevenueCalc(ctx context.Context, payloadStr string) (string, error) {
	var payload FunctionPayload
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		return "", fmt.Errorf("invalid payload: %v", err)
	}
	if payload.BatchSize <= 0 {
		payload.BatchSize = 1000
	}
	if payload.SimulateCount <= 0 {
		payload.SimulateCount = 100000
	}

	batches := payload.SimulateCount / payload.BatchSize
	if payload.SimulateCount%payload.BatchSize != 0 {
		batches++
	}

	var totalRevenue int64
	var wg sync.WaitGroup
	concurrencyLimit := make(chan struct{}, 10)
	start := time.Now()

	for i := 0; i < batches; i++ {
		select {
		case <-ctx.Done():
			wg.Wait()
			return "", ctx.Err()
		default:
		}

		wg.Add(1)
		concurrencyLimit <- struct{}{}
		go func(batchIndex int) {
			defer wg.Done()
			defer func() { <-concurrencyLimit }()

			sleepTime := time.Duration(rand.Intn(40)+10) * time.Millisecond
			select {
			case <-ctx.Done():
				return
			case <-time.After(sleepTime):
			}

			batchRevenue := int64(payload.BatchSize * rand.Intn(100))
			atomic.AddInt64(&totalRevenue, batchRevenue)
		}(i)
	}

	wg.Wait()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	return fmt.Sprintf("Processed %d users in %d batches. Total Revenue: %.2f RMB. Time taken: %s",
		payload.SimulateCount, batches, float64(totalRevenue)/100.0, time.Since(start).String()), nil
}
