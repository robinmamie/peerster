package gossiper

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"

	"github.com/dedis/protobuf"
)

// Gossiper defines a peer and stores the necessary information to use it.
type Gossiper struct {
	Address         string
	conn            *net.UDPConn
	cliConn         *net.UDPConn
	UIPort          string
	Name            string
	simple          bool
	Peers           []string
	statusWaiting   map[string]chan *messages.StatusPacket
	expected        map[string]chan bool
	msgHistory      map[messages.PeerStatus]*messages.GossipPacket
	allMessages     []*messages.RumorMessage
	latestMessageID int
	nextIDs         map[string]uint32
	ownID           uint32
}

// NewGossiper creates a Gossiper with a given address, name, port, mode and
// list of peers.
func NewGossiper(address, name string, uiPort string, simple bool, peers []string) *Gossiper {
	// Creation of all necessary UDP sockets.
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	tools.Check(err)
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	tools.Check(err)

	cliAddr, err := net.ResolveUDPAddr("udp4", ":"+uiPort)
	tools.Check(err)
	cliConn, err := net.ListenUDP("udp4", cliAddr)
	tools.Check(err)

	// Channels used to communicate between routines during rumormongering.
	statusWaiting := make(map[string](chan *messages.StatusPacket))
	expected := make(map[string]chan bool)
	for _, p := range peers {
		channel := make(chan *messages.StatusPacket)
		statusWaiting[p] = channel
		expChannel := make(chan bool)
		expected[p] = expChannel
	}

	// Map between an identifier and a list of RumorMessage
	msgHistory := make(map[messages.PeerStatus]*messages.GossipPacket)
	allMessages := make([]*messages.RumorMessage, 0)
	nextIDs := make(map[string]uint32)

	return &Gossiper{
		Address:         address,
		conn:            udpConn,
		cliConn:         cliConn,
		UIPort:          uiPort,
		Name:            name,
		simple:          simple,
		Peers:           peers,
		statusWaiting:   statusWaiting,
		expected:        expected,
		msgHistory:      msgHistory,
		allMessages:     allMessages,
		latestMessageID: 0,
		nextIDs:         nextIDs,
		ownID:           1,
	}
}

// Run starts the node and runs it.
func (gossiper *Gossiper) Run(antiEntropyDelay uint64) {
	// Activate anti-entropy if necessary
	if !gossiper.simple && antiEntropyDelay > 0 {
		go gossiper.antiEntropy(antiEntropyDelay)
	}

	go gossiper.listenClient()
	gossiper.listen()
}

// listenClient handles the messages coming from the client.
func (gossiper *Gossiper) listenClient() {
	for {
		textMsg := gossiper.getMessage()
		fmt.Println("CLIENT MESSAGE", textMsg)

		if gossiper.simple {
			packet := &messages.GossipPacket{
				Simple: &messages.SimpleMessage{
					OriginalName:  gossiper.Name,
					RelayPeerAddr: gossiper.Address,
					Contents:      textMsg,
				},
			}
			gossiper.sendSimple(packet)
		} else {
			packet := &messages.GossipPacket{
				Rumor: &messages.RumorMessage{
					Origin: gossiper.Name,
					ID:     gossiper.ownID,
					Text:   textMsg,
				},
			}
			gossiper.ownID++
			gossiper.receivedRumor(packet)
		}
	}
}

// listen handles all the messages coming from other gossipers.
func (gossiper *Gossiper) listen() {
	for {
		packet, address := gossiper.getGossipPacket()

		// Parse address accordingly
		var addressTxt string
		if gossiper.simple {
			if packet.Simple == nil {
				// Ignore any unexpected package
				continue
			}
			addressTxt = packet.Simple.RelayPeerAddr
		} else if !gossiper.simple {
			addressTxt = tools.AddressToString(address)
		}

		// Add sender to known peers
		if gossiper.isSenderAbsent(addressTxt) {
			gossiper.Peers = append(gossiper.Peers, addressTxt)
			channel := make(chan *messages.StatusPacket)
			gossiper.statusWaiting[addressTxt] = channel
			expChannel := make(chan bool)
			gossiper.expected[addressTxt] = expChannel
		}

		if packet.Simple != nil {
			// SIMPLE case
			fmt.Println("SIMPLE MESSAGE origin", packet.Simple.OriginalName,
				"from", packet.Simple.RelayPeerAddr,
				"contents", packet.Simple.Contents)
			gossiper.printPeers()

			// Send packet to all other known peers if we are in simple mode
			if gossiper.simple {
				gossiper.sendSimple(packet)
			}
		} else if !gossiper.simple && packet.Rumor != nil {
			// Received a rumor
			fmt.Println("RUMOR origin", packet.Rumor.Origin, "from",
				address, "ID", packet.Rumor.ID, "contents",
				packet.Rumor.Text)
			gossiper.printPeers()

			gossiper.receivedRumor(packet)
			gossiper.sendCurrentStatus(addressTxt)

		} else if !gossiper.simple && packet.Status != nil {
			// Received a status message
			fmt.Print("STATUS from ", address)
			for _, s := range packet.Status.Want {
				fmt.Print(" peer ", s.Identifier, " nextID ", s.NextID)
			}
			fmt.Println()
			gossiper.printPeers()
			if packet.Status.IsEqual(gossiper.nextIDs) {
				fmt.Println("IN SYNC WITH", addressTxt)
			}

			// Wake up correctsubroutine if status received
			unexpected := true
			for target, channel := range gossiper.statusWaiting {

				if target == addressTxt {
					// Empty expected channel before
					for len(gossiper.expected[target]) > 0 {
						<-gossiper.expected[target]
					}

					// Send packet to correct channel, as many times as possible
					listening := true
					for listening {
						select {
						case channel <- packet.Status:
							// Allow for the routine to process the message
							timeout := time.NewTicker(10 * time.Millisecond)
							select {
							case <-gossiper.expected[target]:
								unexpected = false
							case <-timeout.C:
								listening = false
							}
						default:
							listening = false
						}
					}
				}
			}
			// If unexpected Status, then compare vectors
			if unexpected {
				gossiper.compareVectors(packet.Status, addressTxt)
			}
		}
	}
}

