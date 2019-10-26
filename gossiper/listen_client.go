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

		fmt.Print("CLIENT MESSAGE ", message.Text)
		if *message.Destination != "" {
			fmt.Print(" dest ", *message.Destination)
		}
		fmt.Println()

		if gossiper.simple {
			gossiper.createSimple(message)
		} else {
			if *message.Destination != "" {
				if *message.File != "" {
					if *message.Request != nil {
						gossiper.createRequest(message)
					} else {
						gossiper.indexedFiles = append(gossiper.indexedFiles,
							files.NewFileMetadata(*message.File))
					}
				} else {
					gossiper.createPrivate(message)
				}
			} else {
				gossiper.createRumor(message)
			}
		}
	}
}

func (gossiper *Gossiper) createSimple(message *messages.Message) {
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
	gossiper.handleOriginDataRequest(request)
}

func (gossiper *Gossiper) createPrivate(message *messages.Message) {
	private := &messages.PrivateMessage{
		Origin:      gossiper.Name,
		ID:          0,
		Text:        message.Text,
		Destination: *message.Destination,
		HopLimit:    hopLimit,
	}
	gossiper.handlePrivate(private)
}

func (gossiper *Gossiper) createRumor(message *messages.Message) {
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
