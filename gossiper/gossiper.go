package gossiper

import (
	"math/rand"
	"net"
	"sync"

	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"
)

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
	hw3ex2 bool
	hw3ex3 bool
	ackAll bool
	Peers  []string
	// Network information
	n               uint64
	stubbornTimeout uint64
	hopLimit        uint32
	// Channels used to communicate between threads
	statusWaiting        sync.Map
	expected             sync.Map
	dataChannels         sync.Map
	searchRequestLookup  chan *messages.SearchRequest
	searchReply          chan *messages.SearchReply
	searchRequestTimeout chan bool
	searchFinished       chan bool
	ackBlock             chan *messages.TLCAck
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
	indexedFiles     sync.Map
	fileChunks       sync.Map
	fileDestinations sync.Map
	// Lock used to synchronize writing on the vector clock and the history
	updateMutex      *sync.Mutex
	peerMutex        *sync.Mutex
	idMutex          *sync.Mutex
	destinationMutex *sync.Mutex
	// TLC
	myTime uint32
}

// NewGossiper creates a Gossiper with a given address, name, port, mode and
// list of peers.
func NewGossiper(address, name string, uiPort string, peers []string, n uint64,
	stubbornTimeout uint64, hopLimit uint32, simple bool, hw3ex2 bool, hw3ex3 bool, ackAll bool) *Gossiper {
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
		Address:              address,
		conn:                 udpConn,
		cliConn:              cliConn,
		UIPort:               uiPort,
		Name:                 name,
		simple:               simple,
		hw3ex2:               hw3ex2,
		hw3ex3:               hw3ex3,
		ackAll:               ackAll,
		n:                    n,
		stubbornTimeout:      stubbornTimeout,
		hopLimit:             hopLimit,
		searchRequestLookup:  make(chan *messages.SearchRequest),
		searchReply:          make(chan *messages.SearchReply),
		searchRequestTimeout: make(chan bool),
		searchFinished:       make(chan bool),
		ackBlock:             make(chan *messages.TLCAck),
		allMessages:          make([]*messages.GossipPacket, 0),
		latestMessageID:      0,
		vectorClock:          &messages.StatusPacket{Want: nil},
		ownID:                1,
		destinationList:      make([]string, 0),
		updateMutex:          &sync.Mutex{},
		peerMutex:            &sync.Mutex{},
		idMutex:              &sync.Mutex{},
		destinationMutex:     &sync.Mutex{},
		myTime:               0,
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
	gossiper.peerMutex.Lock()
	if len(gossiper.Peers) > 0 {
		peer := gossiper.Peers[rand.Int()%len(gossiper.Peers)]
		gossiper.peerMutex.Unlock()
		return peer, true
	}
	gossiper.peerMutex.Unlock()
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

// incrementOwnID atomically reads the ownID of the Gossiper.
func (gossiper *Gossiper) getAndIncrementOwnID() uint32 {
	gossiper.idMutex.Lock()
	id := gossiper.ownID
	gossiper.ownID++
	gossiper.idMutex.Unlock()
	return id
}

func (gossiper *Gossiper) getRandomPeerList(sender string) []string {
	randomOrder := rand.Perm(len(gossiper.Peers))
	var randomPeerList []string = nil
	for _, v := range randomOrder {
		el := gossiper.Peers[v]
		if el != sender {
			randomPeerList = append(randomPeerList, el)
		}
	}
	return randomPeerList
}
