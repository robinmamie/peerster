package gossiper

import (
	"fmt"
	"time"

	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"
)

// receivedGossip handles any received gossiping message, new or not.
func (gossiper *Gossiper) receivedGossip(g messages.Gossiping, forceResend bool) {

	gossipStatus := messages.PeerStatus{
		Identifier: g.GetOrigin(),
		NextID:     g.GetID(),
	}
	_, present := gossiper.msgHistory.Load(gossipStatus)

	if tlc, ok := g.(*messages.TLCMessage); ok {
		go gossiper.handleTLC(tlc, present)
	}

	// New gossiping message detected
	if !present || forceResend {
		// Add message to history and update vector clock
		gossiper.msgHistory.Store(gossipStatus, g)
		gossiper.updateVectorClock(g, gossipStatus)

		// Rumormonger it
		if target, ok := gossiper.pickRandomPeer(); ok {
			gossiper.rumormonger(g, target)
		}

		// Do not display route rumors on the GUI
		if rumor, ok := g.(*messages.RumorMessage); ok && rumor.Text != "" {
			gossiper.allMessages = append(gossiper.allMessages, &messages.GossipPacket{Rumor: rumor})
		}
	}
}

func (gossiper *Gossiper) handleTLC(tlc *messages.TLCMessage, present bool) {

	if !present {
		toStore := (int)(tlc.ID)
		if tlc.Confirmed != -1 {
			toStore = tlc.Confirmed
		}

		if roundRaw, ok := gossiper.rounds.Load(tlc.Origin); ok {
			round := roundRaw.([]int)
			present := false
			for _, el := range round {
				if el == toStore {
					present = true
				}
			}
			if !present {
				gossiper.rounds.Store(tlc.Origin, append(round, toStore))
			}
		} else {
			gossiper.rounds.Store(tlc.Origin, []int{toStore})
		}
	}

	if tlc.Confirmed == -1 {
		fmt.Println("UNCONFIRMED GOSSIP origin", tlc.Origin, "ID", tlc.ID,
			"file name", tlc.TxBlock.Transaction.Name,
			"size", tlc.TxBlock.Transaction.Size,
			"metahash", tools.BytesToHexString(tlc.TxBlock.Transaction.MetafileHash))

		// Only acknowledge messages if they are from now or the future
		theirTime := 0
		if round, ok := gossiper.rounds.Load(tlc.Origin); ok {
			theirTime = len(round.([]int)) - 1
		}
		if gossiper.Name != tlc.Origin &&
			(!gossiper.hw3ex3 || theirTime >= (int)(gossiper.myTime) || gossiper.ackAll) &&
			(!gossiper.hw3ex4 || gossiper.validate(tlc)) {
			ack := &messages.TLCAck{
				Origin:      gossiper.Name,
				ID:          tlc.ID,
				Text:        "",
				Destination: tlc.Origin,
				HopLimit:    gossiper.hopLimit,
			}
			gossiper.handleAck(ack)
			fmt.Println("SENDING ACK origin", tlc.Origin, "ID", tlc.ID)
		}
	} else {
		fmt.Println("CONFIRMED GOSSIP origin", tlc.Origin, "ID", tlc.ID,
			"file name", tlc.TxBlock.Transaction.Name,
			"size", tlc.TxBlock.Transaction.Size,
			"metahash", tools.BytesToHexString(tlc.TxBlock.Transaction.MetafileHash))
		if !present {
			select {
			case gossiper.roundsUpdated <- tlc:
			}
		}
	}
}

func (gossiper *Gossiper) validate(tlc *messages.TLCMessage) bool {
	// 1st criteria
	for _, b := range gossiper.blockchain {
		if b.Transaction.Name == tlc.TxBlock.Transaction.Name {
			return false
		}
	}
	// 2nd criteria
	ok := true
	var otherChain []string = nil
	hashArray := tlc.TxBlock.Hash()
	hash := tools.BytesToHexString(hashArray[:])
	for ok {
		prev, ok := gossiper.allBlocks.Load(hash)
		if ok {
			hashArray = prev.(*messages.TLCMessage).TxBlock.Hash()
			hash = tools.BytesToHexString(hashArray[:])
			otherChain = append(otherChain, hash)
		}
	}
	for i, b := range gossiper.blockchain[1:] {
		ourHash := tools.BytesToHexString(b.PrevHash[:])
		if ourHash != otherChain[len(otherChain)-1-i] {
			return false
		}
	}
	return true
}

