package gossiper

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/robinmamie/Peerster/files"

	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"

	"github.com/dedis/protobuf"
)

// Gossiper defines a peer and stores the necessary information to use it.
type Gossiper struct {
	// Net information
	Address string
	conn    *net.UDPConn
	cliConn *net.UDPConn
	UIPort  string
	// Gossiper information
	Name   string
	simple bool
	Peers  []string
	// Channels used to communicate between threads
	statusWaiting map[string]chan *messages.StatusPacket
	expected      map[string]chan bool
	// Message history
	msgHistory      map[messages.PeerStatus]*messages.GossipPacket
	allMessages     []*messages.RumorMessage
	latestMessageID int
	// ID information
	vectorClock map[string]uint32
	maxIDs      map[string]uint32
	ownID       uint32
	// Routing table used for next hops
	routingTable map[string]string
	// Slice of indexed files
	indexedFiles []*files.FileMetadata
	// Lock used to synchronize writing on the vector clock and the history
	updateMutex *sync.Mutex
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
		statusWaiting[p] = make(chan *messages.StatusPacket)
		expected[p] = make(chan bool)
	}

	return &Gossiper{
		Address:       address,
		conn:          udpConn,
		cliConn:       cliConn,
		UIPort:        uiPort,
		Name:          name,
		simple:        simple,
		Peers:         peers,
		statusWaiting: statusWaiting,
		expected:      expected,
		// Map between an identifier and a list of RumorMessages
		msgHistory:      make(map[messages.PeerStatus]*messages.GossipPacket),
		allMessages:     make([]*messages.RumorMessage, 0),
		latestMessageID: 0,
		vectorClock:     make(map[string]uint32),
		maxIDs:          make(map[string]uint32),
		ownID:           1,
		routingTable:    make(map[string]string),
		indexedFiles:    make([]*files.FileMetadata, 0),
		updateMutex:     &sync.Mutex{},
	}
}

// Run starts the node and runs it.
func (gossiper *Gossiper) Run(antiEntropyDelay uint64, rtimer uint64) {
	// Activate anti-entropy if necessary
	if !gossiper.simple {
		if antiEntropyDelay > 0 {
			go gossiper.antiEntropy(antiEntropyDelay)
		}
		if rtimer > 0 {
			go gossiper.routeRumor(rtimer)
		}
	}

	go gossiper.listenClient()
	gossiper.listen()
}

// listenClient handles the messages coming from the client.
func (gossiper *Gossiper) listenClient() {
	for {
		message := gossiper.getMessage()
		fmt.Println("CLIENT MESSAGE", message.Text)

		if gossiper.simple {
			packet := &messages.GossipPacket{
				Simple: &messages.SimpleMessage{
					OriginalName:  gossiper.Name,
					RelayPeerAddr: gossiper.Address,
					Contents:      message.Text,
				},
			}
			gossiper.sendSimple(packet)
		} else {
			if *message.Destination != "" {
				packet := &messages.GossipPacket{
					Private: &messages.PrivateMessage{
						Origin:      gossiper.Name,
						ID:          0,
						Text:        message.Text,
						Destination: *message.Destination,
						HopLimit:    9, // The source already decrements it.
					},
				}
				if address, ok := gossiper.routingTable[packet.Private.Destination]; ok {
					gossiper.sendGossipPacket(address, packet)
				}
			} else if *message.File != "" {
				if *message.Destination != "" {

				} else {
					gossiper.indexedFiles = append(gossiper.indexedFiles,
						files.NewFileMetadata(*message.File))
				}
			} else {
				packet := &messages.GossipPacket{
					Rumor: &messages.RumorMessage{
						Origin: gossiper.Name,
						ID:     gossiper.ownID,
						Text:   message.Text,
					},
				}
				// TODO use another lock?
				gossiper.updateMutex.Lock()
				gossiper.ownID++
				gossiper.updateMutex.Unlock()
				gossiper.receivedRumor(packet, "")
			}
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
			gossiper.AddPeer(addressTxt)
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
		} else if !gossiper.simple {
			if packet.Rumor != nil {
				// Received a rumor
				fmt.Println("RUMOR origin", packet.Rumor.Origin, "from",
					address, "ID", packet.Rumor.ID, "contents",
					packet.Rumor.Text)
				gossiper.printPeers()

				gossiper.receivedRumor(packet, addressTxt)
				gossiper.sendCurrentStatus(addressTxt)

			} else if packet.Status != nil {
				// Received a status message
				fmt.Print("STATUS from ", address)
				for _, s := range packet.Status.Want {
					fmt.Print(" peer ", s.Identifier, " nextID ", s.NextID)
				}
				fmt.Println()
				gossiper.printPeers()
				if packet.Status.IsEqual(gossiper.vectorClock) {
					fmt.Println("IN SYNC WITH", addressTxt)
				}

				// Wake up correct subroutine if status received
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
			} else {
				switch {
				case packet.Private != nil:
					if gossiper.ptpMessageReachedDestination(packet.Private) {
						fmt.Println("PRIVATE origin", packet.Private.Origin,
							"hop-limit", packet.Private.HopLimit,
							"contents", packet.Private.Text)
					}
					// TODO ! should also write peers?
					// TODO !! what to do for the GUI? Other list? Same list but with GossipPacket and then the server handles the differences with a switch?
				}
			}
		}
	}
}

