package gossiper

// listenGossiper handles all the messages coming from other gossipers.
func (gossiper *Gossiper) listenGossiper() {
	for {
		packet, address := gossiper.getGossipPacket()

		// Parse address accordingly
		if gossiper.simple {
			if packet.Simple == nil {
				// Ignore any unexpected package
				continue
			}
			address = packet.Simple.RelayPeerAddr
		}

		// Add sender to known peers
		if gossiper.isSenderAbsent(address) {
			gossiper.AddPeer(address)
		}

		switch {
		case packet.Simple != nil:
			gossiper.handleSimple(packet.Simple)
		case packet.Rumor != nil:
			gossiper.handleRumor(packet.Rumor, address)
		case packet.Status != nil:
			gossiper.handleStatus(packet.Status, address)
		case packet.Private != nil:
			gossiper.handlePrivate(packet.Private)
		case packet.DataRequest != nil:
			gossiper.handleDataRequest(packet.DataRequest)
		case packet.DataReply != nil:
			gossiper.handleDataReply(packet.DataReply)
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
