package main

import (
	"flag"
	"log"
	"messages"
	"net"

	"github.com/dedis/protobuf"
)

func main() {
	var uiPort string
	var msg string
	flag.StringVar(&uiPort, "UIPort", "8080", "port for the UI client")
	flag.StringVar(&msg, "msg", "", "message to be sent")
	flag.Parse()

	// Create and encode packet
	simple := &messages.SimpleMessage{
		OriginalName:  "Client",
		RelayPeerAddr: "",
		Contents:      msg,
	}
	packet := messages.GossipPacket{Simple: simple}
	packetBytes, err := protobuf.Encode(&packet)
	if err != nil {
		log.Fatal(err)
	}

	// Create local UDP connection
	// TODO is there a better way than assigning a unique port?
	udpAddr, err := net.ResolveUDPAddr("udp4", ":4242")
	if err != nil {
		log.Fatal(err)
	}
	udpConn, _ := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer udpConn.Close()

	// Resolve recipient address and send packet
	udpDest, err := net.ResolveUDPAddr("udp4", ":"+uiPort)
	if err != nil {
		log.Fatal(err)
	}
	bytes, err := udpConn.WriteToUDP(packetBytes, udpDest)
	if err != nil {
		log.Fatal(err)
	}
	if bytes != len(packetBytes) {
		log.Fatal(bytes, "bytes were sent instead of", len(packetBytes),
			"bytes.")
	}
}
