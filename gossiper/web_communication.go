package gossiper

import "github.com/robinmamie/Peerster/messages"

// GetLatestMessagesList returns a list of the latest rumor and private
// messages.
func (gossiper *Gossiper) GetLatestMessagesList() []*messages.GossipPacket {
	messagesQuantity := len(gossiper.allMessages)
	defer func() {
		gossiper.latestMessageID = messagesQuantity
	}()
	if gossiper.latestMessageID < messagesQuantity {
		return gossiper.allMessages[gossiper.latestMessageID:]
	}
	return nil
}

// GetMessagesList returns the list of all rumor messages.
func (gossiper *Gossiper) GetMessagesList() []*messages.GossipPacket {
	gossiper.latestMessageID = len(gossiper.allMessages)
	return gossiper.allMessages
}

// AddPeer adds a new known peer and its logic to the gossiper.
func (gossiper *Gossiper) AddPeer(address string) {
	gossiper.peerMutex.Lock()
	gossiper.Peers = append(gossiper.Peers, address)
	gossiper.peerMutex.Unlock()
	gossiper.statusWaiting.Store(address, make(chan *messages.StatusPacket))
	gossiper.expected.Store(address, make(chan bool))
}

// GetLatestDestinationsList returns a list of the latest destinations.
func (gossiper *Gossiper) GetLatestDestinationsList() []string {
	gossiper.destinationMutex.Lock()
	destinationsQuantity := len(gossiper.destinationList)
	defer func() {
		gossiper.latestDestinationID = destinationsQuantity
		gossiper.destinationMutex.Unlock()
	}()
	if gossiper.latestDestinationID < destinationsQuantity {
		return gossiper.destinationList[gossiper.latestDestinationID:]
	}
	return nil
}

// GetDestinationsList returns the list of all destinations.
func (gossiper *Gossiper) GetDestinationsList() []string {
	gossiper.latestDestinationID = len(gossiper.destinationList)
	return gossiper.destinationList
}

// GetFileNames returns all known file names.
func (gossiper *Gossiper) GetFileNames() []string {
	var names []string = nil
	gossiper.FileHashes.Range(func(name interface{}, hash interface{}) bool {
		names = append(names, name.(string))
		return true
	})
	return names
}
