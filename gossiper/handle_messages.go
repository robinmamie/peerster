package gossiper

import (
	"fmt"
	"strings"
	"time"

	"github.com/robinmamie/Peerster/files"
	"github.com/robinmamie/Peerster/messages"
)

func (gossiper *Gossiper) handleSimple(simple *messages.SimpleMessage) {
	fmt.Println("SIMPLE MESSAGE origin", simple.OriginalName,
		"from", simple.RelayPeerAddr,
		"contents", simple.Contents)
	gossiper.printPeers()

	// Send packet to all other known peers if we are in simple mode
	if gossiper.simple {
		gossiper.sendSimple(simple)
	}
}

func (gossiper *Gossiper) handleRumor(rumor *messages.RumorMessage, address string) {
	fmt.Println("RUMOR origin", rumor.Origin, "from",
		address, "ID", rumor.ID, "contents",
		rumor.Text)
	gossiper.printPeers()

	gossiper.receivedRumor(rumor, address)
	gossiper.sendCurrentStatus(address)
}

func (gossiper *Gossiper) handleStatus(status *messages.StatusPacket, address string) {
	fmt.Print("STATUS from ", address)
	for _, s := range status.Want {
		fmt.Print(" peer ", s.Identifier, " nextID ", s.NextID)
	}
	fmt.Println()
	gossiper.printPeers()
	if status.IsEqual(gossiper.vectorClock) {
		fmt.Println("IN SYNC WITH", address)
	}

	// Wake up correct subroutine if status received
	unexpected := true
	for target, channel := range gossiper.statusWaiting {

		if target == address {
			// Empty expected channel before
			for len(gossiper.expected[target]) > 0 {
				<-gossiper.expected[target]
			}

			// Send packet to correct channel, as many times as possible
			listening := true
			for listening {
				select {
				case channel <- status:
					// Allow for the routine to process the message
					timeout := time.NewTicker(10 * time.Millisecond)
					select {
					case <-gossiper.expected[target]:
						unexpected = false
					case <-timeout.C:
						listening = false
					}
				default:
					listening = false
				}
			}
		}
	}
	// If unexpected Status, then compare vectors
	if unexpected {
		gossiper.compareVectors(status, address)
	}
}

func (gossiper *Gossiper) handlePrivate(private *messages.PrivateMessage) {
	// TODO ! should also write peers?
	// TODO !! what to do for the GUI? Other list? Same list but with GossipPacket and then the server handles the differences with a switch?
	if gossiper.ptpMessageReachedDestination(private) {
		fmt.Println("PRIVATE origin", private.Origin,
			"hop-limit", private.HopLimit,
			"contents", private.Text)
	}
}

func (gossiper *Gossiper) handleOriginDataRequest(request *messages.DataRequest) {
	// TODO communicate using a channel! Create it here

	go func() {

	}()
}

func (gossiper *Gossiper) handleDataRequest(request *messages.DataRequest) {
	if gossiper.ptpMessageReachedDestination(request) {
		// Send corresponding DataReply back
		for _, fileMetadata := range gossiper.indexedFiles {
			if string(fileMetadata.MetaHash) == string(request.HashValue) {
				gossiper.sendDataReply(request, fileMetadata.MetaFile)
			}
			numberOfChunks := 1 + fileMetadata.FileSize/files.ChunkSize
			for i := 0; i < numberOfChunks; i++ {
				currentHash := string(fileMetadata.MetaFile[files.SHA256Size*i : files.SHA256Size*(i+1)])
				if currentHash == string(request.HashValue) {
					data := fileMetadata.ExtractCorrespondingData(i)
					gossiper.sendDataReply(request, data)
				}
			}
		}
	}
}

func (gossiper *Gossiper) sendDataReply(request *messages.DataRequest, data []byte) {
	reply := &messages.DataReply{
		Origin:      gossiper.Name,
		Destination: request.Origin,
		HopLimit:    hopLimit,
		HashValue:   request.HashValue,
		Data:        data,
	}
	gossiper.handleDataReply(reply)
}

func (gossiper *Gossiper) handleDataReply(reply *messages.DataReply) {
	if gossiper.ptpMessageReachedDestination(reply) {
		// Communicate with channel OriginDataRequest to transmit DataReply
	}
}

// ptpMessageReachedDestination verifies if a point-to-point message has reached its destination.
// Otherwise, it just forwards it along its route.
func (gossiper *Gossiper) ptpMessageReachedDestination(ptpMessage messages.PointToPoint) bool {
	if ptpMessage.GetDestination() == gossiper.Name {
		return true
	}
	// TODO combine 2 interface functions (get/decrement hoplimit) in 1?
	if ptpMessage.GetHopLimit() > 0 {
		ptpMessage.DecrementHopLimit()
		// It will crash if not known. It should never happen.
		gossiper.sendGossipPacket(gossiper.routingTable[ptpMessage.GetDestination()], ptpMessage.CreatePacket())
	}
	return false
}

// printPeers prints the list of known peers.
func (gossiper *Gossiper) printPeers() {
	fmt.Println("PEERS", strings.Join(gossiper.Peers, ","))
}
