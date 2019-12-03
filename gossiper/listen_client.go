package gossiper

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/robinmamie/Peerster/tools"

	"github.com/robinmamie/Peerster/files"
	"github.com/robinmamie/Peerster/messages"
)

// listenClient handles the messages coming from the client.
func (gossiper *Gossiper) listenClient() {
	for {
		message := gossiper.getMessage()
		if gossiper.simple {
			gossiper.createSimple(message)
		} else {
			if message.Keywords != nil {
				go gossiper.createSearch(message)
			} else if message.Destination != nil && *message.Destination != "" {
				go gossiper.createPrivate(message)
			} else if message.File != nil && *message.File != "" {
				if message.Request != nil && *message.Request != nil && len(*message.Request) != 0 {
					go gossiper.createRequest(message)
				} else {
					go gossiper.indexFile(*message.File)
				}
			} else {
				go gossiper.createRumor(message)
			}
		}
	}
}

// createSimple creates a SimpleMessage and sends it.
func (gossiper *Gossiper) createSimple(message *messages.Message) {
	simple := &messages.SimpleMessage{
		OriginalName:  gossiper.Name,
		RelayPeerAddr: gossiper.Address,
		Contents:      message.Text,
	}
	gossiper.sendSimple(simple)
}

// createRequest creates a first DataRequest and starts the downloading of the
// file.
func (gossiper *Gossiper) createRequest(message *messages.Message) {
	destination, ok := gossiper.fileDestinations.Load(tools.BytesToHexString(*message.Request))
	if !ok {
		return
	}
	request := &messages.DataRequest{
		Origin:      gossiper.Name,
		Destination: destination.(string),
		HopLimit:    gossiper.hopLimit,
		HashValue:   *message.Request,
	}
	gossiper.handleClientDataRequest(request, *message.File)
}

// indexFile indexes a local file and saves all its chunks.
func (gossiper *Gossiper) indexFile(fileName string) {
	fileMetaData, chunks := files.NewFileMetadata(fileName)
	gossiper.indexedFiles.Store(tools.BytesToHexString(fileMetaData.MetaHash), fileMetaData)

	// Store chunks in gossiper
	if chunks != nil {
		totalChunks := len(fileMetaData.MetaFile) / files.SHA256ByteSize
		for chunkNumber := 1; chunkNumber <= totalChunks; chunkNumber++ {
			hashValue := fileMetaData.MetaFile[files.SHA256ByteSize*(chunkNumber-1) : files.SHA256ByteSize*chunkNumber]
			hexChunkHash := tools.BytesToHexString(hashValue)
			gossiper.fileChunks.Store(hexChunkHash, chunks[chunkNumber-1])
		}
	}

	if gossiper.hw3ex2 {
		if gossiper.hw3ex3 {
			if gossiper.gossipWC {
				<-gossiper.canGossipWC
			}
			gossiper.gossipWithConfirmation <- true
		}
		gossiper.publish(fileMetaData)
	}
}

func (gossiper *Gossiper) publish(fileMetaData *files.FileMetadata) {
	bp := messages.BlockPublish{
		Transaction: messages.TxPublish{
			Name:         fileMetaData.FileName,
			Size:         fileMetaData.FileSize,
			MetafileHash: fileMetaData.MetaHash,
		},
	}
	vc := &messages.StatusPacket{
		Want: make([]messages.PeerStatus, len(gossiper.vectorClock.Want)),
	}
	copy(vc.Want, gossiper.vectorClock.Want)
	tlcID := gossiper.getAndIncrementOwnID()
	tlc := &messages.TLCMessage{
		Origin:    gossiper.Name,
		ID:        tlcID,
		Confirmed: -1,
		TxBlock:   bp,
	}
	if gossiper.hw3ex3 {
		tlc.VectorClock = vc
	}
	if gossiper.hw3ex4 {
		rand.Seed(time.Now().UTC().UnixNano())
		tlc.Fitness = rand.Float32()
	}
	gossiper.receivedGossip(tlc, false)
	acknowledgements := make(map[string]bool)
	acknowledgements[gossiper.Name] = true
	ticker := time.NewTicker(time.Duration(gossiper.stubbornTimeout) * time.Second)
	for {
		if len(acknowledgements) > (int)(gossiper.n)-len(acknowledgements) {
			var witnesses []string = nil
			for k := range acknowledgements {
				witnesses = append(witnesses, k)
			}
			fmt.Println("RE-BROADCAST ID", tlc.ID,
				"WITNESSES", strings.Join(witnesses, ","))
			vc := &messages.StatusPacket{
				Want: make([]messages.PeerStatus, len(gossiper.vectorClock.Want)),
			}
			copy(vc.Want, gossiper.vectorClock.Want)
			conf := &messages.TLCMessage{
				Origin:    tlc.Origin,
				ID:        gossiper.getAndIncrementOwnID(),
				Confirmed: (int)(tlc.ID),
				TxBlock:   tlc.TxBlock,
			}
			if gossiper.hw3ex3 {
				conf.VectorClock = vc
			}
			gossiper.receivedGossip(conf, false)
			return
		}
		select {
		case <-ticker.C:
			go gossiper.receivedGossip(tlc, true)
		case ack := <-gossiper.ackBlock:
			if ack.ID == tlcID {
				acknowledgements[ack.Origin] = true
			}
		case <-gossiper.canGossipWC:
			return
		}
	}
}

// createPrivate creates a PrivateMessage and forwards it.
func (gossiper *Gossiper) createPrivate(message *messages.Message) {
	private := &messages.PrivateMessage{
		Origin:      gossiper.Name,
		ID:          0, // No need to count
		Text:        message.Text,
		Destination: *message.Destination,
		HopLimit:    gossiper.hopLimit,
	}
	gossiper.handlePrivate(private)
}

// createRumor creates a RumorMessage and rumormongers it.
func (gossiper *Gossiper) createRumor(message *messages.Message) {
	rumor := &messages.RumorMessage{
		Origin: gossiper.Name,
		ID:     gossiper.getAndIncrementOwnID(),
		Text:   message.Text,
	}
	gossiper.receivedGossip(rumor, false)
}

func (gossiper *Gossiper) createSearch(message *messages.Message) {
	budget := 2
	userDefined := false
	if message.Budget != nil {
		b, err := strconv.Atoi(*message.Budget)
		if err == nil {
			budget = b
			userDefined = true
		}
	}
	request := &messages.SearchRequest{
		Origin:   gossiper.Name,
		Budget:   (uint64)(budget),
		Keywords: *message.Keywords,
	}
	gossiper.handleClientSearchRequest(request, userDefined)
}
