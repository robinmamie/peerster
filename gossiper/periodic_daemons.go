package gossiper

import (
	"time"

	"github.com/robinmamie/Peerster/messages"
)

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
		}
	}
}

// routeRumor periodically sends an empty rumor message so that neighbors do not
// forget about this node.
func (gossiper *Gossiper) routeRumor(rtimer uint64) {
	// Startup route rumor
	gossiper.createRouteRumor()

	// Periodic route rumors
	ticker := time.NewTicker(time.Duration(rtimer) * time.Second)
	for {
		select {
		case <-ticker.C:
			gossiper.createRouteRumor()
		}
	}
}

// createRouteRumor creates a route rumor, and then mongers it to a random peer.
func (gossiper *Gossiper) createRouteRumor() {
	packet := &messages.GossipPacket{
		Rumor: &messages.RumorMessage{
			Origin: gossiper.Name,
			ID:     gossiper.getOwnID(),
			Text:   "",
		},
	}
	gossiper.incrementOwnID()
	// Add our own address to the routing table.
	gossiper.receivedRumor(packet.Rumor, gossiper.Address)
}
