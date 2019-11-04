package gossiper

import (
	"fmt"
	"log"
	"time"

	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"
)

// receivedRumor handles any received rumor, new or not.
func (gossiper *Gossiper) receivedRumor(rumor *messages.RumorMessage, address string) {

	rumorStatus := messages.PeerStatus{
		Identifier: rumor.Origin,
		NextID:     rumor.ID,
	}
	_, present := gossiper.msgHistory.Load(rumorStatus)

	// New rumor detected
	if !present {
		fmt.Println("RUMOR origin", rumor.Origin, "from",
			address, "ID", rumor.ID, "contents",
			rumor.Text)
		// Add rumor to history and update vector clock atomically
		gossiper.updateMutex.Lock()
		gossiper.msgHistory.Store(rumorStatus, rumor)

		// Do not display route rumors on the GUI
		if rumor.Text != "" {
			gossiper.allMessages = append(gossiper.allMessages, rumor)
		}

		gossiper.updateVectorClock(rumor, rumorStatus)
		gossiper.updateRoutingTable(rumor, address)
		gossiper.updateMutex.Unlock()

		if target, ok := gossiper.pickRandomPeer(); ok {
			gossiper.rumormonger(rumor, target)
		}
	}
}

// rumormonger handles the main logic of the rumormongering protocol. Always
// creates a new go routine.
func (gossiper *Gossiper) rumormonger(rumor *messages.RumorMessage, target string) {
	packet := &messages.GossipPacket{
		Rumor: rumor,
	}
	gossiper.sendGossipPacket(target, packet)

	go func() {
		// Set timeout and listen to acknowledgement channel
		timeout := time.NewTicker(10 * time.Second)
		statusChannelRaw, _ := gossiper.statusWaiting.Load(target)
		statusChannel := statusChannelRaw.(chan *messages.StatusPacket)
		expectedRaw, _ := gossiper.expected.Load(target)
		expected := expectedRaw.(chan bool)
		for {
			select {
			case <-timeout.C:
				if target, ok := gossiper.pickRandomPeer(); ok {
					gossiper.rumormonger(rumor, target)
				}
				return

			case status := <-statusChannel:
				for _, sp := range status.Want {
					if sp.Identifier == packet.Rumor.Origin && sp.NextID > packet.Rumor.ID {
						// Announce that the package is expected
						expected <- true
						// We have to compare vectors first, in case we/they have somthing interesting.
						// We flip the coin iff we are level. Otherwise, there is no mention of any coin in the specs.
						if gossiper.compareVectors(status, target) && tools.FlipCoin() {
							if target, ok := gossiper.pickRandomPeer(); ok {
								gossiper.rumormonger(rumor, target)
							}
						}
						return
					}
				}
			}
		}
	}()
}

// updateVectorClock updates the internal vector clock.
func (gossiper *Gossiper) updateVectorClock(rumor *messages.RumorMessage, rumorStatus messages.PeerStatus) {
	// TODO create own package
	for i, ps := range gossiper.vectorClock.Want {
		if ps.Identifier == rumor.Origin {
			if ps.NextID == rumor.ID {
				stillPresent := true
				status := rumorStatus
				// Verify if a sequence was completed
				for stillPresent {
					status.NextID++
					gossiper.vectorClock.Want[i].NextID++
					_, stillPresent = gossiper.msgHistory.Load(status)
				}
			}
			// else do not update vector clock, will be done once the sequence is completed
			return
		}
	}
	// It's a new message, initialize the vector clock accordingly.
	ps := messages.PeerStatus{
		Identifier: rumor.Origin,
		NextID:     2,
	}
	if rumor.ID != 1 {
		// Got a newer message, still wait on #1
		ps.NextID = 1
	}
	gossiper.vectorClock.Want = append(gossiper.vectorClock.Want, ps)
}

// updateRoutingTable takes a RumorMessage and updates the routing table accordingly.
func (gossiper *Gossiper) updateRoutingTable(rumor *messages.RumorMessage, address string) {
	if val, ok := gossiper.maxIDs.Load(rumor.Origin); ok && rumor.ID <= val.(uint32) {
		// This is not a newer RumorMessage
		return
	}

	gossiper.maxIDs.Store(rumor.Origin, rumor.ID)

	if _, ok := gossiper.routingTable.Load(rumor.Origin); !ok {
		// Add destination to list
		gossiper.DestinationList = append(gossiper.DestinationList, rumor.Origin)
	}

	gossiper.routingTable.Store(rumor.Origin, address)
	if rumor.Text != "" {
		fmt.Println("DSDV", rumor.Origin, address)
	}
}

// compareVectors establishes the difference between two vector clocks and
// handles the updating logic. Returns true if the vectors are equal.
func (gossiper *Gossiper) compareVectors(status *messages.StatusPacket, target string) bool {
	ourMap := make(map[string]uint32)
	for _, e := range gossiper.vectorClock.Want {
		ourMap[e.Identifier] = e.NextID
	}
	// 3. If equal, then return immediately.
	if status.IsEqual(ourMap) {
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
		ourID, ok := ourMap[theirE.Identifier]
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
	oldRumorRaw, _ := gossiper.msgHistory.Load(ps)
	oldRumor := oldRumorRaw.(*messages.RumorMessage)
	gossiper.rumormonger(oldRumor, target)
}

// sendCurrentStatus sends the current vector clock as a GossipPacket to the
// mentionned peer.
func (gossiper *Gossiper) sendCurrentStatus(address string) {
	gossiper.sendGossipPacket(address, gossiper.getCurrentStatus())
}
