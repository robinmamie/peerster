package gossiper

import (
	"fmt"

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
			if message.Destination != nil && *message.Destination != "" {
				if message.File != nil && *message.File != "" {
					if message.Request != nil && *message.Request != nil {
						go gossiper.createRequest(message)
					}
				} else {
					go gossiper.createPrivate(message)
				}
			} else if message.File != nil && *message.File != "" {
				go gossiper.indexFile(*message.File)
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
	request := &messages.DataRequest{
		Origin:      gossiper.Name,
		Destination: *message.Destination,
		HopLimit:    hopLimit,
		HashValue:   *message.Request,
	}
	gossiper.handleClientDataRequest(request, *message.File)
}

// indexFile indexes a local file and saves all its chunks.
func (gossiper *Gossiper) indexFile(fileName string) {
	fileMetaData, chunks := files.NewFileMetadata(fileName)
	gossiper.indexedFiles.Store(tools.BytesToHexString(fileMetaData.MetaHash), fileMetaData.MetaFile)

	// Store chunks in gossiper
	if chunks != nil {
		totalChunks := len(fileMetaData.MetaFile) / files.SHA256ByteSize
		for chunkNumber := 1; chunkNumber <= totalChunks; chunkNumber++ {
			hashValue := fileMetaData.MetaFile[files.SHA256ByteSize*(chunkNumber-1) : files.SHA256ByteSize*chunkNumber]
			hexChunkHash := tools.BytesToHexString(hashValue)
			gossiper.fileChunks.Store(hexChunkHash, chunks[chunkNumber-1])
		}
	}
}

// createPrivate creates a PrivateMessage and forwards it.
func (gossiper *Gossiper) createPrivate(message *messages.Message) {
	fmt.Println("CLIENT MESSAGE", message.Text,
		"dest", *message.Destination)

	private := &messages.PrivateMessage{
		Origin:      gossiper.Name,
		ID:          0, // No need to count
		Text:        message.Text,
		Destination: *message.Destination,
		HopLimit:    hopLimit,
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
	gossiper.receivedRumor(rumor)
}
