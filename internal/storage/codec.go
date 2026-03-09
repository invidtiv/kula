package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"kula-szpiegula/internal/collector"
	"time"
)

// AggregatedSample holds a time-aggregated metric sample.
// For tier 1 (1s), this is just a wrapper around the raw sample.
// For higher tiers, Data holds averaged values and Min/Max fields hold
// the minimum and maximum observed values over the aggregation window.
type AggregatedSample struct {
	Timestamp time.Time         `json:"ts"`
	Duration  time.Duration     `json:"dur"`
	Data      *collector.Sample `json:"data"`
	Min       *collector.Sample `json:"min,omitempty"`
	Max       *collector.Sample `json:"max,omitempty"`
}

func encodeSample(s *AggregatedSample) ([]byte, error) {
	return json.Marshal(s)
}

func decodeSample(data []byte) (*AggregatedSample, error) {
	s := &AggregatedSample{}
	err := json.Unmarshal(data, s)
	return s, err
}

// extractTimestamp finds and parses the "ts" field from JSON data without full decoding.
func extractTimestamp(data []byte) (time.Time, error) {
	idx := bytes.Index(data, []byte(`"ts":"`))
	if idx == -1 {
		return time.Time{}, fmt.Errorf("ts not found")
	}
	start := idx + 6
	end := bytes.IndexByte(data[start:], '"')
	if end == -1 {
		return time.Time{}, fmt.Errorf("malformed timestamp")
	}
	end += start

	return time.Parse(time.RFC3339Nano, string(data[start:end]))
}
