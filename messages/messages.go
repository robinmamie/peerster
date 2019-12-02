package messages

// Message is the packet sent by the client to the Gossiper
type Message struct {
	Text        string
	Destination *string
	File        *string
	Request     *[]byte
	Keywords    *[]string
	Budget      uint64
}

// GossipPacket is used to store different messages and
// are sent between gossipers.
type GossipPacket struct {
	Simple        *SimpleMessage
	Rumor         *RumorMessage
	Status        *StatusPacket
	Private       *PrivateMessage
	DataRequest   *DataRequest
	DataReply     *DataReply
	SearchRequest *SearchRequest
	SearchReply   *SearchReply
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

// SearchRequest represents a search request to be processed.
type SearchRequest struct {
	Origin   string
	Budget   uint64
	Keywords []string
}

// SearchResult contains the results of a search
type SearchResult struct {
	FileName     string
	MetafileHash []byte
	ChunkMap     []uint64
	ChunkCount   uint64
}

// PointToPoint represents all point to point messages. It is used to handle
// the forwarding logic generically.
type PointToPoint interface {
	GetDestination() string
	GetHopLimit() uint32
	DecrementHopLimit()
	CreatePacket() *GossipPacket
}

// PrivateMessage is sent between nodes wanting to share private content
type PrivateMessage struct {
	Origin      string
	ID          uint32
	Text        string
	Destination string
	HopLimit    uint32
}

// GetDestination gives the destination of the message
func (pm *PrivateMessage) GetDestination() string {
	return pm.Destination
}

// GetHopLimit gives the current HopLimit of the message
func (pm *PrivateMessage) GetHopLimit() uint32 {
	return pm.HopLimit
}

// DecrementHopLimit decrements the current HopLimit of the message
func (pm *PrivateMessage) DecrementHopLimit() {
	pm.HopLimit--
}

// CreatePacket creates a packet from the current message
func (pm *PrivateMessage) CreatePacket() *GossipPacket {
	return &GossipPacket{
		Private: pm,
	}
}

// DataRequest handles chunk and metafile requests
type DataRequest struct {
	Origin      string
	Destination string
	HopLimit    uint32
	HashValue   []byte
}

// GetDestination gives the destination of the message
func (dr *DataRequest) GetDestination() string {
	return dr.Destination
}

// GetHopLimit gives the current HopLimit of the message
func (dr *DataRequest) GetHopLimit() uint32 {
	return dr.HopLimit
}

// DecrementHopLimit decrements the current HopLimit of the message
func (dr *DataRequest) DecrementHopLimit() {
	dr.HopLimit--
}

// CreatePacket creates a packet from the current message
func (dr *DataRequest) CreatePacket() *GossipPacket {
	return &GossipPacket{
		DataRequest: dr,
	}
}

// DataReply handles chunk and metafile replies
type DataReply struct {
	Origin      string
	Destination string
	HopLimit    uint32
	HashValue   []byte
	Data        []byte
}

// GetDestination gives the destination of the message
func (dr *DataReply) GetDestination() string {
	return dr.Destination
}

// GetHopLimit gives the current HopLimit of the message
func (dr *DataReply) GetHopLimit() uint32 {
	return dr.HopLimit
}

// DecrementHopLimit decrements the current HopLimit of the message
func (dr *DataReply) DecrementHopLimit() {
	dr.HopLimit--
}

// CreatePacket creates a packet from the current message
func (dr *DataReply) CreatePacket() *GossipPacket {
	return &GossipPacket{
		DataReply: dr,
	}
}

// SearchReply answers to a SearchRequest
type SearchReply struct {
	Origin      string
	Destination string
	HopLimit    uint32
	Results     []*SearchResult
}

// GetDestination gives the destination of the message
func (sr *SearchReply) GetDestination() string {
	return sr.Destination
}

// GetHopLimit gives the current HopLimit of the message
func (sr *SearchReply) GetHopLimit() uint32 {
	return sr.HopLimit
}

// DecrementHopLimit decrements the current HopLimit of the message
func (sr *SearchReply) DecrementHopLimit() {
	sr.HopLimit--
}

// CreatePacket creates a packet from the current message
func (sr *SearchReply) CreatePacket() *GossipPacket {
	return &GossipPacket{
		SearchReply: sr,
	}
}
