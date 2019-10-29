package gossiper

import (
	"crypto/sha256"
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

func (gossiper *Gossiper) handleClientDataRequest(request *messages.DataRequest, fileName string) {

	fileHash := fmt.Sprintf("%x", request.HashValue) // TODO modularize byte to hex string
	// Avoid several downloads of the same file
	if _, ok := gossiper.dataChannels[fileHash]; ok {
		return
	}
	gossiper.dataChannels[fileHash] = make(chan *messages.DataReply)
	fmt.Println(gossiper.dataChannels[fileHash])
	gossiper.handleDataRequest(request)
	printFileDownloadInformation(request, fileName, 0)

	go func() {
		reply := gossiper.waitForValidDataReply(request, fileHash)
		metaFile := reply.Data
		if metaFile == nil {
			// TODO should abort if node does not have metafile?
			return
		}
		fileMetaData := &files.FileMetadata{
			FileName: fileName,
			FileSize: 0, // TODO !! what to put here?
			MetaFile: metaFile,
			MetaHash: reply.HashValue,
		}
		gossiper.indexedFiles = append(gossiper.indexedFiles, fileMetaData)
		totalChunks := len(metaFile) / files.SHA256ByteSize
		gossiper.fileChunks[fileName] = make([][]byte, 0)

		for chunkNumber := 1; chunkNumber <= totalChunks; chunkNumber++ {
			// Send chunks request & reset hop limit
			request.HashValue = metaFile[files.SHA256ByteSize*(chunkNumber-1) : files.SHA256ByteSize*chunkNumber]
			request.HopLimit = hopLimit
			gossiper.handleDataRequest(request)
			printFileDownloadInformation(request, fileName, chunkNumber)
			reply := gossiper.waitForValidDataReply(request, fileHash)

			// Store chunk
			gossiper.fileChunks[fileName] = append(gossiper.fileChunks[fileName], reply.Data)
		}
		// Reconstruct file from chunks
		// TODO save file size here?
		files.BuildFileFromChunks(fileName, gossiper.fileChunks[fileName])
		fmt.Println("RECONSTRUCTED file", fileName)
	}()
}

func (gossiper *Gossiper) waitForValidDataReply(request *messages.DataRequest, fileHash string) *messages.DataReply {
	ticker := time.NewTicker(5 * time.Second)
	chunkHash := fmt.Sprintf("%x", request.HashValue)
	for {
		select {
		case <-ticker.C:
			gossiper.handleDataRequest(request)
		case reply := <-gossiper.dataChannels[fileHash]:
			// Drop any message that has a non-coherent checksum, or does not come from the desired destination
			receivedHash := fmt.Sprintf("%x", sha256.Sum256(reply.Data))
			if receivedHash == chunkHash && reply.Origin == request.Destination {
				return reply
			}
		}
	}
}

func printFileDownloadInformation(request *messages.DataRequest, fileName string, chunk int) {
	fmt.Print("DOWNLOADING ")
	if chunk == 0 {
		fmt.Printf("metafile of %s ", fileName)
	} else {
		fmt.Printf("%s chunk %d ", fileName, chunk)
	}
	fmt.Println("from", request.Destination)
}

func (gossiper *Gossiper) handleDataRequest(request *messages.DataRequest) {
	if gossiper.ptpMessageReachedDestination(request) {
		requestedHash := fmt.Sprintf("%x", request.HashValue)
		// Send corresponding DataReply back
		for _, fileMetadata := range gossiper.indexedFiles {
			if fmt.Sprintf("%x", fileMetadata.MetaHash) == requestedHash {
				gossiper.sendDataReply(request, fileMetadata.MetaFile)
				return
			}
			chunks := gossiper.fileChunks[fileMetadata.FileName]
			numberOfChunks := len(chunks)
			for i := 0; i < numberOfChunks; i++ {
				currentHash := fmt.Sprintf("%x", fileMetadata.MetaFile[files.SHA256ByteSize*i:files.SHA256ByteSize*(i+1)])
				if currentHash == requestedHash {
					gossiper.sendDataReply(request, chunks[i])
					return
				}
			}
		}
		// If not present, send empty packet (FIXME bugged)
		gossiper.sendDataReply(request, nil)
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
		for _, v := range gossiper.dataChannels {
			v <- reply
		}
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
		// TODO !!! What happens with the routing logic with something else than a rumour? E.g. a private message?
		gossiper.sendGossipPacket(gossiper.routingTable[ptpMessage.GetDestination()], ptpMessage.CreatePacket())
	}
	return false
}

// printPeers prints the list of known peers.
func (gossiper *Gossiper) printPeers() {
	fmt.Println("PEERS", strings.Join(gossiper.Peers, ","))
}