// isSenderAbsent returns true if the given address is not in the list of known
// peers yet.
func (gossiper *Gossiper) isSenderAbsent(address string) bool {
	for _, peer := range gossiper.Peers {
		if peer == address {
			return false
		}
	}
	return true
}

// printPeers prints the list of known peers.
func (gossiper *Gossiper) printPeers() {
	fmt.Println("PEERS", strings.Join(gossiper.Peers, ","))
}

// receivedRumor handles any received rumor, new or not.
func (gossiper *Gossiper) receivedRumor(packet *messages.GossipPacket) {

	rumorStatus := messages.PeerStatus{
		Identifier: packet.Rumor.Origin,
		NextID:     packet.Rumor.ID,
	}
	_, present := gossiper.msgHistory[rumorStatus]

	// New rumor detected
	if !present {
		// Add rumor to history
		gossiper.msgHistory[rumorStatus] = packet
		gossiper.allMessages = append(gossiper.allMessages, packet.Rumor)

		gossiper.updateVectorClock(packet, rumorStatus)

		if target, ok := gossiper.pickRandomPeer(); ok {
			gossiper.rumormonger(packet, target)
		}
	}
}

// updateVectorClock updates the internal vector clock.
func (gossiper *Gossiper) updateVectorClock(packet *messages.GossipPacket, rumorStatus messages.PeerStatus) {
	if val, ok := gossiper.nextIDs[packet.Rumor.Origin]; ok {
		if val == packet.Rumor.ID {
			stillPresent := true
			status := rumorStatus
			// Verify if a sequence was completed
			for stillPresent {
				status.NextID++
				gossiper.nextIDs[packet.Rumor.Origin]++
				_, stillPresent = gossiper.msgHistory[status]
			}
		}
		// else do not update vector clock, will be done once the sequence is completed

	} else if packet.Rumor.ID == 1 {
		// It's a new message, initialize the vector clock accordingly.
		gossiper.nextIDs[packet.Rumor.Origin] = 2
	} else {
		gossiper.nextIDs[packet.Rumor.Origin] = 1
	}

}

// pickRandomPeer picks a random peer from the list of known peers of the
// gossiper.
func (gossiper *Gossiper) pickRandomPeer() (string, bool) {
	if len(gossiper.Peers) > 0 {
		return gossiper.Peers[rand.Int()%len(gossiper.Peers)], true
	}
	return "", false
}

// rumormonger handles the main logic of the rumormongering protocol. Always
// creates a new go routine.
func (gossiper *Gossiper) rumormonger(packet *messages.GossipPacket, target string) {
	gossiper.sendGossipPacket(target, packet)
	fmt.Println("MONGERING with", target)

	go func() {
		// Set timeout and listen to acknowledgement channel
		timeout := time.NewTicker(10 * time.Second)
		for {
			select {
			case <-timeout.C:
				if target, ok := gossiper.pickRandomPeer(); ok {
					defer gossiper.rumormonger(packet, target)
				}
				return

			case status := <-gossiper.statusWaiting[target]:
				for _, sp := range status.Want {
					if sp.Identifier == packet.Rumor.Origin && sp.NextID > packet.Rumor.ID {
						// Announce that the package is expected
						gossiper.expected[target] <- true
						if gossiper.compareVectors(status, target) && tools.FlipCoin() {
							target, ok := gossiper.pickRandomPeer()
							if ok {
								fmt.Println("FLIPPED COIN sending rumor to", target)
								defer gossiper.rumormonger(packet, target)
							}
						}
						return
					}
				}
			}
		}
	}()

}

