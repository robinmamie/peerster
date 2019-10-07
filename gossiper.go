package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"messages"
	"net"
	"strings"
	"time"

	"github.com/dedis/protobuf"
)

// UDPSize is the maximum number of bytes sent by a UDP message
const UDPSize = 65507

var uiPort string
var gossipAddr string
var name string
var peers []string = nil
var simple bool

// Map between an identifier and a list of RumorMessage
var msgHistory map[messages.PeerStatus]*messages.GossipPacket = make(map[messages.PeerStatus]*messages.GossipPacket)
var nextIDs map[string]uint32 = make(map[string]uint32)
var ownStatus messages.StatusPacket
var ownID uint32 = 1
var statusWaiting map[string](chan *messages.StatusPacket) = make(map[string](chan *messages.StatusPacket))

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

func (gossiper *Gossiper) listenClient() {
	for {
		textMsg := getMessage(gossiper.cliConn)
		fmt.Println("CLIENT MESSAGE", textMsg)

		if simple {
			packet := messages.GossipPacket{
				Simple: &messages.SimpleMessage{
					OriginalName:  name,
					RelayPeerAddr: gossipAddr,
					Contents:      textMsg,
				},
			}
			sendSimple(gossiper.conn, &packet)
		} else {
			packet := messages.GossipPacket{
				Rumor: &messages.RumorMessage{
					Origin: name,
					ID:     ownID,
					Text:   textMsg,
				},
			}
			ownID++
			gossiper.receivedRumor(&packet)
		}
		// TODO !! per instruction should be here, but not according to example
		fmt.Println("PEERS", strings.Join(peers, ","))
	}
}

func addressToString(address *net.UDPAddr) string {
	return address.IP.String() + ":" + fmt.Sprintf("%d", address.Port)
}

func (gossiper *Gossiper) listen() {
	for {
		packet, address := getPacket(gossiper.conn)

		// Parse address accordingly
		var addressTxt string
		if simple {
			if packet.Simple != nil {
				log.Fatal("Got an unknown message while in simple mode!")
			}
			addressTxt = packet.Simple.RelayPeerAddr
		} else if !simple {
			addressTxt = addressToString(address)
		}

		// Add sender to known peers
		// TODO should be function to return instead of breaking
		senderAbsent := true
		for _, peer := range peers {
			if peer == addressTxt {
				senderAbsent = false
				break
			}
		}
		if senderAbsent {
			peers = append(peers, addressTxt)
		}

		// SIMPLE case
		if packet.Simple != nil {
			fmt.Println("SIMPLE MESSAGE origin", packet.Simple.OriginalName,
				"from", packet.Simple.RelayPeerAddr,
				"contents", packet.Simple.Contents)
			printPeers()

			// Send packet to all other known peers
			if simple {
				sendSimple(gossiper.conn, packet)
			}
		} else if !simple && packet.Rumor != nil {

			fmt.Println("RUMOR origin", packet.Rumor.Origin, "from",
				address, "ID", packet.Rumor.ID, "contents",
				packet.Rumor.Text)
			printPeers()

			gossiper.receivedRumor(packet)
			gossiper.sendCurrentStatus(addressTxt)

		} else if !simple && packet.Status != nil {
			fmt.Print("STATUS from ", address)
			for _, s := range packet.Status.Want {
				fmt.Print(" peer ", s.Identifier, " nextID ", s.NextID)
			}
			fmt.Println()
			printPeers()

			// Wake up subroutine if status received
			unexpected := true
			for target, channel := range statusWaiting {
				if target == addressTxt {
					channel <- packet.Status
					delete(statusWaiting, target)
					unexpected = false
				}
			}

			// If unexpected Status, then compare vectors
			if unexpected {
				gossiper.compareVectors(packet.Status, addressTxt)
			}
		}

		if packet.Rumor == nil && packet.Simple == nil && packet.Status == nil {
			log.Fatal("Received an invalid package!")
		}
	}
}

func printPeers() {
	fmt.Println("PEERS", strings.Join(peers, ","))
}

func (gossiper *Gossiper) receivedRumor(packet *messages.GossipPacket) {

	// Update vector clock
	val, ok := nextIDs[packet.Rumor.Origin]
	if ok {
		if val == packet.Rumor.ID {
			// TODO ! what happens if we get message 3 but we have 1, 2 and 4?
			nextIDs[packet.Rumor.Origin] = val + 1
		}
	} else {
		if packet.Rumor.ID == 1 {
			nextIDs[packet.Rumor.Origin] = 2
		} else {
			nextIDs[packet.Rumor.Origin] = 1
		}
	}

	gossiper.rumormongerInit(packet)
}

