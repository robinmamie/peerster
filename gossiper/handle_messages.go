package gossiper

import (
	"fmt"
	"strings"
	"time"

	"github.com/robinmamie/Peerster/messages"
)

func (gossiper *Gossiper) handleSimple(simple *messages.SimpleMessage) {
	fmt.Println("SIMPLE MESSAGE origin", simple.OriginalName,
		"from", simple.RelayPeerAddr,
		"contents", simple.Contents)
	gossiper.printPeers()

	// Send packet to all other known peers if we are in simple mode
	if gossiper.simple {
		gossiper.sendSimple(simple)
	}
}

func (gossiper *Gossiper) handleRumor(rumor *messages.RumorMessage, address string) {
	fmt.Println("RUMOR origin", rumor.Origin, "from",
		address, "ID", rumor.ID, "contents",
		rumor.Text)
	gossiper.printPeers()

	gossiper.receivedRumor(rumor, address)
	gossiper.sendCurrentStatus(address)
}

func (gossiper *Gossiper) handleStatus(sp *messages.StatusPacket, address string) {
	fmt.Print("STATUS from ", address)
	for _, s := range sp.Want {
		fmt.Print(" peer ", s.Identifier, " nextID ", s.NextID)
	}
	fmt.Println()
	gossiper.printPeers()
	if sp.IsEqual(gossiper.vectorClock) {
		fmt.Println("IN SYNC WITH", address)
	}

	// Wake up correct subroutine if status received
	unexpected := true
	for target, channel := range gossiper.statusWaiting {

		if target == address {
			// Empty expected channel before
			for len(gossiper.expected[target]) > 0 {
				<-gossiper.expected[target]
			}

			// Send packet to correct channel, as many times as possible
			listening := true
			for listening {
				select {
				case channel <- sp:
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
		gossiper.compareVectors(sp, address)
	}
}

func (gossiper *Gossiper) handlePrivate(pm *messages.PrivateMessage) {
	// TODO ! should also write peers?
	// TODO !! what to do for the GUI? Other list? Same list but with GossipPacket and then the server handles the differences with a switch?
	if gossiper.ptpMessageReachedDestination(pm) {
		fmt.Println("PRIVATE origin", pm.Origin,
			"hop-limit", pm.HopLimit,
			"contents", pm.Text)
	}
}

func (gossiper *Gossiper) ptpMessageReachedDestination(ptpMessage messages.PointToPoint) bool {
	if ptpMessage.GetDestination() == gossiper.Name {
		return true
	}
	// TODO combine 2 interface functions (get/decrement hoplimit) in 1?
	if ptpMessage.GetHopLimit() > 0 {
		ptpMessage.DecrementHopLimit()
		// It will crash if not known. It should never happen.
		gossiper.sendGossipPacket(gossiper.routingTable[ptpMessage.GetDestination()], ptpMessage.CreatePacket())
	}
	return false
}

// printPeers prints the list of known peers.
func (gossiper *Gossiper) printPeers() {
	fmt.Println("PEERS", strings.Join(gossiper.Peers, ","))
}
