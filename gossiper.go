package main

import (
	"flag"
	"fmt"
	"log"
	"messages"
	"net"
	"strings"

	"github.com/dedis/protobuf"
)

var uiPort string
var gossipAddr string
var name string
var peers []string = nil
var simple bool

var msgHistory map[messages.PeerStatus]string = make(map[messages.PeerStatus]string)
var ownStatus messages.StatusPacket

func parseFlags() {
	flag.StringVar(&uiPort, "UIPort", "8080", "port for the UI client")
	flag.StringVar(&gossipAddr, "gossipAddr", "127.0.0.1:5000",
		"ip:port for the gossiper")
	flag.StringVar(&name, "name", "", "name of the gossiper")
	var peersString string
	flag.StringVar(&peersString, "peers", "",
		"comma separated list of peers of the form ip:port")
	flag.BoolVar(&simple, "simple", false,
		"run gossiper in simple broadcast mode")

	flag.Parse()

	if peersString != "" {
		peers = strings.Split(peersString, ",")
	}
}

// Gossiper defines a peer and stores the necessary information to handle it.
type Gossiper struct {
	address *net.UDPAddr
	conn    *net.UDPConn
	Name    string
}

// NewGossiper creates a Gossiper with a given address and name.
func NewGossiper(address, name string) *Gossiper {
	// TODO add socket for communicating with client in Gossiper(?)
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		log.Fatal(err)
	}
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		log.Fatal(err)
	}
	return &Gossiper{
		address: udpAddr,
		conn:    udpConn,
		Name:    name,
	}
}

func (gossiper *Gossiper) listenGossipers() {
	for {
		packet := getPacket(gossiper.conn)
		if simple {
			fmt.Println("SIMPLE MESSAGE origin", packet.Simple.OriginalName,
				"from", packet.Simple.RelayPeerAddr,
				"contents", packet.Simple.Contents)
			sendSimple(gossiper.conn, packet)
		}
	}
}

func listenClient() {

	udpAddr, err := net.ResolveUDPAddr("udp4", ":"+uiPort)
	if err != nil {
		log.Fatal(err)
	}
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		log.Fatal(err)
	}
	for {
		packet := getPacket(udpConn)
		if simple {
			packet.Simple.OriginalName = name
			fmt.Println("CLIENT MESSAGE", packet.Simple.Contents)
			sendSimple(udpConn, packet)
		}
	}
}

func getPacket(connection *net.UDPConn) *messages.GossipPacket {
	var packetBytes []byte = make([]byte, 1024) // TODO fix a sensible value
	var packet messages.GossipPacket

	// Retrieve packet
	// The address of the sender could be retrieved here (2nd argument)
	n, _, err := connection.ReadFromUDP(packetBytes)
	if err != nil {
		log.Fatal(err)
	}

	// Decode packet
	err = protobuf.Decode(packetBytes[:n], &packet)
	if err != nil {
		log.Fatal(err)
	}
	return &packet
}

func sendSimple(connection *net.UDPConn, packet *messages.GossipPacket) {
	// Update packet with address and encode packet
	fromPeer := packet.Simple.RelayPeerAddr
	packet.Simple.RelayPeerAddr = gossipAddr
	packetBytes, err := protobuf.Encode(packet)
	if err != nil {
		log.Fatal(err)
	}

	// Send to all peers except the last sender
	fromPeerShallBeAdded := fromPeer != "" // should not add client
	for _, address := range peers {
		if address != fromPeer {
			sendPacket(connection, address, packetBytes)
		} else {
			fromPeerShallBeAdded = false
		}
	}

	// Add peer if not yet present, and display all known peers
	if fromPeerShallBeAdded {
		peers = append(peers, fromPeer)
	}
	fmt.Println("PEERS", strings.Join(peers, ","))
}

func sendPacket(connection *net.UDPConn, address string, packetBytes []byte) {
	udpDest, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		log.Fatal(err)
	}
	bytes, err := connection.WriteToUDP(packetBytes, udpDest)
	if err != nil {
		log.Fatal(err)
	}
	if bytes != len(packetBytes) {
		log.Fatal(bytes, "bytes were sent instead of", len(packetBytes),
			"bytes.")
	}
}

func main() {

	parseFlags()

	go listenClient()
	gossiper := NewGossiper(gossipAddr, name)
	gossiper.listenGossipers()
}
