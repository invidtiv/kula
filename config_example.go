package kula

import _ "embed"

// ExampleConfig is the packaged config.example.yaml, embedded at build time so
// the binary can seed a missing config file instead of refusing to start.
//
//go:embed config.example.yaml
var ExampleConfig []byte
