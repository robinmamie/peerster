package gossiper

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/robinmamie/Peerster/files"
	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"
)

func (gossiper *Gossiper) handleSimple(simple *messages.SimpleMessage) {
	// Send packet to all other known peers if we are in simple mode
	if gossiper.simple {
		gossiper.sendSimple(simple)
	}
}

func (gossiper *Gossiper) handleRumor(rumor *messages.RumorMessage, address string) {
	fmt.Println("RUMOR origin", rumor.Origin, "from",
		address, "ID", rumor.ID, "contents",
		rumor.Text)

	gossiper.receivedRumor(rumor, address)
	gossiper.sendCurrentStatus(address)
}

func (gossiper *Gossiper) handleStatus(status *messages.StatusPacket, address string) {
	// Wake up correct subroutine if status received
	unexpected := true
	for _, target := range gossiper.Peers {

		if target == address {
			// Empty expected channel before
			expectedRaw, _ := gossiper.expected.Load(target)
			expected := expectedRaw.(chan bool)
			for len(expected) > 0 {
				<-expected
			}

			// Send packet to correct channel, as many times as possible
			listening := true
			channelRaw, _ := gossiper.statusWaiting.Load(target)
			channel := channelRaw.(chan *messages.StatusPacket)
			for listening {
				select {
				case channel <- status:
					// Allow for the routine to process the message
					timeout := time.NewTicker(10 * time.Millisecond)
					select {
					case <-expected:
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
	// TODO !! what to do for the GUI? Other list? Same list but with GossipPacket and then the server handles the differences with a switch?
	if gossiper.ptpMessageReachedDestination(private) {

		if oldValue, ok := gossiper.PrivateMessages.Load(private.Origin); ok {
			gossiper.PrivateMessages.Store(private.Origin, append(oldValue.([]*messages.PrivateMessage), private))
		} else {
			gossiper.PrivateMessages.Store(private.Origin, []*messages.PrivateMessage{private})
		}

		fmt.Println("PRIVATE origin", private.Origin,
			"hop-limit", private.HopLimit,
			"contents", private.Text)
	}
}

func (gossiper *Gossiper) handleClientDataRequest(request *messages.DataRequest, fileName string) {

	fileHash := tools.BytesToHexString(request.HashValue)
	// Avoid several downloads of the same file
	// TODO here would be the reason to use a map for the files already downloaded
	if _, ok := gossiper.dataChannels.Load(fileHash); ok {
		return
	}
	gossiper.dataChannels.Store(fileHash, make(chan *messages.DataReply))
	gossiper.handleDataRequest(request)
	printFileDownloadInformation(request, fileName, 0)

	go func() {
		reply := gossiper.waitForValidDataReply(request, fileHash)
		metaFile := reply.Data
		if metaFile == nil {
			// TODO should abort if node does not have metafile?
			return
		}
		totalChunks := len(metaFile) / files.SHA256ByteSize
		fileMetaData := &files.FileMetadata{
			FileName: fileName,
			FileSize: totalChunks * files.ChunkSize, // Biggest possible size, will be changed when exact size known
			MetaFile: metaFile,
			MetaHash: reply.HashValue,
		}
		gossiper.indexedFiles = append(gossiper.indexedFiles, fileMetaData)

		gossiper.fileChunks.Store(fileName, make([][]byte, 0))

		for chunkNumber := 1; chunkNumber <= totalChunks; chunkNumber++ {
			// Send chunks request & reset hop limit
			request.HashValue = metaFile[files.SHA256ByteSize*(chunkNumber-1) : files.SHA256ByteSize*chunkNumber]
			request.HopLimit = hopLimit
			gossiper.handleDataRequest(request)
			printFileDownloadInformation(request, fileName, chunkNumber)
			reply := gossiper.waitForValidDataReply(request, fileHash)

			// Store chunk
			fileChunksRaw, _ := gossiper.fileChunks.Load(fileName)
			gossiper.fileChunks.Store(fileName, append(fileChunksRaw.([][]byte), reply.Data))
		}
		// Reconstruct file from chunks and save correct size
		fileChunksRaw, _ := gossiper.fileChunks.Load(fileName)
		fileMetaData.FileSize = files.BuildFileFromChunks(fileName, fileChunksRaw.([][]byte))
		fmt.Println("RECONSTRUCTED file", fileName)
		gossiper.dataChannels.Delete(fileHash)
	}()
}

func (gossiper *Gossiper) waitForValidDataReply(request *messages.DataRequest, fileHash string) *messages.DataReply {
	ticker := time.NewTicker(5 * time.Second)
	chunkHashStr := tools.BytesToHexString(request.HashValue)
	channel, _ := gossiper.dataChannels.Load(fileHash)
	for {
		select {
		case <-ticker.C:
			// TODO !! Print message!! Integrate to handleDataRequest
			gossiper.handleDataRequest(request)
		case reply := <-channel.(chan *messages.DataReply):
			// Drop any message that has a non-coherent checksum, or does not come from the desired destination
			// TODO !! If empty data, should choose another peer? Continue without reconstructing? Look answer on forum.
			receivedHash := sha256.Sum256(reply.Data)
			receivedHashStr := tools.BytesToHexString(receivedHash[:])
			if receivedHashStr == chunkHashStr && reply.Origin == request.Destination {
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
		requestedHash := tools.BytesToHexString(request.HashValue)
		// Send corresponding DataReply back
		for _, fileMetadata := range gossiper.indexedFiles {
			if tools.BytesToHexString(fileMetadata.MetaHash) == requestedHash {
				gossiper.sendDataReply(request, fileMetadata.MetaFile)
				return
			}
			chunksRaw, _ := gossiper.fileChunks.Load(fileMetadata.FileName)
			chunks := chunksRaw.([][]byte)
			numberOfChunks := len(chunks)
			for i := 0; i < numberOfChunks; i++ {
				currentHash := tools.BytesToHexString(fileMetadata.MetaFile[files.SHA256ByteSize*i : files.SHA256ByteSize*(i+1)])
				if currentHash == requestedHash {
					gossiper.sendDataReply(request, chunks[i])
					return
				}
			}
		}
		// If not present, send empty packet
		// FIXME bugged?
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
		gossiper.dataChannels.Range(func(key interface{}, value interface{}) bool {
			channel, _ := gossiper.dataChannels.Load(key)
			channel.(chan *messages.DataReply) <- reply
			return true
		})
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
		if destination, ok := gossiper.routingTable.Load(ptpMessage.GetDestination()); ok {
			gossiper.sendGossipPacket(destination.(string), ptpMessage.CreatePacket())
		}
	}
	return false
}
