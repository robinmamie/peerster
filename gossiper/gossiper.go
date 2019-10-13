package gossiper

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/robinmamie/Peerster/messages"

	"github.com/dedis/protobuf"
)

// UDPSize is the maximum number of bytes sent by a UDP message
const UDPSize = 65507

// TODO put all variables INSIDE gossiper
// Map between an identifier and a list of RumorMessage
var msgHistory map[messages.PeerStatus]*messages.GossipPacket = make(map[messages.PeerStatus]*messages.GossipPacket)
var latestMessages []*messages.RumorMessage = nil
var nextIDs map[string]uint32 = make(map[string]uint32)
var ownStatus messages.StatusPacket
var ownID uint32 = 1

var statusWaiting map[string](chan *messages.StatusPacket) = make(map[string](chan *messages.StatusPacket))
var expected map[string]chan bool = make(map[string]chan bool)

// Gossiper defines a peer and stores the necessary information to handle it.
type Gossiper struct {
	address *net.UDPAddr
	conn    *net.UDPConn
	cliConn *net.UDPConn
	Name    string
	simple  bool
	Peers   []string
}

// NewGossiper creates a Gossiper with a given address and name.
func NewGossiper(address, name string, uiPort string, simple bool, peers []string) *Gossiper {
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

	for _, p := range peers {
		channel := make(chan *messages.StatusPacket)
		statusWaiting[p] = channel
		expChannel := make(chan bool)
		expected[p] = expChannel
	}

	return &Gossiper{
		address: udpAddr,
		conn:    udpConn,
		cliConn: cliConn,
		Name:    name,
		simple:  simple,
		Peers:   peers,
	}
}

// Run starts the node and runs it.
func (gossiper *Gossiper) Run(antiEntropy uint64) {
	// Activate anti-entropy if necessary
	if !gossiper.simple {
		go gossiper.antiEntropy(antiEntropy)
	}

	go gossiper.listenClient()
	gossiper.listen()
}

// ListenClient handles the messages coming from the client.
func (gossiper *Gossiper) listenClient() {
	for {
		textMsg := getMessage(gossiper.cliConn)
		fmt.Println("CLIENT MESSAGE", textMsg)

		if gossiper.simple {
			packet := &messages.GossipPacket{
				Simple: &messages.SimpleMessage{
					OriginalName:  gossiper.Name,
					RelayPeerAddr: addressToString(gossiper.address),
					Contents:      textMsg,
				},
			}
			gossiper.sendSimple(packet)
		} else {
			packet := &messages.GossipPacket{
				Rumor: &messages.RumorMessage{
					Origin: gossiper.Name,
					ID:     ownID,
					Text:   textMsg,
				},
			}
			ownID++
			gossiper.receivedNewRumor(packet)
		}
	}
}

func addressToString(address *net.UDPAddr) string {
	return address.IP.String() + ":" + fmt.Sprintf("%d", address.Port)
}