// rumormonger handles the main logic of the rumormongering protocol. Always
// creates a new go routine.
func (gossiper *Gossiper) rumormonger(g messages.Gossiping, target string) {
	packet := g.CreatePacket()
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
					gossiper.rumormonger(g, target)
				}
				return

			case status := <-statusChannel:
				for _, sp := range status.Want {
					if sp.Identifier == g.GetOrigin() && sp.NextID > g.GetID() {
						// Announce that the package is expected
						select {
						case expected <- true:
						default:
						}
						// We have to compare vectors first, in case we/they have somthing interesting.
						// We flip the coin iff we are level. Otherwise, there is no mention of any coin in the specs.
						if gossiper.compareVectors(status, target) && tools.FlipCoin() {
							if target, ok := gossiper.pickRandomPeer(); ok {
								gossiper.rumormonger(g, target)
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
func (gossiper *Gossiper) updateVectorClock(g messages.Gossiping, status messages.PeerStatus) {
	for i, ps := range gossiper.vectorClock.Want {
		if ps.Identifier == g.GetOrigin() {
			if ps.NextID == g.GetID() {
				stillPresent := true
				status := status
				// Verify if a sequence was completed
				for stillPresent {
					status.NextID++
					gossiper.updateMutex.Lock()
					gossiper.vectorClock.Want[i].NextID++
					gossiper.updateMutex.Unlock()
					_, stillPresent = gossiper.msgHistory.Load(status)
				}
				// Signal update
				if gossiper.hw3ex3 {
					update := true
					for update {
						select {
						case gossiper.vectorUpdate <- true:
						default:
							update = false
						}
					}
				}
			}
			// else do not update vector clock, will be done once the sequence
			// is completed
			return
		}
	}
	// It's a new message, initialize the vector clock accordingly.
	ps := messages.PeerStatus{
		Identifier: g.GetOrigin(),
		NextID:     2,
	}
	if g.GetID() != 1 {
		// Got a newer message, still wait on #1
		ps.NextID = 1
	}
	gossiper.updateMutex.Lock()
	gossiper.vectorClock.Want = append(gossiper.vectorClock.Want, ps)
	gossiper.updateMutex.Unlock()

	// Signal update
	if gossiper.hw3ex3 {
		update := true
		for update {
			select {
			case gossiper.vectorUpdate <- true:
			default:
				update = false
			}
		}
	}
}

// updateRoutingTable takes a RumorMessage and updates the routing table
// accordingly.
func (gossiper *Gossiper) updateRoutingTable(g messages.Gossiping, address string) {
	if val, ok := gossiper.maxIDs.Load(g.GetOrigin()); ok && g.GetID() <= val.(uint32) {
		// This is not a newer RumorMessage
		return
	}

	if address == "" {
		// Comes directly from us
		return
	}

	gossiper.maxIDs.Store(g.GetOrigin(), g.GetID())

	if _, ok := gossiper.routingTable.Load(g.GetOrigin()); !ok {
		// For the GUI: add destination to list if previously unknown.
		gossiper.destinationMutex.Lock()
		gossiper.destinationList = append(gossiper.destinationList, g.GetOrigin())
		gossiper.destinationMutex.Unlock()
	}

	gossiper.routingTable.Store(g.GetOrigin(), address)

	// Do not display route rumors!
	if rumor, ok := g.(*messages.RumorMessage); !ok || (ok && rumor.Text != "") {
		fmt.Println("DSDV", g.GetOrigin(), address)
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

	oldGossipRaw, _ := gossiper.msgHistory.Load(ps)
	oldGossip := oldGossipRaw.(messages.Gossiping)
	gossiper.rumormonger(oldGossip, target)
}

// sendCurrentStatus sends the current vector clock as a GossipPacket to the
// mentionned peer.
func (gossiper *Gossiper) sendCurrentStatus(address string) {
	gossiper.updateMutex.Lock()
	gossiper.sendGossipPacket(address, gossiper.getCurrentStatus())
	gossiper.updateMutex.Unlock()
}
