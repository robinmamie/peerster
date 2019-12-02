package gossiper

import (
	"fmt"
	"time"

	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"
)

// receivedRumor handles any received rumor, new or not.
func (gossiper *Gossiper) receivedRumor(rumor *messages.RumorMessage) {

	rumorStatus := messages.PeerStatus{
		Identifier: rumor.Origin,
		NextID:     rumor.ID,
	}
	_, present := gossiper.msgHistory.Load(rumorStatus)

	// New rumor detected
	if !present {
		// Add rumor to history and update vector clock
		gossiper.msgHistory.Store(rumorStatus, rumor)
		gossiper.updateVectorClock(rumor, rumorStatus)

		// Rumormonger it
		if target, ok := gossiper.pickRandomPeer(); ok {
			gossiper.rumormonger(rumor, target)
		}

		// Do not display route rumors on the GUI
		if rumor.Text != "" {
			gossiper.allMessages = append(gossiper.allMessages, &messages.GossipPacket{Rumor: rumor})
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
						select {
						case expected <- true:
						default:
						}
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
	for i, ps := range gossiper.vectorClock.Want {
		if ps.Identifier == rumor.Origin {
			if ps.NextID == rumor.ID {
				stillPresent := true
				status := rumorStatus
				// Verify if a sequence was completed
				for stillPresent {
					status.NextID++
					gossiper.updateMutex.Lock()
					gossiper.vectorClock.Want[i].NextID++
					gossiper.updateMutex.Unlock()
					_, stillPresent = gossiper.msgHistory.Load(status)
				}
			}
			// else do not update vector clock, will be done once the sequence
			// is completed
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
	gossiper.updateMutex.Lock()
	gossiper.vectorClock.Want = append(gossiper.vectorClock.Want, ps)
	gossiper.updateMutex.Unlock()
}

// updateRoutingTable takes a RumorMessage and updates the routing table
// accordingly.
func (gossiper *Gossiper) updateRoutingTable(rumor *messages.RumorMessage, address string) {
	if val, ok := gossiper.maxIDs.Load(rumor.Origin); ok && rumor.ID <= val.(uint32) {
		// This is not a newer RumorMessage
		return
	}

	if address == "" {
		// Comes directly from us
		return
	}

	gossiper.maxIDs.Store(rumor.Origin, rumor.ID)

	if _, ok := gossiper.routingTable.Load(rumor.Origin); !ok {
		// For the GUI: add destination to list if previously unknown.
		gossiper.destinationMutex.Lock()
		gossiper.destinationList = append(gossiper.destinationList, rumor.Origin)
		gossiper.destinationMutex.Unlock()
	}

	gossiper.routingTable.Store(rumor.Origin, address)

	// Do not display route rumors!
	if rumor.Text != "" {
		fmt.Println("DSDV", rumor.Origin, address)
	}
}

// compareVectors establishes the difference between two vector clocks and
// handles the updating logic. Returns true if the vectors are equal.
func (gossiper *Gossiper) compareVectors(status *messages.StatusPacket, target string) bool {
	ourMap := make(map[string]uint32)
	gossiper.updateMutex.Lock()
	for _, e := range gossiper.vectorClock.Want {
		ourMap[e.Identifier] = e.NextID
	}
	gossiper.updateMutex.Unlock()
	// 3. If equal, then return immediately.
	if status.IsEqual(ourMap) {
		return true
	}
	theirMap := make(map[string]uint32)
	for _, e := range status.Want {
		theirMap[e.Identifier] = e.NextID
	}
	// 1. Verify if we know something that they don't.
	gossiper.updateMutex.Lock()
	for _, ourE := range gossiper.vectorClock.Want {
		theirID, ok := theirMap[ourE.Identifier]
		if !ok && 1 < ourE.NextID {
			gossiper.rumormongerPastMsg(ourE.Identifier, 1, target)
			gossiper.updateMutex.Unlock()
			return false
		}
		if ok && theirID < ourE.NextID {
			gossiper.rumormongerPastMsg(ourE.Identifier, theirID, target)
			gossiper.updateMutex.Unlock()
			return false
		}
	}
	gossiper.updateMutex.Unlock()
	// 2. Verify if they know something new.
	for _, theirE := range status.Want {
		ourID, ok := ourMap[theirE.Identifier]
		if !ok || ourID < theirE.NextID {
			gossiper.sendCurrentStatus(target)
			return false
		}
	}
	// Should never happen.
	return true
}

// rumormongerPastMsg retrieves an older message to broadcast it to another
// peer which does not posess it yet. It returns true iff the message was
// indeed present.
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
	gossiper.updateMutex.Lock()
	gossiper.sendGossipPacket(address, gossiper.getCurrentStatus())
	gossiper.updateMutex.Unlock()
}
