package gotrmock

import (
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// Hop struct to store the information of a hop
type Hop struct {
	SendTime    int64      // UnixNano
	ElapsedTime float64    // in milliseconds
	TTL         int        // Time to Live
	RAddr       net.Addr   // address that replied to the request
	Mutex       sync.Mutex // to synchronize access to the struct, first sendTime, then elapsedTime
	NumTries    int        // number of tries
	IsEchoReply bool       // is the response an echo reply
}

// config struct to store the configuration of the traceroute process
type config struct {
	MaxHops int           // maximum number of hops
	Timeout time.Duration // timeout in for each connection, in seconds
	Delay   time.Duration // delay between each request, in milliseconds
	Retries int           // number of retries for each hop that receives no response
}

// newDefaultConfig returns a new config with default values
func newDefaultConfig() *config {
	return &config{
		MaxHops: 30,
		Timeout: 1 * time.Second,
		Delay:   50 * time.Millisecond,
		Retries: 3,
	}
}

// NewConfig returns a new config with the given values
func NewConfig(maxHops int, timeout time.Duration, delay time.Duration, retries int) *config {
	return &config{
		MaxHops: maxHops,
		Timeout: timeout,
		Delay:   delay,
		Retries: retries,
	}
}

type Gotr struct {
	// config
	Config *config
	Hops   []*Hop
	Mutex  sync.Mutex
	Addr   string
}

func NewGotr(addr string, opts ...*config) *Gotr {
	var c *config
	if len(opts) == 0 {
		c = newDefaultConfig()
	} else {
		c = opts[0]
	}
	gotr := &Gotr{
		Config: c,
		Addr:   addr,
	}
	// initialize the hops
	for i := 0; i < c.MaxHops; i++ {
		gotr.Hops = append(gotr.Hops, &Hop{
			SendTime:    0,
			ElapsedTime: 0,
			TTL:         0,
			RAddr:       nil,
			NumTries:    0,
			IsEchoReply: false,
		})
	}
	return gotr
}

// sendICMPEchoRequest sends an ICMP Echo Request to the given IP address with the given TTL value
func (g *Gotr) sendICMPEchoRequest(ipAddr *net.IPAddr, conn *icmp.PacketConn, ttl int, hop *Hop) error {
	// lock the mutex of the hop first
	hop.Mutex.Lock()

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
	hop.SendTime = sendTime

	// unlock the mutex of the hop
	hop.Mutex.Unlock()

	return nil
}

// receiveICMPEchoReply receives an ICMP Echo Reply from the connection
func (g *Gotr) receiveICMPEchoReply(conn *icmp.PacketConn, rb []byte) (net.Addr, int64, error) {
	// set the read deadline of the connection
	(*conn).SetReadDeadline(time.Now().Add(g.Config.Timeout))

	// read the response
	_, rAddr, err := conn.ReadFrom(rb)
	if err != nil {
		return nil, 0, err
	}

	// get the current time in UnixNano
	receiveTime := int64(time.Now().UnixNano())

	return rAddr, receiveTime, nil
}

// PerformHop performs a hop with the given TTL value
func (g *Gotr) performHop(ipAddr *net.IPAddr, conn *icmp.PacketConn, ttl int, wg *sync.WaitGroup) {
	defer func() {
		// get the hop with the corresponding TTL value
		hop := g.Hops[ttl-1]
		// lock the mutex of the hop
		hop.Mutex.Lock()
		// check if the hop with the corresponding TTL value is not recived
		if hop.RAddr == nil && hop.NumTries < g.Config.Retries {
			// increment the number of tries
			hop.NumTries++
			// unlock the mutex of the hop
			hop.Mutex.Unlock()
			// perform the hop again
			g.performHop(ipAddr, conn, ttl, wg)
		} else { // if the hop is received or the number of tries is reached
			// unlock the mutex of the hop
			hop.Mutex.Unlock()
			// decrement the wait group counter
			wg.Done()
		}
	}()
	g.Mutex.Lock()

	// get the hop with the given TTL value, minus 1 because the TTL value starts from 1
	hop := g.Hops[ttl-1]

	// set the TTL value of the hop
	hop.TTL = ttl

	// unlock the mutex of the Gotr
	g.Mutex.Unlock()

	// send the ICMP Echo Request
	if err := g.sendICMPEchoRequest(ipAddr, conn, ttl, hop); err != nil {
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
	hop = g.Hops[recvTtl-1]

	// calculate the elapsed time
	elapsedTime := float64(recvTime-hop.SendTime) / 1000000.0 // in milliseconds

	// lock the mutex of the hop
	hop.Mutex.Lock()

	// set the elapsed time of the hop
	hop.ElapsedTime = elapsedTime

	// set the address of the response
	hop.RAddr = rAddr

	// set the isEchoReply value of the hop
	hop.IsEchoReply = recvType == 0

	// unlock the mutex of the hop
	hop.Mutex.Unlock()
}

func (g *Gotr) Trace() []map[net.Addr]float64 {
	// resolve the IP address of the destination
	ipAddr, err := net.ResolveIPAddr("ip", g.Addr)
	if err != nil {
		fmt.Println("Error resolving IP address: ", err)
	}

	// create a new ICMP connection
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		fmt.Println("Error creating ICMP connection: ", err)
	}

	// close the connection when the function returns
	defer conn.Close()

	// create a new wait group
	var wg sync.WaitGroup

	// perform the hops concurrently
	for ttl := 1; ttl <= g.Config.MaxHops; ttl++ {
		// increment the wait group counter
		wg.Add(1)
		// perform the hop
		go g.performHop(ipAddr, conn, ttl, &wg)
		// delay between each hop
		time.Sleep(g.Config.Delay)
	}

	// wait for all the hops to finish
	wg.Wait()

	// a slice to store the hops that replied
	var result []map[net.Addr]float64

	// initialize the result slice
	result = make([]map[net.Addr]float64, 0)

	// iterate over the hops until maxHops or until a hop is Echo Reply
	for _, hop := range g.Hops {
		// create a new map to store the address and the elapsed time
		m := make(map[net.Addr]float64)
		// set the address and the elapsed time
		m[hop.RAddr] = hop.ElapsedTime
		// append the map to the result slice
		result = append(result, m)

		if hop.IsEchoReply {
			break
		}
	}

	// return the result
	return result
}