// Listen handles all the messages coming from other gossipers.
func (gossiper *Gossiper) listen() {

	for {
		packet, address := getPacket(gossiper.conn)

		// Parse address accordingly
		var addressTxt string
		if gossiper.simple {
			if packet.Simple == nil {
				// Ignore any unexpected package
				continue
			}
			addressTxt = packet.Simple.RelayPeerAddr
		} else if !gossiper.simple {
			addressTxt = addressToString(address)
		}

		// Add sender to known peers
		if gossiper.isSenderAbsent(addressTxt) {
			gossiper.Peers = append(gossiper.Peers, addressTxt)
			channel := make(chan *messages.StatusPacket)
			statusWaiting[addressTxt] = channel
			expChannel := make(chan bool)
			expected[addressTxt] = expChannel
		}

		// SIMPLE case
		if packet.Simple != nil {
			fmt.Println("SIMPLE MESSAGE origin", packet.Simple.OriginalName,
				"from", packet.Simple.RelayPeerAddr,
				"contents", packet.Simple.Contents)
			gossiper.printPeers()

			// Send packet to all other known peers
			if gossiper.simple {
				gossiper.sendSimple(packet)
			}
		} else if !gossiper.simple && packet.Rumor != nil {

			fmt.Println("RUMOR origin", packet.Rumor.Origin, "from",
				address, "ID", packet.Rumor.ID, "contents",
				packet.Rumor.Text)
			gossiper.printPeers()

			gossiper.receivedNewRumor(packet)
			gossiper.sendCurrentStatus(addressTxt)

		} else if !gossiper.simple && packet.Status != nil {
			fmt.Print("STATUS from ", address)
			for _, s := range packet.Status.Want {
				fmt.Print(" peer ", s.Identifier, " nextID ", s.NextID)
			}
			fmt.Println()
			gossiper.printPeers()

			// Wake up correctsubroutine if status received
			unexpected := false
			for target, channel := range statusWaiting {

				if target == addressTxt {
					// Empty expected channel before
					for len(expected[target]) > 0 {
						<-expected[target]
					}

					// Send packet to correct channel
					select {
					case channel <- packet.Status:
						timeout := time.NewTicker(10 * time.Millisecond)
						select {
						case <-expected[target]:
						case <-timeout.C:
							unexpected = true
						default:
							unexpected = true
						}
					default:
						unexpected = true
					}
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

func (gossiper *Gossiper) isSenderAbsent(address string) bool {
	for _, peer := range gossiper.Peers {
		if peer == address {
			return false
		}
	}
	return true
}

func (gossiper *Gossiper) printPeers() {
	fmt.Println("PEERS", strings.Join(gossiper.Peers, ","))
}

// receivedRumor returns true if the given rumor is not present.
func (gossiper *Gossiper) receivedNewRumor(packet *messages.GossipPacket) {

	rumorStatus := messages.PeerStatus{
		Identifier: packet.Rumor.Origin,
		NextID:     packet.Rumor.ID,
	}
	_, present := msgHistory[rumorStatus]

	// Update vector clock
	val, ok := nextIDs[packet.Rumor.Origin]
	if ok {
		if val == packet.Rumor.ID {
			stillPresent := present
			status := rumorStatus
			// Verify if a sequence was completed
			for stillPresent {
				status.NextID++
				nextIDs[packet.Rumor.Origin]++
				_, stillPresent = msgHistory[status]
			}
		}
		// else do not update vector clock, will be done once the sequence is completed
	} else {
		// It's a new message, initialize the vector clock accordingly.
		if packet.Rumor.ID == 1 {
			nextIDs[packet.Rumor.Origin] = 2
		} else {
			nextIDs[packet.Rumor.Origin] = 1
		}
	}

	// New rumor detected
	if !present {
		msgHistory[rumorStatus] = packet
		latestMessages = append(latestMessages, packet.Rumor)
		// TODO code copy with AntiEntropy and rumormonger in status == nil
		if len(gossiper.Peers) != 0 {
			target := gossiper.Peers[rand.Int()%len(gossiper.Peers)]
			go gossiper.rumormonger(packet, target)
		}
	}
}

func (gossiper *Gossiper) rumormonger(packet *messages.GossipPacket, target string) {

	sendGossipPacket(gossiper.conn, target, packet)
	fmt.Println("MONGERING with", target)

	// Set timeout and listen to acknowledgement channel
	timeout := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-timeout.C:
			target = gossiper.Peers[rand.Int()%len(gossiper.Peers)]
			defer gossiper.rumormonger(packet, target)
			return

		case status := <-statusWaiting[target]:
			for _, sp := range status.Want {
				if sp.Identifier == packet.Rumor.Origin && sp.NextID > packet.Rumor.ID {
					// Announce that the package is expected
					expected[target] <- true
					if gossiper.compareVectors(status, target) && rand.Int()%2 == 0 {
						// TODO modularize this
						target = gossiper.Peers[rand.Int()%len(gossiper.Peers)]
						fmt.Println("FLIPPED COIN sending rumor to", target)
						defer gossiper.rumormonger(packet, target)
					}
					return
				}
			}
		}
	}

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
			return false
		}
		if theirID < ourE.NextID {
			gossiper.rumormongerPastMsg(ourE.Identifier, theirID, target)
			return false
		}
	}
	// 2. Verify if they know something new
	for _, theirE := range status.Want {
		ourID, ok := nextIDs[theirE.Identifier]
		if !ok || ourID < theirE.NextID {
			gossiper.sendCurrentStatus(target)
			return false
		}
	}
	log.Fatal("Undefined behavior in vector comparison")
	return false
}

func (gossiper *Gossiper) rumormongerPastMsg(origin string, id uint32, target string) {
	ps := messages.PeerStatus{
		Identifier: origin,
		NextID:     id,
	}
	go gossiper.rumormonger(msgHistory[ps], target)
}

func (gossiper *Gossiper) sendCurrentStatus(address string) {
	packet := getCurrentStatus()
	sendGossipPacket(gossiper.conn, address, packet)
}

// TODO should return a pointer
func getCurrentStatus() *messages.GossipPacket {
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
	return &packet
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

func (gossiper *Gossiper) sendSimple(packet *messages.GossipPacket) {
	// Save previous address, update packet with address and encode it
	fromPeer := packet.Simple.RelayPeerAddr
	packet.Simple.RelayPeerAddr = addressToString(gossiper.address)
	packetBytes, err := protobuf.Encode(packet)
	if err != nil {
		log.Fatal(err)
	}

	// Send to all peers except the last sender
	for _, address := range gossiper.Peers {
		if address != fromPeer {
			sendPacket(gossiper.conn, address, packetBytes)
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

// AntiEntropy fires every 10 seconds to send a StatusPacket to a random peer.
func (gossiper *Gossiper) antiEntropy(delay uint64) {
	ticker := time.NewTicker(time.Duration(delay) * time.Second)
	for {
		select {
		case <-ticker.C:
			// TODO ! example to modularize
			if len(gossiper.Peers) != 0 {
				target := gossiper.Peers[rand.Int()%len(gossiper.Peers)]
				gossiper.sendCurrentStatus(target)
			}
		default:
		}
	}
}

// GetLatestRumorMessagesList returns a list of the latest rumor messages.
func (gossiper *Gossiper) GetLatestRumorMessagesList() []*messages.RumorMessage {
	// TODO should delete or just keep a fixed size?
	defer func() {
		latestMessages = nil
	}()
	return latestMessages
}
