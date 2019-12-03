package messages

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/robinmamie/Peerster/tools"
)

// TxPublish represents a transaction to be published.
type TxPublish struct {
	Name         string
	Size         int64
	MetafileHash []byte
}

// Equals tests the equality of 2 TxPublish
func (t TxPublish) Equals(that TxPublish) bool {
	return t.Name == that.Name &&
		t.Size == that.Size &&
		tools.BytesToHexString(t.MetafileHash) == tools.BytesToHexString(that.MetafileHash)
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

// Equals checks whether 2 TLCMessages are the same.
func (tlc *TLCMessage) Equals(other *TLCMessage) bool {
	return tlc.ID == other.ID &&
		tlc.Origin == other.Origin
}

// TLCAck is used to acknowledge TLC messages
type TLCAck PrivateMessage

// GetDestination gives the destination of the message
func (ack *TLCAck) GetDestination() string {
	return ack.Destination
}

// GetHopLimit gives the current HopLimit of the message
func (ack *TLCAck) GetHopLimit() uint32 {
	return ack.HopLimit
}

// DecrementHopLimit decrements the current HopLimit of the message
func (ack *TLCAck) DecrementHopLimit() {
	ack.HopLimit--
}

// CreatePacket creates a packet from the current message
func (ack *TLCAck) CreatePacket() *GossipPacket {
	return &GossipPacket{
		Ack: ack,
	}
}

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

// Hash of a BlockPublish
func (b *BlockPublish) Hash() (out [32]byte) {
	h := sha256.New()
	h.Write(b.PrevHash[:])
	th := b.Transaction.Hash()
	h.Write(th[:])
	copy(out[:], h.Sum(nil))
	return
}

// Hash of a TxPublish
func (t *TxPublish) Hash() (out [32]byte) {
	h := sha256.New()
	binary.Write(h, binary.LittleEndian,
		uint32(len(t.Name)))
	h.Write([]byte(t.Name))
	h.Write(t.MetafileHash)
	copy(out[:], h.Sum(nil))
	return
}
