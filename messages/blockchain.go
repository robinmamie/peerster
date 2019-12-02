package messages

// TxPublish represents a transaction to be published.
type TxPublish struct {
	Name         string
	Size         int64
	MetafileHash []byte
}

// BlockPublish represents a publication on the blockchain.
type BlockPublish struct {
	PrevHash    [32]byte
	Transaction TxPublish
}

// TLCMessage is the message used for the Threshold Logical Clocks.
type TLCMessage struct {
	Origin      string
	ID          uint32
	Confirmed   int
	TxBlock     BlockPublish
	VectorClock *StatusPacket
	Fitness     float32
}

// TLCAck is used to acknowledge TLC messages
type TLCAck PrivateMessage

// Gossiping represents all gossiping messages (rumor and TLCMessage)
type Gossiping interface {
	GetOrigin() string
	GetID() uint32
	CreatePacket() *GossipPacket
}

// GetOrigin gives the origin of the message
func (r *RumorMessage) GetOrigin() string {
	return r.Origin
}

// GetID gives the ID of the message
func (r *RumorMessage) GetID() uint32 {
	return r.ID
}

// CreatePacket creates a packet from the current message
func (r *RumorMessage) CreatePacket() *GossipPacket {
	return &GossipPacket{
		Rumor: r,
	}
}

// GetOrigin gives the origin of the message
func (tlc *TLCMessage) GetOrigin() string {
	return tlc.Origin
}

// GetID gives the ID of the message
func (tlc *TLCMessage) GetID() uint32 {
	return tlc.ID
}

// CreatePacket creates a packet from the current message
func (tlc *TLCMessage) CreatePacket() *GossipPacket {
	return &GossipPacket{
		TLCMessage: tlc,
	}
}
