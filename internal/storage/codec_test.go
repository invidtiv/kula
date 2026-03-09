package storage

import (
	"kula-szpiegula/internal/collector"
	"testing"
	"time"
)

// ---- helpers ----------------------------------------------------------------

func makeSampleFull(ts time.Time) *AggregatedSample {
	return &AggregatedSample{
		Timestamp: ts,
		Duration:  time.Second,
		Data: &collector.Sample{
			Timestamp: ts,
			CPU: collector.CPUStats{
				Total: collector.CPUCoreStats{
					User:   25.5,
					System: 10.2,
					Usage:  35.7,
				},
				NumCores: 8,
			},
			LoadAvg: collector.LoadAvg{
				Load1:  1.5,
				Load5:  1.2,
				Load15: 0.8,
			},
			Memory: collector.MemoryStats{
				Total:       16 * 1024 * 1024 * 1024,
				Used:        8 * 1024 * 1024 * 1024,
				Free:        4 * 1024 * 1024 * 1024,
				Shmem:       512 * 1024 * 1024,
				UsedPercent: 50.0,
			},
			Network: collector.NetworkStats{
				Interfaces: []collector.NetInterface{
					{Name: "eth0", RxMbps: 1.5, TxMbps: 0.3},
				},
				TCP:     collector.TCPStats{CurrEstab: 42, InErrs: 0.1, OutRsts: 0.5},
				Sockets: collector.SocketStats{TCPInUse: 42, UDPInUse: 5, TCPTw: 3},
			},
			System: collector.SystemStats{
				Hostname:    "test-host",
				Entropy:     256,
				ClockSource: "tsc",
				ClockSync:   true,
				UserCount:   2,
			},
		},
	}
}

// ---- TestEncodeDecode -------------------------------------------------------

func TestEncodeDecode(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	original := makeSampleFull(now)

	encoded, err := encodeSample(original)
	if err != nil {
		t.Fatalf("encodeSample() error: %v", err)
	}
	if len(encoded) == 0 {
		t.Fatal("encodeSample() returned empty data")
	}

	decoded, err := decodeSample(encoded)
	if err != nil {
		t.Fatalf("decodeSample() error: %v", err)
	}

	if !decoded.Timestamp.Equal(original.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", decoded.Timestamp, original.Timestamp)
	}
	if decoded.Duration != original.Duration {
		t.Errorf("Duration = %v, want %v", decoded.Duration, original.Duration)
	}
	if decoded.Data == nil {
		t.Fatal("Decoded Data is nil")
	}
	if decoded.Data.CPU.Total.Usage != original.Data.CPU.Total.Usage {
		t.Errorf("CPU Usage = %f, want %f", decoded.Data.CPU.Total.Usage, original.Data.CPU.Total.Usage)
	}
	if decoded.Data.CPU.NumCores != original.Data.CPU.NumCores {
		t.Errorf("NumCores = %d, want %d", decoded.Data.CPU.NumCores, original.Data.CPU.NumCores)
	}
	if decoded.Data.System.Hostname != "test-host" {
		t.Errorf("Hostname = %q, want \"test-host\"", decoded.Data.System.Hostname)
	}
	if decoded.Data.Memory.Total != original.Data.Memory.Total {
		t.Errorf("Memory Total = %d, want %d", decoded.Data.Memory.Total, original.Data.Memory.Total)
	}
	if decoded.Data.Memory.Shmem != original.Data.Memory.Shmem {
		t.Errorf("Memory Shmem = %d, want %d", decoded.Data.Memory.Shmem, original.Data.Memory.Shmem)
	}
	// Network TCP stats survive round-trip
	if decoded.Data.Network.TCP.CurrEstab != original.Data.Network.TCP.CurrEstab {
		t.Errorf("TCP.CurrEstab = %d, want %d",
			decoded.Data.Network.TCP.CurrEstab,
			original.Data.Network.TCP.CurrEstab)
	}
}

func TestEncodeDecodeRoundTripTimestamp(t *testing.T) {
	// Verify sub-millisecond precision is preserved.
	ts := time.Date(2026, 3, 4, 12, 30, 0, 123456789, time.UTC)
	s := &AggregatedSample{
		Timestamp: ts,
		Duration:  time.Second,
		Data:      &collector.Sample{Timestamp: ts},
	}
	enc, err := encodeSample(s)
	if err != nil {
		t.Fatalf("encodeSample: %v", err)
	}
	dec, err := decodeSample(enc)
	if err != nil {
		t.Fatalf("decodeSample: %v", err)
	}
	if !dec.Timestamp.Equal(ts) {
		t.Errorf("Timestamp mismatch: got %v, want %v", dec.Timestamp, ts)
	}
}

