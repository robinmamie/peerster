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
	gossiper.receivedRumor(packet.Rumor, target)
}
