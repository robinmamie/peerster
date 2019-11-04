package gossiper

import (
	"fmt"

	"github.com/robinmamie/Peerster/files"
	"github.com/robinmamie/Peerster/messages"
)

const hopLimit uint32 = 10

// listenClient handles the messages coming from the client.
func (gossiper *Gossiper) listenClient() {
	for {
		message := gossiper.getMessage()

		if gossiper.simple {
			gossiper.createSimple(message)
		} else {
			if *message.Destination != "" {
				if *message.File != "" {
					if *message.Request != nil {
						gossiper.createRequest(message)
					}
				} else {
					gossiper.createPrivate(message)
				}
			} else if *message.File != "" {
				gossiper.indexFile(*message.File)
			} else {
				gossiper.createRumor(message)
			}
		}
	}
}

func (gossiper *Gossiper) createSimple(message *messages.Message) {
	fmt.Println("CLIENT MESSAGE", message.Text)
	simple := &messages.SimpleMessage{
		OriginalName:  gossiper.Name,
		RelayPeerAddr: gossiper.Address,
		Contents:      message.Text,
	}
	gossiper.sendSimple(simple)
}

func (gossiper *Gossiper) createRequest(message *messages.Message) {
	request := &messages.DataRequest{
		Origin:      gossiper.Name,
		Destination: *message.Destination,
		HopLimit:    hopLimit,
		HashValue:   *message.Request,
	}
	gossiper.handleClientDataRequest(request, *message.File)
}

func (gossiper *Gossiper) indexFile(fileName string) {
	fileMetaData, chunks := files.NewFileMetadata(fileName)
	gossiper.indexedFiles = append(gossiper.indexedFiles, fileMetaData)
	// Store chunks in gossiper
	if chunks != nil {
		gossiper.fileChunks[fileName] = chunks
	}
}

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

func (gossiper *Gossiper) createRumor(message *messages.Message) {
	fmt.Println("CLIENT MESSAGE", message.Text)

	rumor := &messages.RumorMessage{
		Origin: gossiper.Name,
		ID:     gossiper.ownID,
		Text:   message.Text,
	}
	// TODO use another lock?
	gossiper.updateMutex.Lock()
	gossiper.ownID++
	gossiper.updateMutex.Unlock()
	gossiper.receivedRumor(rumor, "")
}
