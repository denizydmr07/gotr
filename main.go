/*
Package to send ICMP Echo Requests to a host with
different TTL values and print the results.
*/
package main

import (
	"log"
	"net"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// Hop struct to store the information of a hop
type Hop struct {
	sendTime    int64      // UnixNano
	elapsedTime float64    // in milliseconds
	ttl         int        // Time to Live
	rAddr       net.Addr   // address that replied to the request
	mutex       sync.Mutex // to synchronize access to the struct, first sendTime, then elapsedTime
	numTries    int        // number of tries
	isEchoReply bool       // is the response an echo reply
}

type Hops struct {
	// create a new slice to store the hops, with a maximum of 30 hops
	hops [30]Hop
	// create a new mutex to synchronize access to the hops
	mutex sync.Mutex
}

func performHop(ipAddr *net.IPAddr, conn *icmp.PacketConn, ttl int, hops *Hops, wg *sync.WaitGroup) {
	defer func() {
		// check if the hop with the corresponding TTL value is not recived
		hops.hops[ttl-1].mutex.Lock()
		if hops.hops[ttl-1].rAddr == nil && hops.hops[ttl-1].numTries < 3 {
			// increment the number of tries
			hops.hops[ttl-1].numTries++
			// unlock the mutex of the hop
			hops.hops[ttl-1].mutex.Unlock()
			log.Printf("Performing hop again with TTL: %d\n", ttl)
			// if not recieved, perform the hop again
			performHop(ipAddr, conn, ttl, hops, wg)
		} else {
			// unlock the mutex of the hop
			hops.hops[ttl-1].mutex.Unlock()
			// decrement the wait group
			wg.Done()
		}
	}()
	hops.mutex.Lock()
	// get the hop with the corresponding TTL value
	hop := &hops.hops[ttl-1]
	// set the TTL value of the hop
	hop.ttl = ttl
	hops.mutex.Unlock()

	// send ICMP Echo Request
	sendTime, err := sendICMPEchoRequest(ipAddr, conn, ttl, hop)
	if err != nil {
		return
	}

	log.Printf("Sendtime: %d\n", sendTime)

	// create a new byte slice to store the response
	rb := make([]byte, 1500)

	// read the response from the connection
	r_addr, recvTime, err := recvICMPEchoReply(conn, rb)
	if err != nil {
		return
	}

	// ICMP echo reply is 36 bytes, 0..35
	// ICMP exho reply header is 8 bytes, 0..7
	// IP header is 20 bytes, 8..27 (in the data of the ICMP reply)
	// ICMP echo reequest header is 8 bytes, 28..35 (in the data of the ICMP reply)
	// ID of the ICMP echo reply is the initial ttl value of the echo request
	// ID is 2 bytes, 32..33 (in the data of the ICMP reply)

	// gettting the TTL value from the response, two bytes big-endian to integer
	// TODO: find the hop with the corresponding TTL value from hops array
	var recvTtl int
	recvType := int(rb[0])

	if recvType == 11 {
		recvTtl = int(rb[32])<<8 | int(rb[33])
	} else if recvType == 0 {
		recvTtl = int(rb[4])<<8 | int(rb[5])
		log.Printf("Received TTL: %d\n", recvTtl)
	}

	// get the hop with the corresponding TTL value
	hop = &hops.hops[recvTtl-1]

	// calculate the elapsed time
	elapsedTime := float64(recvTime-hop.sendTime) / 1000000.0

	// lock the mutex of the hop
	hop.mutex.Lock()

	// set the elapsed time of the hop
	hop.elapsedTime = elapsedTime

	// set the remote address of the hop
	hop.rAddr = r_addr

	// set the isEchoReply of the hop
	if recvType == 0 {
		hop.isEchoReply = true
	} else {
		hop.isEchoReply = false
	}

	// unlock the mutex of the hop
	hop.mutex.Unlock()

	// print the results
	//log.Printf("TTL: %d, Address: %s, Elapsed Time: %.2f ms\n", hop.ttl, hop.rAddr.String(), hop.elapsedTime)
}

// sendICMPEchoRequest sends an ICMP Echo Request to the specified IP address, returns the send time in UnixNan0 and an error
func sendICMPEchoRequest(ipAddr *net.IPAddr, conn *icmp.PacketConn, ttl int, hop *Hop) (int64, error) {
	// lock the mutex of the hop first
	hop.mutex.Lock()

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
		return 0, err
	}

	// set the TTL value of the message
	if err := conn.IPv4PacketConn().SetTTL(ttl); err != nil {
		return 0, err
	}

	// get the current time in UnixNano
	sendTime := int64(time.Now().UnixNano())
	// write the message to the connection
	if _, err := conn.WriteTo(b, ipAddr); err != nil {
		log.Println("Error writing to connection:", err)
		return 0, err
	}

	// set the send time of the hop
	hop.sendTime = sendTime

	// unlock the mutex of the hop
	hop.mutex.Unlock()

	return sendTime, nil
}

func recvICMPEchoReply(conn *icmp.PacketConn, rb []byte) (net.Addr, int64, error) {
	// set the read deadline of the connection
	(*conn).SetReadDeadline(time.Now().Add(3 * time.Second))
	// read the response from the connection
	_, r_addr, err := conn.ReadFrom(rb)
	if err != nil {
		log.Println("Error reading from connection:", err)
		return nil, 0, err
	}

	// get the current time in UnixNano
	recvTime := int64(time.Now().UnixNano())

	return r_addr, recvTime, nil
}

func main() {
	log.SetPrefix("gotr: ")
	log.SetFlags(0)
	// resolve the IP address of the host
	ipAddr, err := net.ResolveIPAddr("ip", "www.google.com")
	if err != nil {
		log.Println("Error resolving IP address:", err)
		return
	}

	// create a new ICMP connection
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		log.Println("Error creating ICMP connection:", err)
		return
	}

	// close the connection when the function returns
	defer conn.Close()

	// create a new Hops struct to store the hops
	hops := Hops{}

	// ceate a wait group to wait for the hops to finish
	var wg sync.WaitGroup

	// perform the hop
	for ttl := 1; ttl <= 15; ttl++ {
		// increment the wait group
		wg.Add(1)
		// call perform hop concurrently
		go performHop(ipAddr, conn, ttl, &hops, &wg)
		// sleep for 50 ms
		time.Sleep(50 * time.Millisecond)
	}

	// wait for the hops to finish
	wg.Wait()

	// print the results
	for i := 0; i < 15; i++ {
		hop := &hops.hops[i]
		if hop.rAddr != nil {
			log.Printf("TTL: %d, Address: %s, Elapsed Time: %.2f ms, isEchoReply: %t\n", hop.ttl, hop.rAddr.String(), hop.elapsedTime, hop.isEchoReply)
		} else {
			log.Printf("TTL: %d, Address: %s, Elapsed Time: %.2f ms\n", hop.ttl, "Request Timed Out", 0.0)
		}
	}
}
