package collector

import (
	"log"
	"math"
	"strconv"
)

// round2 rounds a float to 2 decimal places
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// parseUint wrapper replacing `strconv.ParseUint` that logs errors explicitly at debug level
func parseUint(s string, base int, bitSize int, fieldName string) uint64 {
	val, err := strconv.ParseUint(s, base, bitSize)
	if err != nil {
		log.Printf("debug: failed to parse %s (%q): %v", fieldName, s, err)
		return 0
	}
	return val
}

// parseFloat wrapper replacing `strconv.ParseFloat` that logs errors explicitly at debug level
func parseFloat(s string, bitSize int, fieldName string) float64 {
	val, err := strconv.ParseFloat(s, bitSize)
	if err != nil {
		log.Printf("debug: failed to parse %s (%q): %v", fieldName, s, err)
		return 0
	}
	return val
}
