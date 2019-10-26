package gossiper

import (
	"fmt"

	"github.com/dedis/protobuf"
	"github.com/robinmamie/Peerster/files"
	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"
)

const hopLimit uint32 = 10

// listenClient handles the messages coming from the client.
func (gossiper *Gossiper) listenClient() {
	for {
		message := gossiper.getMessage()

		if gossiper.simple {
			simple := &messages.SimpleMessage{
				OriginalName:  gossiper.Name,
				RelayPeerAddr: gossiper.Address,
				Contents:      message.Text,
			}
			gossiper.sendSimple(simple)
		} else {
			if *message.Destination != "" {
				if *message.File != "" {
					if *message.Request != nil {
						request := &messages.DataRequest{
							Origin:      gossiper.Name,
							Destination: *message.Destination,
							HopLimit:    hopLimit,
							HashValue:   *message.Request,
						}
						gossiper.handleOriginDataRequest(request)
					} else {
						gossiper.indexedFiles = append(gossiper.indexedFiles,
							files.NewFileMetadata(*message.File))
					}
				} else {
					private := &messages.PrivateMessage{
						Origin:      gossiper.Name,
						ID:          0,
						Text:        message.Text,
						Destination: *message.Destination,
						HopLimit:    hopLimit,
					}
					gossiper.handlePrivate(private)
				}
			} else {
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
		}
	}
}

// getMessage listens to the client and waits for a Message.
func (gossiper *Gossiper) getMessage() *messages.Message {
	packetBytes, _ := tools.GetPacketBytes(gossiper.cliConn)

	// Decode packet
	var msg messages.Message
	err := protobuf.Decode(packetBytes, &msg)
	tools.Check(err)

	fmt.Print("CLIENT MESSAGE ", msg.Text)
	if *msg.Destination != "" {
		fmt.Print(" dest ", *msg.Destination)
	}
	fmt.Println()

	return &msg
}
