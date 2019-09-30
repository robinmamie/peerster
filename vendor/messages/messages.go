package messages

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
