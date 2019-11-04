package gossiper

import (
	"math/rand"
	"net"
	"sync"

	"github.com/robinmamie/Peerster/files"

	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"
)

// Gossiper defines a peer and stores the necessary information to use it.
type Gossiper struct {
	// Net information
	Address string
	conn    *net.UDPConn
	cliConn *net.UDPConn
	UIPort  string
	// Gossiper information
	Name   string
	simple bool
	Peers  []string
	// Channels used to communicate between threads
	statusWaiting sync.Map
	expected      sync.Map
	dataChannels  sync.Map
	// Message history
	msgHistory      sync.Map
	allMessages     []*messages.RumorMessage // Used for the GUI
	PrivateMessages sync.Map
	latestMessageID int
	fileChunks      sync.Map
	// ID information
	vectorClock *messages.StatusPacket
	maxIDs      sync.Map
	ownID       uint32
	// Routing table used for next hops
	routingTable    sync.Map
	DestinationList []string
	// Slice of indexed files
	// TODO Could we simply map from FileHash to the MetaHash? ([]byte to []byte)
	indexedFiles []*files.FileMetadata
	// Lock used to synchronize writing on the vector clock and the history
	updateMutex *sync.Mutex
}

// NewGossiper creates a Gossiper with a given address, name, port, mode and
// list of peers.
func NewGossiper(address, name string, uiPort string, simple bool, peers []string) *Gossiper {
	// Creation of all necessary UDP sockets.
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	tools.Check(err)
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	tools.Check(err)

	cliAddr, err := net.ResolveUDPAddr("udp4", ":"+uiPort)
	tools.Check(err)
	cliConn, err := net.ListenUDP("udp4", cliAddr)
	tools.Check(err)

	// Channels used to communicate between routines during rumormongering.
	emptyStatus := &messages.StatusPacket{
		Want: nil,
	}

	gossiper := &Gossiper{
		Address:         address,
		conn:            udpConn,
		cliConn:         cliConn,
		UIPort:          uiPort,
		Name:            name,
		simple:          simple,
		Peers:           peers,
		allMessages:     make([]*messages.RumorMessage, 0),
		latestMessageID: 0,
		vectorClock:     emptyStatus,
		ownID:           1,
		DestinationList: make([]string, 0),
		indexedFiles:    make([]*files.FileMetadata, 0),
		updateMutex:     &sync.Mutex{},
	}

	// Create maps for inter-thread communications.
	for _, p := range peers {
		gossiper.statusWaiting.Store(p, make(chan *messages.StatusPacket))
		gossiper.expected.Store(p, make(chan bool))
	}

	return gossiper
}

// Run starts the node and runs it.
func (gossiper *Gossiper) Run(antiEntropyDelay uint64, rtimer uint64) {
	// Activate anti-entropy/route rumors if necessary
	if !gossiper.simple {
		if antiEntropyDelay > 0 {
			go gossiper.antiEntropy(antiEntropyDelay)
		}
		if rtimer > 0 {
			go gossiper.routeRumor(rtimer)
		}
	}

	go gossiper.listenClient()
	gossiper.listenGossiper()
}

// pickRandomPeer picks a random peer from the list of known peers of the
// gossiper.
func (gossiper *Gossiper) pickRandomPeer() (string, bool) {
	if len(gossiper.Peers) > 0 {
		return gossiper.Peers[rand.Int()%len(gossiper.Peers)], true
	}
	return "", false
}

// getCurrentStatus dynamically creates this node's vector clock as a
// GossipPacket.
func (gossiper *Gossiper) getCurrentStatus() *messages.GossipPacket {
	// TODO suboptimal, will take too much time!
	packet := messages.GossipPacket{
		Status: gossiper.vectorClock,
	}
	return &packet
}

func (gossiper *Gossiper) incrementOwnID() {
	gossiper.updateMutex.Lock()
	gossiper.ownID++
	gossiper.updateMutex.Unlock()
}
