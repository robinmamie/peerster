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
			go gossiper.handleSimple(packet.Simple)
		case packet.Rumor != nil:
			go gossiper.handleRumor(packet.Rumor, address)
		case packet.Status != nil:
			go gossiper.handleStatus(packet.Status, address)
		case packet.Private != nil:
			go gossiper.handlePrivate(packet.Private)
		case packet.DataRequest != nil:
			go gossiper.handleDataRequest(packet.DataRequest, "", -1)
		case packet.DataReply != nil:
			go gossiper.handleDataReply(packet.DataReply)
		case packet.SearchRequest != nil:
			go gossiper.handleSearchRequest(packet.SearchRequest, address)
		case packet.SearchReply != nil:
			go gossiper.handleSearchReply(packet.SearchReply)
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
