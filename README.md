# Gotr: Traceroute Package for Go

## Overview
`Gotr` is a Go package for performing traceroute operations, allowing you to track the path packets take across a network and identify routers and the time it takes to reach each hop. This package leverages goroutines to execute traceroute operations concurrently, leading to faster results.

## Features
- Concurrent execution for efficient traceroute operations.
- Customizable configurations, allowing you to set the maximum number of hops, timeout, delay, and retries.
- Retrieve information about network hops, including address, round-trip time (RTT), and Time To Live (TTL).

## Usage
To use `Gotr` with the default configuration, create a `Gotr` instance with a target address and call the `Trace` method. This performs each hop concurrently, providing faster traceroute results. It returns a list of hops with their round-trip times and other relevant information.

``` go
package main

import (
    "fmt"
    "github.com/denizydmr07/gotr" 
)

func main() {
    // Create an instance
    traceroute := gotr.NewGotr("example.com")
    
    // Perform traceroute
    results := traceroute.Trace()
    
    // Display the results
    for _, result in range results {
        fmt.Printf("Hop %d: Address = %v, RTT = %.2f ms\n", result.TTL, result.Addr, result.RTT)
    }
}
```

You can create a custom configuration for `Gotr` by specifying the maximum number of hops, timeout duration, delay between attempts, and the number of retries.

``` go
package main

import (
    "fmt"
    "time"
    "github.com/denizydmr07/gotr"
)

func main() {
    // Create a custom configuration
    customConfig := gotr.NewConfig(20, 2*time.Second, 100*time.Millisecond, 3)
    
    // Create an instance
    traceroute := gotr.NewGotr("example.com", customConfig)
    
    // Perform traceroute
    results := traceroute.Trace()
    
    // Display the results
    for _, result in range results,
        fmt.Printf("Hop %d: Address = %v, RTT = %.2f ms\n", result.TTL, result.Addr, result.RTT)
    }
}
```

## Administrative Privileges

This package uses ICMP and raw sockets, which may require administrative privileges. Ensure your Go environment and system permissions allow the use of raw sockets and ICMP traffic. If you're running into permission-related issues, try running your program with administrative or superuser rights.


