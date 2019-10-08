package main

import (
	"flag"
	"log"
	"net"

	"github.com/robinmamie/Peerster/messages"

	"github.com/dedis/protobuf"
)

func main() {
	var uiPort string
	var textMsg string
	flag.StringVar(&uiPort, "UIPort", "8080", "port for the UI client")
	flag.StringVar(&textMsg, "msg", "", "message to be sent")
	flag.Parse()

	// Create and encode packet
	msg := messages.Message{
		Text: textMsg,
	}
	packetBytes, err := protobuf.Encode(&msg)
	if err != nil {
		log.Fatal(err)
	}

	// Create local UDP connection
	// TODO !! is there a better way than assigning a unique port? ->net.Dial!
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