func (gossiper *Gossiper) ptpMessageReachedDestination(ptpMessage messages.PointToPoint) bool {
	if ptpMessage.GetDestination() == gossiper.Name {
		return true
	}
	// TODO combine 2 interface functions (get/decrement hoplimit) in 1?
	if ptpMessage.GetHopLimit() > 0 {
		ptpMessage.DecrementHopLimit()
		gossiper.sendGossipPacket(gossiper.routingTable[ptpMessage.GetDestination()], ptpMessage.CreatePacket())
	}
	return false
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
func (gossiper *Gossiper) receivedRumor(packet *messages.GossipPacket, address string) {

	rumorStatus := messages.PeerStatus{
		Identifier: packet.Rumor.Origin,
		NextID:     packet.Rumor.ID,
	}
	_, present := gossiper.msgHistory[rumorStatus]

	// New rumor detected
	if !present {
		// Add rumor to history and update vector clock atomically
		gossiper.updateMutex.Lock()
		gossiper.msgHistory[rumorStatus] = packet
		// Do not display route rumors on the GUI
		if packet.Rumor.Text != "" {
			gossiper.allMessages = append(gossiper.allMessages, packet.Rumor)
		}

		// TODO give rumors as arguments, not packets
		gossiper.updateVectorClock(packet, rumorStatus)
		gossiper.updateRoutingTable(packet, address)
		gossiper.updateMutex.Unlock()

		if target, ok := gossiper.pickRandomPeer(); ok {
			gossiper.rumormonger(packet, target)
		}
	}
}

// updateVectorClock updates the internal vector clock.
func (gossiper *Gossiper) updateVectorClock(packet *messages.GossipPacket, rumorStatus messages.PeerStatus) {
	if val, ok := gossiper.vectorClock[packet.Rumor.Origin]; ok {
		if val == packet.Rumor.ID {
			stillPresent := true
			status := rumorStatus
			// Verify if a sequence was completed
			for stillPresent {
				status.NextID++
				gossiper.vectorClock[packet.Rumor.Origin]++
				_, stillPresent = gossiper.msgHistory[status]
			}
		}
		// else do not update vector clock, will be done once the sequence is completed

	} else if packet.Rumor.ID == 1 {
		// It's a new message, initialize the vector clock accordingly.
		gossiper.vectorClock[packet.Rumor.Origin] = 2
	} else {
		// Got a newer message, still wait on #1
		gossiper.vectorClock[packet.Rumor.Origin] = 1
	}
}

// updateRoutingTable takes a RumorMessage and updates the routing table accordingly.
func (gossiper *Gossiper) updateRoutingTable(packet *messages.GossipPacket, address string) {
	if address == "" {
		// The request comes from the client listener, ignore
		return
	}
	if val, ok := gossiper.maxIDs[packet.Rumor.Origin]; ok {
		if packet.Rumor.ID <= val {
			return
		}
		gossiper.maxIDs[packet.Rumor.Origin] = packet.Rumor.ID
	} else {
		gossiper.maxIDs[packet.Rumor.Origin] = packet.Rumor.ID
		gossiper.routingTable[packet.Rumor.Origin] = address
	}
	// TODO should also take 1st address into account (is it an update?)
	if address != gossiper.routingTable[packet.Rumor.Origin] {
		gossiper.routingTable[packet.Rumor.Origin] = address
	}
	if packet.Rumor.Text != "" {
		fmt.Println("DSDV", packet.Rumor.Origin, address)
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
							if target, ok := gossiper.pickRandomPeer(); ok {
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
	if status.IsEqual(gossiper.vectorClock) {
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
		ourID, ok := gossiper.vectorClock[theirE.Identifier]
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
	for k, v := range gossiper.vectorClock {
		packet.Status.Want = append(packet.Status.Want, messages.PeerStatus{
			Identifier: k,
			NextID:     v,
		})
	}
	return &packet
}

// getMessage listens to the client and waits for a Message.
func (gossiper *Gossiper) getMessage() *messages.Message {
	var packet messages.Message
	packetBytes, _ := tools.GetPacketBytes(gossiper.cliConn)

	// Decode packet
	err := protobuf.Decode(packetBytes, &packet)
	tools.Check(err)

	return &packet
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
			if target, ok := gossiper.pickRandomPeer(); ok {
				gossiper.sendCurrentStatus(target)
			}
		default: // FIXME necessary?
		}
	}
}

// routeRumor periodically sends an empty rumor message so that neighbors do not
// forget about this node.
func (gossiper *Gossiper) routeRumor(rtimer uint64) {
	// Startup route rumors
	for _, p := range gossiper.Peers {
		gossiper.sendRouteRumor(p)
	}

	// Periodic route rumors
	ticker := time.NewTicker(time.Duration(rtimer) * time.Second)
	for {
		select {
		case <-ticker.C:
			if target, ok := gossiper.pickRandomPeer(); ok {
				gossiper.sendRouteRumor(target)
			}
		}
	}
}

func (gossiper *Gossiper) sendRouteRumor(target string) {
	packet := &messages.GossipPacket{
		Rumor: &messages.RumorMessage{
			Origin: gossiper.Name,
			ID:     gossiper.ownID,
			Text:   "",
		},
	}
	gossiper.updateMutex.Lock()
	gossiper.ownID++
	gossiper.updateMutex.Unlock()
	gossiper.receivedRumor(packet, target)
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

// AddPeer adds a new known peer and its logic to the gossiper.
func (gossiper *Gossiper) AddPeer(address string) {
	gossiper.Peers = append(gossiper.Peers, address)
	// TODO modularize this (see constructor)
	gossiper.statusWaiting[address] = make(chan *messages.StatusPacket)
	gossiper.expected[address] = make(chan bool)
}
