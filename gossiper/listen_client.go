package gossiper

import (
	"fmt"

	"github.com/dedis/protobuf"
	"github.com/robinmamie/Peerster/files"
	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"
)

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
					gossiper.indexedFiles = append(gossiper.indexedFiles,
						files.NewFileMetadata(*message.File))
				} else {
					packet := &messages.GossipPacket{
						Private: &messages.PrivateMessage{
							Origin:      gossiper.Name,
							ID:          0,
							Text:        message.Text,
							Destination: *message.Destination,
							HopLimit:    10,
						},
					}
					// TODO handle by source
					if address, ok := gossiper.routingTable[packet.Private.Destination]; ok {
						gossiper.sendGossipPacket(address, packet)
					}
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
