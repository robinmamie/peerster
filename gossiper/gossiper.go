package gossiper

import (
	"math/rand"
	"net"
	"sync"

	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"
)

// hopLimit defines the maximum number of hops.
const hopLimit uint32 = 10

// dataRequestTimeout sets the timeout of a data request packet
const dataRequestTimeout int = 5

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
	allMessages     []*messages.GossipPacket // Used for the GUI
	latestMessageID int                      // Used for the GUI
	// ID information
	vectorClock *messages.StatusPacket
	maxIDs      sync.Map
	ownID       uint32
	// Routing table used for next hops
	routingTable        sync.Map
	destinationList     []string // Used for the GUI
	latestDestinationID int      // Used for the GUI
	// File logic (files indexed and all chunks)
	indexedFiles sync.Map
	fileChunks   sync.Map
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

	gossiper := &Gossiper{
		Address:         address,
		conn:            udpConn,
		cliConn:         cliConn,
		UIPort:          uiPort,
		Name:            name,
		simple:          simple,
		allMessages:     make([]*messages.GossipPacket, 0),
		latestMessageID: 0,
		vectorClock:     &messages.StatusPacket{Want: nil},
		ownID:           1,
		destinationList: make([]string, 0),
		updateMutex:     &sync.Mutex{},
	}

	// Create peers (and channels for inter-thread communications).
	for _, p := range peers {
		gossiper.AddPeer(p)
	}

	return gossiper
}

// Run starts the node and runs it.
func (gossiper *Gossiper) Run(antiEntropyDelay uint64, rtimer uint64) {
	// Activate anti-entropy/route rumors if necessary.
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
	gossiper.updateMutex.Lock()
	if len(gossiper.Peers) > 0 {
		peer := gossiper.Peers[rand.Int()%len(gossiper.Peers)]
		gossiper.updateMutex.Unlock()
		return peer, true
	}
	gossiper.updateMutex.Unlock()
	return "", false
}

// getCurrentStatus dynamically creates this node's vector clock as a
// GossipPacket.
func (gossiper *Gossiper) getCurrentStatus() *messages.GossipPacket {
	packet := &messages.GossipPacket{
		Status: gossiper.vectorClock,
	}
	return packet
}

// incrementOwnID atomically increments the ownID of the Gossiper.
func (gossiper *Gossiper) incrementOwnID() {
	gossiper.updateMutex.Lock()
	gossiper.ownID++
	gossiper.updateMutex.Unlock()
}