// compareVectors establishes the difference between two vector clocks and
// handles the updating logic. Returns true if the vectors are equal.
func (gossiper *Gossiper) compareVectors(status *messages.StatusPacket, target string) bool {
	// 3. If equal, then return immediately.
	if status.IsEqual(gossiper.nextIDs) {
		return true
	}
	ourStatus := gossiper.getCurrentStatus().Status.Want
	theirMap := make(map[string]uint32)
	for _, e := range status.Want {
		theirMap[e.Identifier] = e.NextID
	}
	// 1. Verify if we know something that they don't.
	for _, ourE := range ourStatus {
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
	// 2. Verify if they know something new.
	for _, theirE := range status.Want {
		ourID, ok := gossiper.nextIDs[theirE.Identifier]
		if !ok || ourID < theirE.NextID {
			gossiper.sendCurrentStatus(target)
			return false
		}
	}
	log.Fatal("Undefined behavior in vector comparison")
	return false
}

// rumormongerPastMsg retrieves an older message to broadcast it to another
// peer which does not posess it yet.
func (gossiper *Gossiper) rumormongerPastMsg(origin string, id uint32, target string) {
	ps := messages.PeerStatus{
		Identifier: origin,
		NextID:     id,
	}
	gossiper.rumormonger(gossiper.msgHistory[ps], target)
}

// sendCurrentStatus send the current vector clock as a GossipPacket to the
// mentionned peer.
func (gossiper *Gossiper) sendCurrentStatus(address string) {
	gossiper.sendGossipPacket(address, gossiper.getCurrentStatus())
}

// getCurrentStatus dynamically creates this node's vector clock as a
// GossipPacket.
func (gossiper *Gossiper) getCurrentStatus() *messages.GossipPacket {
	packet := messages.GossipPacket{
		Status: &messages.StatusPacket{
			Want: nil,
		},
	}
	for k, v := range gossiper.nextIDs {
		packet.Status.Want = append(packet.Status.Want, messages.PeerStatus{
			Identifier: k,
			NextID:     v,
		})
	}
	return &packet
}

// getMessage listens to the client and waits for a Message. Returns its string
// content.
func (gossiper *Gossiper) getMessage() string {
	var packet messages.Message
	packetBytes, _ := tools.GetPacketBytes(gossiper.cliConn)

	// Decode packet
	err := protobuf.Decode(packetBytes, &packet)
	tools.Check(err)

	return packet.Text
}

// getGossipPacket listens to other peers and waits for a GossipPacket.
func (gossiper *Gossiper) getGossipPacket() (*messages.GossipPacket, *net.UDPAddr) {
	var packet messages.GossipPacket
	packetBytes, address := tools.GetPacketBytes(gossiper.conn)

	// Decode packet
	err := protobuf.Decode(packetBytes, &packet)
	tools.Check(err)

	return &packet, address
}

// sendSimple sends a simple message to all other peers except the one who just
// forwarded it.
func (gossiper *Gossiper) sendSimple(packet *messages.GossipPacket) {
	// Save previous address, update packet with address and encode it
	fromPeer := packet.Simple.RelayPeerAddr
	packet.Simple.RelayPeerAddr = gossiper.Address
	packetBytes, err := protobuf.Encode(packet)
	tools.Check(err)

	// Send to all peers except the last sender
	for _, address := range gossiper.Peers {
		if address != fromPeer {
			tools.SendPacketBytes(gossiper.conn, address, packetBytes)
		}
	}
}

// sendGossipPacket sends a gossipPacket to the mentionned address.
func (gossiper *Gossiper) sendGossipPacket(address string, packet *messages.GossipPacket) {
	packetBytes, err := protobuf.Encode(packet)
	tools.Check(err)
	tools.SendPacketBytes(gossiper.conn, address, packetBytes)
}

// antiEntropy fires every mentionned seconds to send a StatusPacket to a
// random peer. 0 seconds means that there is no antiEntropy set.
func (gossiper *Gossiper) antiEntropy(delay uint64) {
	ticker := time.NewTicker(time.Duration(delay) * time.Second)
	for {
		select {
		case <-ticker.C:
			target, ok := gossiper.pickRandomPeer()
			if ok {
				gossiper.sendCurrentStatus(target)
			}
		default:
		}
	}
}

// GetLatestRumorMessagesList returns a list of the latest rumor messages.
func (gossiper *Gossiper) GetLatestRumorMessagesList() []*messages.RumorMessage {
	messagesQuantity := len(gossiper.allMessages)
	defer func() {
		gossiper.latestMessageID = messagesQuantity
	}()
	if gossiper.latestMessageID < messagesQuantity {
		return gossiper.allMessages[gossiper.latestMessageID:]
	}
	return nil
}

// GetRumorMessagesList returns the list of all rumor messages.
func (gossiper *Gossiper) GetRumorMessagesList() []*messages.RumorMessage {
	gossiper.latestMessageID = len(gossiper.allMessages)
	return gossiper.allMessages
}
