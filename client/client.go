package main

import (
    "flag"
    "messages"
    "net"
    "github.com/dedis/protobuf"
)

var uiPort string
var msg string

func parseFlags() {
    flag.StringVar(&uiPort, "UIPort", "8080", "port for the UI client")
    flag.StringVar(&msg, "msg", "", "message to be sent")
    flag.Parse()
}

func main() {
    parseFlags()

    address := "127.0.0.1:1"+uiPort
    recipient := "127.0.0.1:"+uiPort

    smsg := &messages.SimpleMessage{OriginalName: "Client", RelayPeerAddr: address, Contents: msg}
    packet := messages.GossipPacket{Simple: smsg}
    packetBytes, _ := protobuf.Encode(&packet)
    protobuf.Decode(packetBytes, &packet)

    udpAddr, _ := net.ResolveUDPAddr("udp4", address)
    udpConn, _ := net.ListenUDP("udp4", udpAddr)
    udpDest, _ := net.ResolveUDPAddr("udp4", recipient)
    udpConn.WriteToUDP(packetBytes, udpDest)
}
