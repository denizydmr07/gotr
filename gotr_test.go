package gotr_test

import (
	"testing"
	"time"

	"github.com/denizydmr07/gotr" // Replace with your actual package path
)

// TestNewConfig checks if a new configuration is created with the specified values
func TestNewConfig(t *testing.T) {
	maxHops := 30
	timeout := 2 * time.Second
	delay := 100 * time.Millisecond
	retries := 5

	config := gotr.NewConfig(maxHops, timeout, delay, retries)

	if config.MaxHops != maxHops {
		t.Errorf("Expected MaxHops to be %d, got %d", maxHops, config.MaxHops)
	}

	if config.Timeout != timeout {
		t.Errorf("Expected Timeout to be %v, got %v", timeout, config.Timeout)
	}

	if config.Delay != delay {
		t.Errorf("Expected Delay to be %v, got %v", delay, config.Delay)
	}

	if config.Retries != retries {
		t.Errorf("Expected Retries to be %d, got %d", retries, config.Retries)
	}
}

// TestNewGotr verifies if a new Gotr instance is created with the given address
func TestNewGotr(t *testing.T) {
	address := "www.google.com"
	config := gotr.NewConfig(30, 2*time.Second, 100*time.Millisecond, 5)

	gotr := gotr.NewGotr(address, config)

	if gotr.Addr != address {
		t.Errorf("Expected Address to be %s, got %s", address, gotr.Addr)
	}
}

// TestTrace verifies if the trace function returns the correct hops
// This test needs an admin privilege to run
func TestTrace(t *testing.T) {
	address := "www.google.com"
	config := gotr.NewConfig(30, 2*time.Second, 100*time.Millisecond, 5)

	gotr := gotr.NewGotr(address, config)

	hops := gotr.Trace()

	if hops.Len() == 0 {
		t.Error("Expected at least one hop, got none")
	}
}
