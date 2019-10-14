package main

import (
	"flag"
	"log"
	"net"

	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"

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
	tools.Check(err)

	// Create local UDP connection
	conn, err := net.Dial("udp4", ":"+uiPort)
	tools.Check(err)

	// Send packet and close connection
	bytes, err := conn.Write(packetBytes)
	tools.Check(err)
	if bytes != len(packetBytes) {
		log.Fatal(bytes, "bytes were sent instead of", len(packetBytes),
			"bytes.")
	}

	err = conn.Close()
	tools.Check(err)
}
