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
	_, present := gossiper.msgHistory[rumorStatus]

	// New rumor detected
	if !present {
		// Add rumor to history and update vector clock atomically
		gossiper.updateMutex.Lock()
		gossiper.msgHistory[rumorStatus] = rumor

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
	fmt.Println("MONGERING with", target)

	go func() {
		// Set timeout and listen to acknowledgement channel
		timeout := time.NewTicker(10 * time.Second)
		for {
			select {
			case <-timeout.C:
				if target, ok := gossiper.pickRandomPeer(); ok {
					defer gossiper.rumormonger(rumor, target)
				}
				return

			case status := <-gossiper.statusWaiting[target]:
				for _, sp := range status.Want {
					if sp.Identifier == packet.Rumor.Origin && sp.NextID > packet.Rumor.ID {
						// Announce that the package is expected
						gossiper.expected[target] <- true
						// We have to compare vectors first, in case we/they have somthing interesting.
						// We flip the coin iff we are level. Otherwise, there is no mention of any coin in the specs.
						if gossiper.compareVectors(status, target) && tools.FlipCoin() {
							if target, ok := gossiper.pickRandomPeer(); ok {
								fmt.Println("FLIPPED COIN sending rumor to", target)
								defer gossiper.rumormonger(rumor, target)
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
	if val, ok := gossiper.vectorClock[rumor.Origin]; ok {
		if val == rumor.ID {
			stillPresent := true
			status := rumorStatus
			// Verify if a sequence was completed
			for stillPresent {
				status.NextID++
				gossiper.vectorClock[rumor.Origin]++
				_, stillPresent = gossiper.msgHistory[status]
			}
		}
		// else do not update vector clock, will be done once the sequence is completed

	} else if rumor.ID == 1 {
		// It's a new message, initialize the vector clock accordingly.
		gossiper.vectorClock[rumor.Origin] = 2
	} else {
		// Got a newer message, still wait on #1
		gossiper.vectorClock[rumor.Origin] = 1
	}
}

// updateRoutingTable takes a RumorMessage and updates the routing table accordingly.
func (gossiper *Gossiper) updateRoutingTable(rumor *messages.RumorMessage, address string) {
	if address == "" {
		// The request comes from the client listener, ignore
		return
	}
	if val, ok := gossiper.maxIDs[rumor.Origin]; ok && rumor.ID <= val {
		// This is not a newer RumorMessage
		return
	}

	gossiper.maxIDs[rumor.Origin] = rumor.ID

	if dest, ok := gossiper.routingTable.Load(rumor.Origin); !ok || address != dest.(string) {
		gossiper.routingTable.Store(rumor.Origin, address)
	}
	if rumor.Text != "" {
		fmt.Println("DSDV", rumor.Origin, address)
	}
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

// sendCurrentStatus sends the current vector clock as a GossipPacket to the
// mentionned peer.
func (gossiper *Gossiper) sendCurrentStatus(address string) {
	gossiper.sendGossipPacket(address, gossiper.getCurrentStatus())
}
