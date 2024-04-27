// Package gotr provides functionality to perform traceroute operations.
// Traceroute is a diagnostic tool used to track the path packets take across a network,
// identifying the routers and the time it takes to reach each hop.
//
// The package allows users to create a traceroute instance with customizable configurations,
// such as maximum hops, timeout, delay, and retries.
//
// To use this package, create a `Gotr` instance with a target address and, optionally, a configuration.
// The `Trace` method can then be used to perform the traceroute, returning a list of hops and
// their round-trip times.
//
// This package requires network access and might require administrative privileges to send
// ICMP packets. Ensure your environment allows ICMP traffic for proper functionality.
package gotr

import (
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// hop represents a network hop in the traceroute process.
// It stores the time of sending, elapsed time, TTL (Time to Live),
// the remote address that replied, and other related information.
type hop struct {
	SendTime    int64      // UnixNano time when the hop was sent
	ElapsedTime float64    // Elapsed time in milliseconds
	TTL         int        // Time to Live for this hop
	RAddr       net.Addr   // Address that replied to the request
	Mutex       sync.Mutex // Mutex for synchronization
	NumTries    int        // Number of tries for this hop
	IsEchoReply bool       // Indicates if the response is an echo reply
}

// config struct to store the configuration of the traceroute process.
type config struct {
	MaxHops int           // maximum number of hops
	Timeout time.Duration // timeout in for each connection, in seconds
	Delay   time.Duration // delay between each request, in milliseconds
	Retries int           // number of retries for each hop that receives no response
}

// RouteHop represents information gathered from a single hop during a network traceroute operation.
// It encapsulates details about the responding network address, the round-trip time (RTT) in milliseconds,
// and the Time To Live (TTL) value for the corresponding hop in the route.
//
// This struct is commonly utilized to convey results obtained during the execution of traceroute procedures
// and is returned as part of the response by the `Trace` method within the `Gotr` struct.
type RouteHop struct {
	Addr net.Addr // The network address that responded to the ICMP request
	RTT  float64  // The round-trip time measured in milliseconds
	TTL  int      // The Time To Live value representing the hop's position within the route
}

// Route represents a sequence of network hops obtained from a traceroute operation.
// It comprises a collection of `RouteHop` instances, each representing a distinct hop along the route.
//
// If a hop fails to respond after the configured number of retries, its address will be nil,
// indicating that the hop was unreached even after the specified number of attempts.
type Route struct {
	Hops []*RouteHop // A list of `RouteHop` instances representing individual hops in the route
}

// Len returns the number of hops in the route.
//
// This method provides the length of the route, indicating the total number
// of hops traversed during the traceroute operation. It calculates and returns
// the count of `RouteHop` instances stored within the `Hops` slice of the `Route` struct.
//
// Returns:
// - The number of hops in the route as an integer.
func (r Route) Len() int {
	return len(r.Hops)
}

// newDefaultConfig creates a new default configuration for the traceroute process.
// It sets MaxHops to 30, Timeout to 1 second, Delay to 50 milliseconds, and Retries to 3.
// Returns a pointer to the config struct.
func newDefaultConfig() *config {
	return &config{
		MaxHops: 30,
		Timeout: 1 * time.Second,
		Delay:   50 * time.Millisecond,
		Retries: 3,
	}
}

// NewConfig creates a new configuration for the traceroute process.
// Takes the following parameters:
// - maxHops: The maximum number of hops the traceroute will attempt.
// - timeout: The timeout duration for each hop in seconds.
// - delay: The delay between each traceroute attempt in milliseconds.
// - retries: The number of retries for each hop that receives no response.
// Returns a pointer to the `config` struct with the specified values.
func NewConfig(maxHops int, timeout time.Duration, delay time.Duration, retries int) *config {
	return &config{
		MaxHops: maxHops,
		Timeout: timeout,
		Delay:   delay,
		Retries: retries,
	}
}

// Gotr represents the traceroute process.
// It contains the following fields:
// - Config: The configuration for the traceroute process, containing maxHops, timeout, delay, and retries.
// - Hops: A slice of `Hop` structs representing each hop in the traceroute.
// - Mutex: A mutex for synchronizing access to the `Hops` slice.
// - Addr: The target address for the traceroute.
//
// The `Gotr` struct is used to perform traceroutes to a specified address with the given configuration.
// To create a new `Gotr` instance, use the `NewGotr` function with the desired address and optional configuration parameters.
type Gotr struct {
	c    *config    // Configuration for the traceroute process
	hops []*hop     // Slice of hops encountered during the traceroute
	mut  sync.Mutex // Mutex for synchronization
	Addr string     // Target address for the traceroute
}

// NewGotr creates a new instance of the `Gotr` struct for the traceroute process.
// It takes the following parameters:
// - addr: The target address for the traceroute (e.g., a domain name or IP address).
// - opts: An optional configuration (`config`) for customizing the traceroute process, such as maximum hops, timeout, delay, and retries.
//
// If no configuration is provided in `opts`, the function uses a default configuration with:
// - MaxHops: 30
// - Timeout: 1 second
// - Delay: 50 milliseconds
// - Retries: 3
//
// The function returns a pointer to the `Gotr` struct with the specified configuration and target address.
func NewGotr(addr string, opts ...*config) *Gotr {
	var c *config // config

	// if no config is provided, use the default config
	if len(opts) == 0 {
		c = newDefaultConfig()
	} else { // use the provided config
		c = opts[0]
	}

	// create a new Gotr
	gotr := &Gotr{
		c:    c,
		Addr: addr,
	}
	// initialize the hops
	for i := 0; i < c.MaxHops; i++ {
		// append a new hop to the hops slice
		gotr.hops = append(gotr.hops, &hop{
			SendTime:    0,
			ElapsedTime: 0,
			TTL:         0,
			RAddr:       nil,
			NumTries:    0,
			IsEchoReply: false,
		})
	}

	// return the Gotr
	return gotr
}

// sendICMPEchoRequest sends an ICMP Echo Request to the specified target address.
// This function takes the following parameters:
// - conn: The ICMP connection used to send the request.
// - ttl: The Time To Live value for the packet.
// - addr: The target address for the echo request.
//
// It does not return any values but can raise an error if sending the request fails.
// This function is typically used in a traceroute process to measure network hops.
func (g *Gotr) sendICMPEchoRequest(ipAddr *net.IPAddr, conn *icmp.PacketConn, ttl int, h *hop) error {
	// lock the mutex of the hop first
	h.Mutex.Lock()

	// create a new ICMP message
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho, // 8, Echo Request
		Code: 0,
		Body: &icmp.Echo{
			ID:   ttl, // set the ID of the message to the TTL value, so we can identify the response
			Seq:  1,
			Data: []byte(""),
		},
	}

	// create a new byte slice to store the message
	b, err := msg.Marshal(nil)
	if err != nil {
		return err
	}

	// set the TTL value of the message
	if err := conn.IPv4PacketConn().SetTTL(ttl); err != nil {
		return err
	}

	// get the current time in UnixNano
	sendTime := int64(time.Now().UnixNano())
	// write the message to the connection
	if _, err := conn.WriteTo(b, ipAddr); err != nil {
		return err
	}

	// set the send time of the hop
	h.SendTime = sendTime

	// unlock the mutex of the hop
	h.Mutex.Unlock()

	return nil
}

