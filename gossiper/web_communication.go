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
	// TODO modularize this (see constructor)
	gossiper.statusWaiting[address] = make(chan *messages.StatusPacket)
	gossiper.expected[address] = make(chan bool)
	gossiper.updateMutex.Unlock()
}