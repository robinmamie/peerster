package gossiper

import (
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/robinmamie/Peerster/files"
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
	hw3ex4 bool
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
	vectorUpdate         chan bool
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
	myTime                 uint32
	gossipWithConfirmation chan bool
	rounds                 sync.Map
	roundsUpdated          chan *messages.TLCMessage
	gossipWC               bool
	canGossipWC            chan bool
	// Blockchain
	blockchain []messages.BlockPublish
	allBlocks  sync.Map
	bestBlock  *messages.TLCMessage
}

// NewGossiper creates a Gossiper with a given address, name, port, mode and
// list of peers.
func NewGossiper(address, name string, uiPort string, peers []string, n uint64,
	stubbornTimeout uint64, hopLimit uint32, simple bool, hw3ex2 bool,
	hw3ex3 bool, hw3ex4 bool, ackAll bool) *Gossiper {
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
		Address:                address,
		conn:                   udpConn,
		cliConn:                cliConn,
		UIPort:                 uiPort,
		Name:                   name,
		simple:                 simple,
		hw3ex2:                 hw3ex2,
		hw3ex3:                 hw3ex3,
		hw3ex4:                 hw3ex4,
		ackAll:                 ackAll,
		n:                      n,
		stubbornTimeout:        stubbornTimeout,
		hopLimit:               hopLimit,
		searchRequestLookup:    make(chan *messages.SearchRequest),
		searchReply:            make(chan *messages.SearchReply),
		searchRequestTimeout:   make(chan bool),
		searchFinished:         make(chan bool),
		ackBlock:               make(chan *messages.TLCAck),
		vectorUpdate:           make(chan bool),
		allMessages:            make([]*messages.GossipPacket, 0),
		latestMessageID:        0,
		vectorClock:            &messages.StatusPacket{Want: nil},
		ownID:                  1,
		destinationList:        make([]string, 0),
		updateMutex:            &sync.Mutex{},
		peerMutex:              &sync.Mutex{},
		idMutex:                &sync.Mutex{},
		destinationMutex:       &sync.Mutex{},
		myTime:                 0,
		gossipWithConfirmation: make(chan bool),
		roundsUpdated:          make(chan *messages.TLCMessage),
		gossipWC:               false,
		canGossipWC:            make(chan bool),
		blockchain:             make([]messages.BlockPublish, 0),
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
		if gossiper.hw3ex3 {
			go gossiper.roundCounting()
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

func (gossiper *Gossiper) roundCounting() {
	majorityConfirmedMessages := false
	roundConfirmed := make(map[int][]*messages.TLCMessage)
	for {
		select {
		case <-gossiper.gossipWithConfirmation:
			gossiper.gossipWC = true
		case tlcConfirmed := <-gossiper.roundsUpdated:
			roundRaw, _ := gossiper.rounds.Load(tlcConfirmed.Origin)
			hash := tlcConfirmed.TxBlock.Hash()
			gossiper.allBlocks.Store(tools.BytesToHexString(hash[:]), tlcConfirmed)
			round := len(roundRaw.([]int)) - 1
			if list, ok := roundConfirmed[round]; ok {
				present := false
				for _, el := range list {
					if el.Equals(tlcConfirmed) {
						present = true
					}
				}
				if !present {
					roundConfirmed[round] = append(list, tlcConfirmed)
				}
			} else {
				roundConfirmed[round] = []*messages.TLCMessage{tlcConfirmed}
			}
			confirmed := 0
			for _, r := range roundConfirmed[(int)(gossiper.myTime)] {
				if r.Confirmed != -1 {
					confirmed++
				}
			}
			if confirmed > (int)(gossiper.n)-confirmed {
				majorityConfirmedMessages = true
			}
		}

		// Point 3 of hw3ex3 algorithm
		if gossiper.gossipWC && majorityConfirmedMessages {
			if !gossiper.hw3ex4 || gossiper.myTime%3 == 2 {
				best := roundConfirmed[(int)(gossiper.myTime)][0]
				for _, conf := range roundConfirmed[(int)(gossiper.myTime)] {
					if best.Fitness < conf.Fitness {
						best = conf
					}
				}
				round1 := false
				origin := ""
				id := (uint32)(0)
				for _, conf := range roundConfirmed[(int)(gossiper.myTime)-2] {
					if conf.TxBlock.Transaction.Equals(best.TxBlock.Transaction) {
						round1 = true
						origin = conf.Origin
						id = conf.ID
						break
					}
				}
				round2 := false
				for _, conf := range roundConfirmed[(int)(gossiper.myTime)-1] {
					if conf.TxBlock.Transaction.Equals(best.TxBlock.Transaction) {
						round2 = true
						break
					}
				}
				if round1 && round2 {
					gossiper.blockchain = append(gossiper.blockchain, best.TxBlock)
					fmt.Println("CONSENSUS ON QSC round", gossiper.myTime,
						"message origin", origin,
						"ID", id,
						"file names", gossiper.getFileNames(),
						"size", best.TxBlock.Transaction.Size,
						"metahash", tools.BytesToHexString(best.TxBlock.Transaction.MetafileHash))
					gossiper.bestBlock = nil
				}
				gossiper.gossipWC = false
				ticker := time.NewTicker(time.Millisecond)
				select {
				case gossiper.canGossipWC <- true:
				case <-ticker.C:
				}
			} else if gossiper.hw3ex4 {
				gossiper.bestBlock = roundConfirmed[(int)(gossiper.myTime)][0]
				for _, conf := range roundConfirmed[(int)(gossiper.myTime)] {
					if gossiper.bestBlock.Fitness < conf.Fitness {
						gossiper.bestBlock = conf
					}
				}
				go gossiper.publish(&files.FileMetadata{
					FileName: gossiper.bestBlock.TxBlock.Transaction.Name,
					FileSize: gossiper.bestBlock.TxBlock.Transaction.Size,
					MetaHash: gossiper.bestBlock.TxBlock.Transaction.MetafileHash,
				}, gossiper.bestBlock.Fitness)
			}
			majorityConfirmedMessages = false

			confPairs := gossiper.formatConfirmedPairs(roundConfirmed[(int)(gossiper.myTime)])
			gossiper.myTime++
			fmt.Println("ADVANCING TO round", gossiper.myTime,
				"BASED ON CONFIRMED MESSAGES", confPairs)
		}
	}
}

func (gossiper *Gossiper) getFileNames() string {
	var names []string = nil
	for _, n := range gossiper.blockchain {
		names = append(names, n.Transaction.Name)
	}
	return strings.Join(names, " ")
}

func (gossiper *Gossiper) formatConfirmedPairs(roundConfirmed []*messages.TLCMessage) string {
	var pairs []string = nil
	i := 1
	for _, tlc := range roundConfirmed {
		if tlc.Confirmed != -1 {
			pairs = append(pairs, fmt.Sprintf("origin%d %s ID%d %d", i, tlc.Origin, i, tlc.ID))
			i++
		}
	}
	return strings.Join(pairs, ", ")
}
