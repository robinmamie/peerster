package gossiper

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/robinmamie/Peerster/files"
	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"
)

// handleSimple sends a packet to all other known peers if we are in simple
// mode.
func (gossiper *Gossiper) handleSimple(simple *messages.SimpleMessage) {
	if gossiper.simple {
		gossiper.sendSimple(simple)
	}
}

// handleRumor begins the rumormongering logic when we get a rumor message.
func (gossiper *Gossiper) handleRumor(rumor *messages.RumorMessage, address string) {
	gossiper.updateRoutingTable(rumor, address)
	fmt.Println("RUMOR origin", rumor.Origin, "from",
		address, "ID", rumor.ID, "contents",
		rumor.Text)
	gossiper.receivedRumor(rumor)
	gossiper.sendCurrentStatus(address)
}

// handleStatus communicates with other threads, or handles the status as an
// ack.
func (gossiper *Gossiper) handleStatus(status *messages.StatusPacket, address string) {
	// Wake up correct subroutine if status received
	for _, target := range gossiper.Peers {

		if target == address {
			// Empty expected channel before
			expectedRaw, _ := gossiper.expected.Load(target)
			expected := expectedRaw.(chan bool)
			for len(expected) > 0 {
				select {
				case <-expected:
				default:
				}
			}

			// Send packet to correct channel, as many times as possible
			channelRaw, _ := gossiper.statusWaiting.Load(target)
			channel := channelRaw.(chan *messages.StatusPacket)
			listening := true
			unexpected := true
			for listening {
				select {
				case channel <- status:
					// Allow for the routine to process the message
					timeout := time.NewTicker(1 * time.Millisecond)
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
			// If unexpected status, then compare vectors
			if unexpected {
				gossiper.compareVectors(status, address)
			}
		}
	}
}

// handlePrivate displays and stores (for the GUI) all privately addressed
// private messages.
//
// As for all other point to point messages, the function routes it if it is not
// destined for this node in particular.
func (gossiper *Gossiper) handlePrivate(private *messages.PrivateMessage) {
	if gossiper.ptpMessageReachedDestination(private) {

		gossiper.allMessages = append(gossiper.allMessages, &messages.GossipPacket{Private: private})

		fmt.Println("PRIVATE origin", private.Origin,
			"hop-limit", private.HopLimit,
			"contents", private.Text)
	}
}

// handleClientDataRequest handles all the logic behind downloading and
// and reconstructing a file coming from a foreign peer.
func (gossiper *Gossiper) handleClientDataRequest(request *messages.DataRequest, fileName string) {

	fileHash := tools.BytesToHexString(request.HashValue)
	// Avoid several downloads of the same file at the same time
	if _, ok := gossiper.dataChannels.Load(fileHash); ok {
		return
	}

	// Create communication channel and download metafile
	gossiper.dataChannels.Store(fileHash, make(chan *messages.DataReply))
	gossiper.handleDataRequest(request, fileName, 0)

	reply := gossiper.waitForValidDataReply(request, fileHash, fileName, 0)
	if reply == nil || reply.Data == nil {
		// The destination does not have the file, simply terminate
		return
	}

	// Index the file.
	metaFile := reply.Data
	metadataFile := files.FileMetadata{
		FileName: fileName,
		MetaFile: metaFile,
		MetaHash: reply.HashValue,
	}
	gossiper.indexedFiles.Store(tools.BytesToHexString(reply.HashValue), metadataFile)

	// The metaFileList contains all hashes of all concerned chunks.
	var metaFileList []string
	totalChunks := len(metaFile) / files.SHA256ByteSize
	for chunkNumber := 1; chunkNumber <= totalChunks; chunkNumber++ {
		// Send chunks request
		request.HashValue = metaFile[files.SHA256ByteSize*(chunkNumber-1) : files.SHA256ByteSize*chunkNumber]
		// Update the list of meta-hashes for all chunks
		hexChunkHash := tools.BytesToHexString(request.HashValue)
		metaFileList = append(metaFileList, hexChunkHash)
		if _, ok := gossiper.fileChunks.Load(hexChunkHash); !ok {
			// The chunk is absent from our table, so we download it.
			// Reset hop limit
			request.HopLimit = hopLimit
			gossiper.handleDataRequest(request, fileName, chunkNumber)
			if reply := gossiper.waitForValidDataReply(request, fileHash, fileName, chunkNumber); reply != nil {
				// Store chunk if valid answer.
				gossiper.fileChunks.Store(hexChunkHash, reply.Data)
			}
		}
	}
	// Reconstruct file from chunks and save correct size, iff we have all
	// the chunks.
	if ok := files.BuildFileFromChunks(fileName, metaFileList, &gossiper.fileChunks); ok {
		fmt.Println("RECONSTRUCTED file", fileName)
	}
	gossiper.dataChannels.Delete(fileHash)
}

// waitForValidDataReply resends a DataRequest in 5 seconds intervals, and drops
// every non coherent reply.
func (gossiper *Gossiper) waitForValidDataReply(request *messages.DataRequest, fileHash string, fileName string, index int) *messages.DataReply {
	ticker := time.NewTicker(time.Duration(dataRequestTimeout) * time.Second)
	chunkHashStr := tools.BytesToHexString(request.HashValue)
	channel, _ := gossiper.dataChannels.Load(fileHash)
	for {
		select {
		case <-ticker.C:
			// Timeout
			gossiper.handleDataRequest(request, fileName, index)
		case reply := <-channel.(chan *messages.DataReply):
			// Drop any message that has a non-coherent checksum
			receivedHash := sha256.Sum256(reply.Data)
			receivedHashStr := tools.BytesToHexString(receivedHash[:])
			if receivedHashStr == chunkHashStr {
				return reply
			}
			return nil
		}
	}
}

// printFileDownloadInformation prints the output message when downloading a
// chunk.
func printFileDownloadInformation(request *messages.DataRequest, fileName string, chunk int) {
	fmt.Print("DOWNLOADING ")
	if chunk == 0 {
		fmt.Printf("metafile of %s ", fileName)
	} else {
		fmt.Printf("%s chunk %d ", fileName, chunk)
	}
	fmt.Println("from", request.Destination)
}

// handleDataRequest replies with the asked metafile or chunk.
// The fields fileName and index are only used for displaying purposes, when
// creating a DataRequest.
//
// As for all other point to point messages, the function routes it if it is not
// destined for this node in particular.
func (gossiper *Gossiper) handleDataRequest(request *messages.DataRequest, fileName string, index int) {
	if request.Origin == gossiper.Name {
		printFileDownloadInformation(request, fileName, index)
	}
	if gossiper.ptpMessageReachedDestination(request) {
		requestedHash := tools.BytesToHexString(request.HashValue)
		// Send corresponding DataReply back
		if metadataFile, ok := gossiper.indexedFiles.Load(requestedHash); ok {
			// Send meta-file
			gossiper.sendDataReply(request, metadataFile.(*files.FileMetadata).MetaFile)
		} else if chunk, ok := gossiper.fileChunks.Load(requestedHash); ok {
			// Send chunk data
			gossiper.sendDataReply(request, chunk.([]byte))
		} else {
			// If not present, send empty packet
			gossiper.sendDataReply(request, nil)
		}
	}
}

// sendDataReply creates a DataReply and handles it.
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

// handleDataReply handles a DataReply by giving it to the corresponding thread.
//
// As for all other point to point messages, the function routes it if it is not
// destined for this node in particular.
func (gossiper *Gossiper) handleDataReply(reply *messages.DataReply) {
	if gossiper.ptpMessageReachedDestination(reply) {
		gossiper.dataChannels.Range(func(key interface{}, value interface{}) bool {
			channel, _ := gossiper.dataChannels.Load(key)
			select {
			case channel.(chan *messages.DataReply) <- reply:
			default:
			}
			return true
		})
	}
}

// ptpMessageReachedDestination verifies if a point-to-point message has reached
// its destination. Otherwise, it just forwards it along its route.
func (gossiper *Gossiper) ptpMessageReachedDestination(ptpMessage messages.PointToPoint) bool {
	if ptpMessage.GetDestination() == gossiper.Name {
		return true
	}
	if ptpMessage.GetHopLimit() > 0 {
		ptpMessage.DecrementHopLimit()
		if destination, ok := gossiper.routingTable.Load(ptpMessage.GetDestination()); ok {
			gossiper.sendGossipPacket(destination.(string), ptpMessage.CreatePacket())
		}
	}
	return false
}

type match struct {
	Hash   string
	Name   string
	Origin string
}

func (gossiper *Gossiper) handleClientSearchRequest(request *messages.SearchRequest, budgetIsUserDefined bool) {
	budget := request.Budget
	gossiper.handleSearchRequest(request, "")
	// Periodic increase of budget
	if !budgetIsUserDefined {
		go func() {
			timeout := time.NewTicker(time.Second)
			for {
				select {
				case <-timeout.C:
					if budget == 32 {
						return
					}
					budget = budget * 2
					if budget > 32 {
						budget = 32
					}
					request.Budget = budget
					gossiper.handleSearchRequest(request, "")
				case <-gossiper.searchFinished:
					return
				}
			}
		}()
	}

	var matchList []match = nil
	for {
		// TODO add timer to kill process when nothing comes back
		select {
		case reply := <-gossiper.searchReply:
			for _, results := range reply.Results {
				chunkMap := []string{}
				for _, v := range results.ChunkMap {
					chunkMap = append(chunkMap, strconv.FormatUint(v, 10))
				}
				hexMetaHash := tools.BytesToHexString(results.MetafileHash)
				chunkList := strings.Join(chunkMap, ",")
				fmt.Print("FOUND match ", results.FileName, " at ", reply.Origin,
					" metafile=", hexMetaHash,
					" chunks=", chunkList, "\n")

				if results.ChunkCount == (uint64)(len(results.ChunkMap)) {
					fullMatch := true
					for _, m := range matchList {
						if m.Hash == hexMetaHash && m.Name == results.FileName {
							fullMatch = false
						}
					}
					if fullMatch {
						matchList = append(matchList, match{
							Hash:   hexMetaHash,
							Name:   results.FileName,
							Origin: reply.Origin,
						})
						gossiper.fileDestinations.Store(hexMetaHash, reply.Origin)
						if len(matchList) == 2 {
							gossiper.searchFinished <- true
							fmt.Println("SEARCH FINISHED")
							return
						}
					}
				}
			}
		}
	}
}

func (gossiper *Gossiper) handleSearchRequest(request *messages.SearchRequest, sender string) {
	if gossiper.requestCollision(request) {
		return
	}
	gossiper.processSearch(request)
	request.Budget--
	if request.Budget > 0 {
		randomPeerList := gossiper.getRandomPeerList(sender)
		if request.Budget <= (uint64)(len(randomPeerList)) {
			gossiper.forwardSearchRequest(request, randomPeerList[:len(randomPeerList)])
		} else {
			gossiper.forwardSearchRequest(request, randomPeerList)
		}
	}
}

func (gossiper *Gossiper) requestCollision(request *messages.SearchRequest) bool {
	listening := true
	for listening {
		select {
		case gossiper.searchRequestLookup <- request:
		default:
		}
		timeout := time.NewTicker(time.Millisecond)
		select {
		case conflict := <-gossiper.searchRequestTimeout:
			if conflict {
				return true
			}
		case <-timeout.C:
			listening = false
		}
	}
	go gossiper.listenForCollision(request)
	return false
}

func (gossiper *Gossiper) listenForCollision(request *messages.SearchRequest) {
	timeout := time.NewTicker(500 * time.Millisecond)
	for {
		select {
		case otherRequest := <-gossiper.searchRequestLookup:
			if otherRequest.Origin == request.Origin && len(otherRequest.Keywords) == len(request.Keywords) {
				matching := true
				for i, k := range otherRequest.Keywords {
					if k != request.Keywords[i] {
						matching = false
						select {
						case gossiper.searchRequestTimeout <- false:
						default:
						}
						break
					}
				}
				if matching {
					select {
					case gossiper.searchRequestTimeout <- true:
					default:
					}
				}
			}
		case <-timeout.C:
			return
		}
	}
}

func (gossiper *Gossiper) processSearch(request *messages.SearchRequest) {
	var results []*messages.SearchResult = nil
	for _, regex := range request.Keywords {
		gossiper.indexedFiles.Range(func(key interface{}, value interface{}) bool {
			name := value.(*files.FileMetadata).FileName
			match, _ := regexp.MatchString(regex, name)
			// TODO handle error
			if match {
				hash, _ := hex.DecodeString(key.(string))
				// TODO handle error
				results = append(results, &messages.SearchResult{
					FileName:     name,
					MetafileHash: hash,
				})
			}
			return !match
		})
	}

	for i, r := range results {
		chunkMap, chunkCount := gossiper.getChunkMap(r.MetafileHash)
		results[i].ChunkMap = chunkMap
		results[i].ChunkCount = chunkCount
	}

	if results != nil {
		gossiper.handleSearchReply(&messages.SearchReply{
			Origin:      gossiper.Name,
			Destination: request.Origin,
			HopLimit:    hopLimit,
			Results:     results,
		})
	}
}

func (gossiper *Gossiper) getChunkMap(metaHash []byte) ([]uint64, uint64) {
	metadata, _ := gossiper.indexedFiles.Load(tools.BytesToHexString(metaHash))
	metaFile := metadata.(*files.FileMetadata).MetaFile
	// TODO check error
	chunkNumber := len(metaFile) / files.SHA256ByteSize
	var chunkMap []uint64 = nil
	for i := 1; i <= chunkNumber; i++ {
		chunkHash := metaFile[files.SHA256ByteSize*(i-1) : files.SHA256ByteSize*i]
		chunkHex := tools.BytesToHexString(chunkHash)
		_, ok := gossiper.fileChunks.Load(chunkHex)
		if ok {
			chunkMap = append(chunkMap, (uint64)(i))
		}
	}
	return chunkMap, (uint64)(chunkNumber)
}

func (gossiper *Gossiper) forwardSearchRequest(request *messages.SearchRequest, peers []string) {
	numberOfPeers := (uint64)(len(peers))
	if numberOfPeers == 0 {
		return
	}
	baseBudget := request.Budget / numberOfPeers
	remainingBudget := request.Budget % numberOfPeers
	for i, peer := range peers {
		bonusPoint := (uint64)(0)
		if (uint64)(i) < remainingBudget {
			bonusPoint++
		}
		gossiper.sendGossipPacket(peer, &messages.GossipPacket{
			SearchRequest: &messages.SearchRequest{
				Origin:   request.Origin,
				Keywords: request.Keywords,
				Budget:   baseBudget + bonusPoint,
			},
		})
	}
}

func (gossiper *Gossiper) handleSearchReply(reply *messages.SearchReply) {
	if gossiper.ptpMessageReachedDestination(reply) {
		for {
			select {
			case gossiper.searchReply <- reply:
			default:
				return
			}
		}
	}
}
