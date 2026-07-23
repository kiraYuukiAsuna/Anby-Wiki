package main

import (
	"encoding/json"
	"math"
	"runtime"
	"sort"
	"time"
)

type metric struct {
	Name       string  `json:"name"`
	Requests   int     `json:"requests"`
	Errors     int     `json:"errors"`
	ErrorRate  float64 `json:"error_rate"`
	Throughput float64 `json:"throughput_per_second"`
	P50MS      float64 `json:"p50_ms"`
	P95MS      float64 `json:"p95_ms"`
	P99MS      float64 `json:"p99_ms"`
	MaxMS      float64 `json:"max_ms"`
}

type explain struct {
	Name string          `json:"name"`
	Plan json.RawMessage `json:"plan"`
}

type report struct {
	GeneratedAt        time.Time      `json:"generated_at"`
	Profile            string         `json:"profile"`
	Database           string         `json:"database"`
	Postgres           string         `json:"postgres_version"`
	SearchBackend      string         `json:"search_backend"`
	SearchIndexSeconds float64        `json:"search_index_duration_seconds"`
	Machine            map[string]any `json:"machine"`
	Pages              int            `json:"pages"`
	Revisions          int            `json:"revisions"`
	SearchDocs         int            `json:"search_documents"`
	Relations          int            `json:"relations"`
	M9                 m9Capacity     `json:"m9_capacity"`
	SeedSeconds        float64        `json:"seed_duration_seconds"`
	DatabaseSize       int64          `json:"database_size_bytes"`
	Metrics            []metric       `json:"metrics"`
	Explains           []explain      `json:"explains"`
}

type m9Capacity struct {
	Components            int64 `json:"components"`
	ComponentDependencies int64 `json:"component_dependencies"`
	Collections           int64 `json:"collections"`
	CollectionMemberships int64 `json:"collection_memberships"`
	ExternalResources     int64 `json:"external_resources"`
	Entities              int64 `json:"entities"`
	DueExternalLinks      int64 `json:"due_external_links"`
	PendingClaimChanges   int64 `json:"pending_claim_changes"`
}

func measure(iterations int, name string, fn func(int) error) metric {
	durations := make([]time.Duration, 0, iterations)
	errorsCount := 0
	started := time.Now()
	for i := 0; i < iterations; i++ {
		requestStarted := time.Now()
		if err := fn(i); err != nil {
			errorsCount++
		}
		durations = append(durations, time.Since(requestStarted))
	}
	elapsed := time.Since(started).Seconds()
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	ms := func(value time.Duration) float64 { return float64(value.Microseconds()) / 1000 }
	return metric{
		Name: name, Requests: iterations, Errors: errorsCount,
		ErrorRate:  float64(errorsCount) / float64(iterations),
		Throughput: float64(iterations) / elapsed,
		P50MS:      ms(percentile(durations, 0.50)),
		P95MS:      ms(percentile(durations, 0.95)),
		P99MS:      ms(percentile(durations, 0.99)),
		MaxMS:      ms(durations[len(durations)-1]),
	}
}

func percentile(values []time.Duration, quantile float64) time.Duration {
	index := int(math.Ceil(float64(len(values))*quantile)) - 1
	if index < 0 {
		index = 0
	}
	return values[index]
}

func machineInfo() map[string]any {
	return map[string]any{
		"os": runtime.GOOS, "arch": runtime.GOARCH,
		"logical_cpus": runtime.NumCPU(), "go_version": runtime.Version(),
	}
}