func (gossiper *Gossiper) rumormongerInit(packet *messages.GossipPacket) {
	rumorStatus := messages.PeerStatus{
		Identifier: packet.Rumor.Origin,
		NextID:     packet.Rumor.ID,
	}
	_, present := msgHistory[rumorStatus]

	// New rumor detected
	if !present {
		msgHistory[rumorStatus] = packet
		if len(peers) != 0 {
			target := peers[rand.Int()%len(peers)]
			gossiper.rumormonger(packet, target)
		}
	}
}

func (gossiper *Gossiper) rumormonger(packet *messages.GossipPacket, target string) {

	statusWaiting[target] = make(chan *messages.StatusPacket)
	sendGossipPacket(gossiper.conn, target, packet)
	fmt.Println("MONGERING with", target)

	go func() {
		gotStatus := make(chan *messages.StatusPacket)
		go func() {
			timeout := time.NewTicker(10 * time.Second)
			select {
			case <-timeout.C:
				gotStatus <- nil
			case s := <-statusWaiting[target]:
				gotStatus <- s
			}
		}()
		var status *messages.StatusPacket
		select {
		case status = <-gotStatus:
		}
		if status == nil {
			// TODO !! should be ANOTHER one?
			target = peers[rand.Int()%len(peers)]
			gossiper.rumormonger(packet, target)
		} else if gossiper.compareVectors(status, target) && rand.Int()%2 == 0 {
			// TODO modularize this
			target = peers[rand.Int()%len(peers)]
			fmt.Println("FLIPPED COIN sending rumor to", target)
			gossiper.rumormonger(packet, target)

		}
	}()

}

func (gossiper *Gossiper) compareVectors(status *messages.StatusPacket, target string) bool {
	if status.IsEqual(nextIDs) {
		fmt.Println("IN SYNC WITH", target)
		return true
	}
	ourStatus := getCurrentStatus().Status.Want
	theirMap := make(map[string]uint32)
	for _, e := range status.Want {
		theirMap[e.Identifier] = e.NextID
	}
	// 1. Send everything we know
	for _, ourE := range ourStatus {
		// TODO modularize for both sides of the equation (1 & 2)
		theirID, ok := theirMap[ourE.Identifier]
		if !ok {
			gossiper.rumormongerPastMsg(ourE.Identifier, 1, target)
		} else if theirID < ourE.NextID {
			gossiper.rumormongerPastMsg(ourE.Identifier, theirID, target)
		}
	}
	// 2. Verify if they know something new
	for _, theirE := range status.Want {
		ourID, ok := nextIDs[theirE.Identifier]
		if !ok || ourID < theirE.NextID {
			gossiper.sendCurrentStatus(target)
		}
	}
	return false
}

func (gossiper *Gossiper) rumormongerPastMsg(origin string, id uint32, target string) {
	ps := messages.PeerStatus{
		Identifier: origin,
		NextID:     id,
	}
	gossiper.rumormonger(msgHistory[ps], target)
}

func (gossiper *Gossiper) sendCurrentStatus(address string) {
	packet := getCurrentStatus()
	sendGossipPacket(gossiper.conn, address, &packet)
}

// TODO should return a pointer
func getCurrentStatus() messages.GossipPacket {
	packet := messages.GossipPacket{
		Status: &messages.StatusPacket{
			Want: nil,
		},
	}
	for k, v := range nextIDs {
		packet.Status.Want = append(packet.Status.Want, messages.PeerStatus{
			Identifier: k,
			NextID:     v,
		})
	}
	return packet
}

func getMessage(connection *net.UDPConn) string {
	var packetBytes []byte = make([]byte, UDPSize)
	var packet messages.Message

	n, _, err := connection.ReadFromUDP(packetBytes)
	if err != nil {
		log.Fatal(err)
	}

	// Decode packet
	err = protobuf.Decode(packetBytes[:n], &packet)
	if err != nil {
		log.Fatal(err)
	}

	return packet.Text
}

func getPacket(connection *net.UDPConn) (*messages.GossipPacket, *net.UDPAddr) {
	var packetBytes []byte = make([]byte, UDPSize)
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

func (gossiper *Gossiper) antiEntropy(ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			target := peers[rand.Int()%len(peers)]
			packet := getCurrentStatus()
			sendGossipPacket(gossiper.conn, target, &packet)
		default:
		}
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
	go gossiper.antiEntropy(time.NewTicker(10 * time.Second))
	go gossiper.listenClient()
	gossiper.listen()
}
