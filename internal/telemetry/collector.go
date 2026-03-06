package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type ToolMetric struct {
	Calls               int64   `json:"calls"`
	Errors              int64   `json:"errors"`
	P95LatencyMS        int64   `json:"p95_latency_ms"`
	AverageLatencyMS    int64   `json:"average_latency_ms"`
	ConfidenceSamples   int64   `json:"confidence_samples"`
	AverageConfidence   float64 `json:"average_confidence"`
	LastObservedAt      string  `json:"last_observed_at"`
	latencies           []int64
	totalLatency        int64
	confidenceAggregate float64
}

type Snapshot struct {
	GeneratedAt string                `json:"generated_at"`
	Tools       map[string]ToolMetric `json:"tools"`
}

type Collector struct {
	mu         sync.Mutex
	byTool     map[string]*ToolMetric
	exportPath string
}

func NewCollector(exportPath string) *Collector {
	return &Collector{
		byTool:     make(map[string]*ToolMetric),
		exportPath: exportPath,
	}
}

func (c *Collector) Observe(tool string, duration time.Duration, success bool, confidence *float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	metric, ok := c.byTool[tool]
	if !ok {
		metric = &ToolMetric{}
		c.byTool[tool] = metric
	}

	latencyMS := duration.Milliseconds()
	metric.Calls++
	if !success {
		metric.Errors++
	}
	metric.totalLatency += latencyMS
	metric.latencies = append(metric.latencies, latencyMS)
	if len(metric.latencies) > 2000 {
		metric.latencies = metric.latencies[len(metric.latencies)-2000:]
	}
	if confidence != nil {
		metric.ConfidenceSamples++
		metric.confidenceAggregate += *confidence
		metric.AverageConfidence = metric.confidenceAggregate / float64(metric.ConfidenceSamples)
	}
	metric.LastObservedAt = time.Now().UTC().Format(time.RFC3339)
	metric.AverageLatencyMS = metric.totalLatency / metric.Calls
	metric.P95LatencyMS = percentile95(metric.latencies)

	if c.exportPath != "" {
		_ = c.writeSnapshotLocked()
	}
}

func (c *Collector) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := Snapshot{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Tools:       make(map[string]ToolMetric, len(c.byTool)),
	}
	for name, metric := range c.byTool {
		clone := *metric
		clone.latencies = nil
		clone.totalLatency = 0
		clone.confidenceAggregate = 0
		result.Tools[name] = clone
	}
	return result
}

func (c *Collector) writeSnapshotLocked() error {
	dir := filepath.Dir(c.exportPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create metrics dir: %w", err)
		}
	}
	file, err := os.Create(c.exportPath)
	if err != nil {
		return err
	}
	defer file.Close()

	snapshot := Snapshot{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Tools:       make(map[string]ToolMetric, len(c.byTool)),
	}
	for name, metric := range c.byTool {
		clone := *metric
		clone.latencies = nil
		clone.totalLatency = 0
		clone.confidenceAggregate = 0
		snapshot.Tools[name] = clone
	}

	payload, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	_, err = file.Write(payload)
	return err
}

func percentile95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := make([]int64, len(values))
	copy(copyValues, values)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	idx := int(float64(len(copyValues)-1) * 0.95)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(copyValues) {
		idx = len(copyValues) - 1
	}
	return copyValues[idx]
}
