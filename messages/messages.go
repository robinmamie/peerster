package messages

// Message is the packet sent by the client to the Gossiper
type Message struct {
	Text string
}

// GossipPacket is used to store different messages and
// are sent between gossipers.
type GossipPacket struct {
	Simple *SimpleMessage
	Rumor  *RumorMessage
	Status *StatusPacket
}

// SimpleMessage stores the information about the original sender,
// the address of the current sender, and the content of the message.
type SimpleMessage struct {
	OriginalName  string
	RelayPeerAddr string
	Contents      string
}

// RumorMessage stores the information about the original sender, the ID of the
// message, and the content of the message.
type RumorMessage struct {
	Origin string
	ID     uint32
	Text   string
}

// PeerStatus stores the ID of the next message expected from the mentionned
// origin.
type PeerStatus struct {
	Identifier string
	NextID     uint32
}

// StatusPacket stores all PeerStatus of a given peer.
type StatusPacket struct {
	Want []PeerStatus
}

// IsEqual verifies if a StatusPacket is equal to a locally implemented vector
// clock.
func (sp StatusPacket) IsEqual(thatMap map[string]uint32) bool {
	this := sp.Want
	if len(this) != len(thatMap) {
		return false
	}
	for _, e := range this {
		id, ok := thatMap[e.Identifier]
		if !ok || id != e.NextID {
			return false
		}
	}
	return true
}
