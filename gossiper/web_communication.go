package gossiper

import "github.com/robinmamie/Peerster/messages"

// GetLatestRumorMessagesList returns a list of the latest rumor messages.
func (gossiper *Gossiper) GetLatestRumorMessagesList() []*messages.RumorMessage {
	messagesQuantity := len(gossiper.allMessages)
	defer func() {
		gossiper.latestMessageID = messagesQuantity
	}()
	if gossiper.latestMessageID < messagesQuantity {
		return gossiper.allMessages[gossiper.latestMessageID:]
	}
	return nil
}

// GetRumorMessagesList returns the list of all rumor messages.
func (gossiper *Gossiper) GetRumorMessagesList() []*messages.RumorMessage {
	gossiper.latestMessageID = len(gossiper.allMessages)
	return gossiper.allMessages
}

// AddPeer adds a new known peer and its logic to the gossiper.
func (gossiper *Gossiper) AddPeer(address string) {
	gossiper.updateMutex.Lock()
	gossiper.Peers = append(gossiper.Peers, address)
	gossiper.updateMutex.Unlock()

	gossiper.statusWaiting.Store(address, make(chan *messages.StatusPacket))
	gossiper.expected.Store(address, make(chan bool))
}