// receiveICMPEchoReply listens for ICMP Echo Replies from a specified connection.
// Takes the following parameters:
// - conn: The ICMP connection to listen to for incoming echo replies.
// - timeout: The duration to wait for a response before timing out.
//
// Returns a `net.Addr` indicating the address of the sender and a duration representing the round-trip time.
// If an error occurs during reception, it will be returned.
//
// This function is used to gather information about the network hops in a traceroute process.
func (g *Gotr) receiveICMPEchoReply(conn *icmp.PacketConn, rb []byte) (net.Addr, int64, error) {
	// set the read deadline of the connection, to avoid blocking forever
	(*conn).SetReadDeadline(time.Now().Add(g.c.Timeout))

	// read the response
	n, rAddr, err := conn.ReadFrom(rb)
	if err != nil {
		return nil, 0, err
	}

	// we will recieve either 36 bytes (ICMP Time Exceeded) or 8 bytes (ICMP Echo Reply)
	// if the response is not 36 or 8 bytes, relisten
	for n != 36 && n != 8 {
		n, rAddr, err = conn.ReadFrom(rb)
		if err != nil {
			return nil, 0, err
		}
	}

	// get the current time in UnixNano
	receiveTime := int64(time.Now().UnixNano())

	// return the address of the response, the receive time, and nil error
	return rAddr, receiveTime, nil
}

