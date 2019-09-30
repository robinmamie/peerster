package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
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

// Map between an identifier and a list of RumorMessage
var msgHistory map[messages.PeerStatus]*messages.RumorMessage = make(map[messages.PeerStatus]*messages.RumorMessage)
var ownStatus messages.StatusPacket
var ownID uint32 = 0

// Gossiper defines a peer and stores the necessary information to handle it.
type Gossiper struct {
	address *net.UDPAddr
	conn    *net.UDPConn
	cliConn *net.UDPConn
	Name    string
}

// NewGossiper creates a Gossiper with a given address and name.
func NewGossiper(address, name string) *Gossiper {
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		log.Fatal(err)
	}
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		log.Fatal(err)
	}

	cliAddr, err := net.ResolveUDPAddr("udp4", ":"+uiPort)
	if err != nil {
		log.Fatal(err)
	}
	cliConn, err := net.ListenUDP("udp4", cliAddr)
	if err != nil {
		log.Fatal(err)
	}

	return &Gossiper{
		address: udpAddr,
		conn:    udpConn,
		cliConn: cliConn,
		Name:    name,
	}
}

func (gossiper *Gossiper) listen(client bool) {
	for {
		var packet *messages.GossipPacket
		var address *net.UDPAddr
		if client {
			packet, address = getPacket(gossiper.cliConn)
			if simple {
				packet.Simple.OriginalName = name
			}
			fmt.Println("CLIENT MESSAGE", packet.Simple.Contents)
		} else {
			packet, _ = getPacket(gossiper.conn)

			// Add peer to list if it is unknown
			senderAbsent := true
			for _, peer := range peers {
				if peer == packet.Simple.RelayPeerAddr {
					senderAbsent = false
				}
			}
			if senderAbsent {
				peers = append(peers, packet.Simple.RelayPeerAddr)
			}
		}

		// SIMPLE case
		if simple {
			if !client {
				fmt.Println("SIMPLE MESSAGE origin", packet.Simple.OriginalName,
					"from", packet.Simple.RelayPeerAddr,
					"contents", packet.Simple.Contents)
			}

			// Send packet to all other known peers
			sendSimple(gossiper.conn, packet)

			// RUMORMONGERING PROTOCOL
			// RUMOR case
			// TODO should create new routine, because we need to wait for the answer,
			// but the Gossiper should not block any other message from coming in
		} else if packet.Rumor != nil {
			// Set origin if rumor comes from client
			if client {
				packet.Rumor.Origin = name
				packet.Rumor.ID = ownID
				ownID++
			} else {
				fmt.Println("RUMOR origin", packet.Rumor.Origin, "from",
					address, "ID", packet.Rumor.ID, "contents",
					packet.Rumor.Text)
			}

			// TODO find a better solution. The last rumor should be
			rumorStatus := messages.PeerStatus{
				Identifier: packet.Rumor.Origin,
				NextID:     packet.Rumor.ID,
			}
			_, present := msgHistory[rumorStatus]

			// New rumor detected
			if !present {
				msgHistory[rumorStatus] = packet.Rumor
				target := peers[rand.Int()%len(peers)]
				sendGossipPacket(gossiper.conn, target, packet)
				fmt.Println("MONGERING with", address)
			}

			// TODO Send status packet to sender

			// STATUS case
		} else if packet.Status != nil {
			fmt.Print("STATUS from", address)
			for _, s := range packet.Status.Want {
				fmt.Print(" peer", s.Identifier, "nextID", s.NextID)
			}
			fmt.Println()

			// TODO compare StatusPackets

			// TODO handle case where we have more to send
			// TODO handle case where they have more to send
			// TODO handle case where everything is equal
		}

		// TODO send status packet back to previous sender

		fmt.Println("PEERS", strings.Join(peers, ","))
	}
}

func getPacket(connection *net.UDPConn) (*messages.GossipPacket, *net.UDPAddr) {
	var packetBytes []byte = make([]byte, 1024) // TODO fix a sensible value
	var packet messages.GossipPacket

	// Retrieve packet
	// The address of the sender could be retrieved here (2nd argument)
	n, address, err := connection.ReadFromUDP(packetBytes)
	if err != nil {
		log.Fatal(err)
	}

	// Decode packet
	err = protobuf.Decode(packetBytes[:n], &packet)
	if err != nil {
		log.Fatal(err)
	}

	return &packet, address
}

func sendSimple(connection *net.UDPConn, packet *messages.GossipPacket) {
	// Save previous address, update packet with address and encode it
	fromPeer := packet.Simple.RelayPeerAddr
	packet.Simple.RelayPeerAddr = gossipAddr
	packetBytes, err := protobuf.Encode(packet)
	if err != nil {
		log.Fatal(err)
	}

	// Send to all peers except the last sender
	for _, address := range peers {
		if address != fromPeer {
			sendPacket(connection, address, packetBytes)
		}
	}
}

func sendGossipPacket(connection *net.UDPConn, address string, packet *messages.GossipPacket) {
	packetBytes, err := protobuf.Encode(packet)
	if err != nil {
		log.Fatal(err)
	}
	sendPacket(connection, address, packetBytes)
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

	gossiper := NewGossiper(gossipAddr, name)

	// Listen to client and other gossipers
	go gossiper.listen(true)
	gossiper.listen(false)
}
