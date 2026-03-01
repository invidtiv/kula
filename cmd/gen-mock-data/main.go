package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"kula-szpiegula/internal/collector"
	"kula-szpiegula/internal/config"
	"kula-szpiegula/internal/storage"
)

func main() {
	days := flag.Int("days", 7, "number of days of generated data to simulate (1s resolution)")
	cfgPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("WARNING: This will generate %d days of mock data into '%s'.\n", *days, cfg.Storage.Directory)
	fmt.Printf("This may overwrite or mix with your existing data.\n")
	fmt.Print("Are you sure you want to proceed? (y/N): ")

	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("Aborted by user.")
		os.Exit(0)
	}

	fmt.Printf("Initializing storage at %s\n", cfg.Storage.Directory)
	store, err := storage.NewStore(cfg.Storage)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	totalSamples := *days * 24 * 60 * 60
	fmt.Printf("Generating %d samples (%d days of 1s resolution)...\n", totalSamples, *days)

	// We'll generate data ending at "now"
	now := time.Now()
	startTime := now.Add(-time.Duration(totalSamples) * time.Second)

	// To make the data look somewhat realistic, we can use a basic random walk or sine wave
	var cpuUsage float64 = 5.0
	var memUsed uint64 = 500 * 1024 * 1024 // 500MB
	memTotal := uint64(8 * 1024 * 1024 * 1024)

	rand.Seed(time.Now().UnixNano())

	startGenTime := time.Now()

	for i := 0; i < totalSamples; i++ {
		ts := startTime.Add(time.Duration(i) * time.Second)

		// Jitter
		cpuUsage += rand.Float64()*4 - 2 // [-2, 2]
		if cpuUsage < 0 {
			cpuUsage = 0
		}
		if cpuUsage > 100 {
			cpuUsage = 100
		}

		memUsed = uint64(float64(memUsed) + (rand.Float64()*10-5)*1024*1024)
		if memUsed < 100*1024*1024 {
			memUsed = 100 * 1024 * 1024
		}
		if memUsed > memTotal {
			memUsed = memTotal
		}

		sample := &collector.Sample{
			Timestamp: ts,
			CPU: collector.CPUStats{
				Total: collector.CPUCoreStats{
					ID:    "all",
					Usage: cpuUsage,
				},
			},
			Memory: collector.MemoryStats{
				Total: memTotal,
				Used:  memUsed,
				Free:  memTotal - memUsed,
			},
			LoadAvg: collector.LoadAvg{
				Load1:   cpuUsage / 20.0,
				Load5:   cpuUsage / 25.0,
				Load15:  cpuUsage / 30.0,
				Running: 1,
				Total:   100,
			},
		}

		if err := store.WriteSample(sample); err != nil {
			log.Fatalf("Failed writing sample at index %d: %v", i, err)
		}

		if i > 0 && i%100000 == 0 {
			fmt.Printf("Generated %d / %d samples (%.1f%%)...\n", i, totalSamples, float64(i)/float64(totalSamples)*100)
		}
	}

	elapsed := time.Since(startGenTime)
	fmt.Printf("Finished generating %d samples in %v (%.0f samples/sec).\n", totalSamples, elapsed, float64(totalSamples)/elapsed.Seconds())
	fmt.Println("You can now start kula to test the performance boundaries!")
}