// performHop executes a single hop in the traceroute process.
// It takes the following parameters:
// - conn: The ICMP connection to send and receive ICMP packets.
// - ttl: The Time To Live value for the hop.
// - addr: The target address for the traceroute.
// - retries: The number of retries if the first attempt fails.
//
// The function sends an ICMP Echo Request and waits for an Echo Reply. It returns a `Hop` struct containing
// information about the hop, such as the time sent, elapsed time, TTL, and the replying address.
//
// This function can raise an error if the operation fails or if no response is received after retries.
func (g *Gotr) performHop(ipAddr *net.IPAddr, conn *icmp.PacketConn, ttl int, wg *sync.WaitGroup) {
	defer func() {
		// get the h with the corresponding TTL value
		h := g.hops[ttl-1]
		// lock the mutex of the hop
		h.Mutex.Lock()
		// check if the hop with the corresponding TTL value is not recived
		if h.RAddr == nil && h.NumTries < g.c.Retries {
			// increment the number of tries
			h.NumTries++
			// unlock the mutex of the hop
			h.Mutex.Unlock()
			// perform the hop again
			g.performHop(ipAddr, conn, ttl, wg)
		} else { // if the hop is received or the number of tries is reached
			// unlock the mutex of the hop
			h.Mutex.Unlock()
			// decrement the wait group counter
			wg.Done()
		}
	}()
	// lock the mutex of the Gotr
	g.mut.Lock()

	// get the h with the given TTL value, minus 1 because the TTL value starts from 1
	h := g.hops[ttl-1]

	// set the TTL value of the hop
	h.TTL = ttl

	// unlock the mutex of the Gotr
	g.mut.Unlock()

	// send the ICMP Echo Request
	if err := g.sendICMPEchoRequest(ipAddr, conn, ttl, h); err != nil {
		return
	}

	// create a new byte slice to store the response
	rb := make([]byte, 1500)

	// read the response from the connection
	rAddr, recvTime, err := g.receiveICMPEchoReply(conn, rb)
	if err != nil {
		return
	}

	// ICMP time exceeded is 36 bytes, 0..35
	// ICMP time exceeded header is 8 bytes, 0..7
	// IP header is 20 bytes, 8..27 (in the data of the ICMP time exceeded)
	// ICMP echo reequest header is 8 bytes, 28..35 (in the data of the ICMP time exceeded)
	// ID of the ICMP time exceeded is the initial ttl value of the echo request
	// ID is 2 bytes, 32..33 (in the data of the ICMP time exceeded)
	// ICMP echo reply is 8 bytes
	// ID is 2 bytes, 4..5 (in the header of the ICMP time exceeded)

	// variable to store the TTL value of the response
	var recvTtl int

	// getting the recvType from the response, one byte
	recvType := int(rb[0])

	// gettting the TTL value from the response, two bytes big-endian to integer
	if recvType == 11 { // ICMP Time Exceeded
		recvTtl = int(rb[32])<<8 | int(rb[33])
	} else if recvType == 0 {
		recvTtl = int(rb[4])<<8 | int(rb[5])
	}

	// get the hop with the corresponding TTL value
	h = g.hops[recvTtl-1]

	// calculate the elapsed time
	elapsedTime := float64(recvTime-h.SendTime) / 1000000.0 // in milliseconds

	// lock the mutex of the hop
	h.Mutex.Lock()

	// set the elapsed time of the hop
	h.ElapsedTime = elapsedTime

	// set the address of the response
	h.RAddr = rAddr

	// set the isEchoReply value of the hop
	h.IsEchoReply = recvType == 0

	// unlock the mutex of the hop
	h.Mutex.Unlock()
}

// Trace initiates the traceroute process to the specified target address.
// It utilizes the configuration set in the `Gotr` instance to determine the number of hops, timeout, delay, and retries.
//
// This function concurrently sends ICMP Echo Requests with increasing TTL values and listens for responses,
// recording the elapsed time and address of each hop. It constructs and returns a `Route` struct
// containing information about each hop in ascending order of TTL. Each `RouteHop` struct
// includes the address of the responding hop, the round-trip time (RTT), and the TTL value.
//
// If a hop fails to respond after the configured number of retries, its address will be nil,
// indicating that the hop was unreached even after the specified number of attempts.
//
// The function returns an error if the traceroute process encounters any problems.
// The function returns an empty `Route` struct if the traceroute process fails to complete.
func (g *Gotr) Trace() Route {
	// resolve the IP address of the destination
	ipAddr, err := net.ResolveIPAddr("ip", g.Addr)
	if err != nil {
		fmt.Println("Error resolving IP address: ", err)
		return Route{}
	}

	// create a new ICMP connection
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		fmt.Println("Error creating ICMP connection: ", err)
		return Route{}
	}

	// close the connection when the function returns
	defer conn.Close()

	// create a new wait group
	var wg sync.WaitGroup

	// perform the hops concurrently
	for ttl := 1; ttl <= g.c.MaxHops; ttl++ {
		// increment the wait group counter
		wg.Add(1)
		// perform the hop
		go g.performHop(ipAddr, conn, ttl, &wg)
		// delay between each hop
		time.Sleep(g.c.Delay)
	}

	// wait for all the hops to finish
	wg.Wait()

	// create a new Route
	route := Route{
		Hops: make([]*RouteHop, 0),
	}

	// iterate over the hops until maxHops or until a hop is Echo Reply
	for _, h := range g.hops {
		// create a new response
		resp := RouteHop{
			Addr: h.RAddr,
			RTT:  h.ElapsedTime,
			TTL:  h.TTL,
		}
		// append the response to the result
		route.Hops = append(route.Hops, &resp)

		if h.IsEchoReply {
			break
		}
	}

	// return the route
	return route
}