// ---- TestDecodeInvalid ------------------------------------------------------

func TestDecodeInvalid(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
	}{
		{"not-json", []byte("not json")},
		{"empty", []byte{}},
		{"truncated-json", []byte(`{"ts":"2026`)},
		{"wrong-type", []byte(`{"ts":12345,"dur":1000000000}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeSample(tc.input)
			if err == nil {
				t.Errorf("decodeSample(%q) expected error, got nil", tc.input)
			}
		})
	}
}

// ---- TestEncodeNilData ------------------------------------------------------

func TestEncodeNilData(t *testing.T) {
	s := &AggregatedSample{
		Timestamp: time.Now(),
		Duration:  time.Second,
		Data:      nil,
	}
	encoded, err := encodeSample(s)
	if err != nil {
		t.Fatalf("encodeSample() with nil Data: %v", err)
	}

	decoded, err := decodeSample(encoded)
	if err != nil {
		t.Fatalf("decodeSample() error: %v", err)
	}
	if decoded.Data != nil {
		t.Error("Decoded Data should be nil")
	}
}

// ---- extractTimestamp -------------------------------------------------------

func TestExtractTimestamp_HappyPath(t *testing.T) {
	ts := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	s := &AggregatedSample{
		Timestamp: ts,
		Duration:  time.Second,
		Data:      &collector.Sample{Timestamp: ts},
	}
	data, err := encodeSample(s)
	if err != nil {
		t.Fatalf("encodeSample: %v", err)
	}

	got, err := extractTimestamp(data)
	if err != nil {
		t.Fatalf("extractTimestamp() error: %v", err)
	}
	if !got.Equal(ts) {
		t.Errorf("extractTimestamp() = %v, want %v", got, ts)
	}
}

func TestExtractTimestamp_Missing(t *testing.T) {
	_, err := extractTimestamp([]byte(`{"dur":1000000000}`))
	if err == nil {
		t.Error("extractTimestamp() with no 'ts' field should return error")
	}
}

func TestExtractTimestamp_Malformed(t *testing.T) {
	_, err := extractTimestamp([]byte(`{"ts":"not-a-time"}`))
	if err == nil {
		t.Error("extractTimestamp() with malformed timestamp should return error")
	}
}

func TestExtractTimestamp_Unterminated(t *testing.T) {
	_, err := extractTimestamp([]byte(`{"ts":"2026-03-04T00:00:00Z`))
	if err == nil {
		t.Error("extractTimestamp() with unterminated string should return error")
	}
}

func TestExtractTimestamp_MatchesFullDecode(t *testing.T) {
	// Fast-path extracted timestamp must exactly match the full JSON decode.
	now := time.Now().Truncate(time.Millisecond)
	s := &AggregatedSample{
		Timestamp: now,
		Duration:  time.Second,
		Data:      &collector.Sample{Timestamp: now},
	}
	data, _ := encodeSample(s)

	fast, err := extractTimestamp(data)
	if err != nil {
		t.Fatalf("extractTimestamp: %v", err)
	}
	full, err := decodeSample(data)
	if err != nil {
		t.Fatalf("decodeSample: %v", err)
	}
	if !fast.Equal(full.Timestamp) {
		t.Errorf("extractTimestamp %v != decodeSample %v", fast, full.Timestamp)
	}
}

// ---- Benchmarks -------------------------------------------------------------

func BenchmarkEncodeSample(b *testing.B) {
	s := makeSampleFull(time.Now())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = encodeSample(s)
	}
}

func BenchmarkDecodeSample(b *testing.B) {
	s := makeSampleFull(time.Now())
	data, _ := encodeSample(s)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = decodeSample(data)
	}
}

func BenchmarkExtractTimestamp(b *testing.B) {
	s := makeSampleFull(time.Now())
	data, _ := encodeSample(s)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = extractTimestamp(data)
	}
}

// BenchmarkExtractVsFullDecode shows the speedup of the fast path over full JSON decode.
func BenchmarkExtractVsFullDecode(b *testing.B) {
	s := makeSampleFull(time.Now())
	data, _ := encodeSample(s)

	b.Run("ExtractTimestamp", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = extractTimestamp(data)
		}
	})
	b.Run("FullDecode", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = decodeSample(data)
		}
	})
}
