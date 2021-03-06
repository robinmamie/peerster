package gossiper

import (
	"github.com/dedis/protobuf"
	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"
)

// getMessage listens to the client and waits for a Message.
func (gossiper *Gossiper) getMessage() *messages.Message {
	packetBytes, _ := tools.GetPacketBytes(gossiper.cliConn)

	// Decode packet
	var msg messages.Message
	err := protobuf.Decode(packetBytes, &msg)
	tools.Check(err)

	return &msg
}

// sendSimple sends a simple message to all other peers except the one who just
// forwarded it.
func (gossiper *Gossiper) sendSimple(simple *messages.SimpleMessage) {
	// Save previous address, update packet with address and encode it
	fromPeer := simple.RelayPeerAddr
	simple.RelayPeerAddr = gossiper.Address

	packet := &messages.GossipPacket{
		Simple: simple,
	}
	packetBytes, err := protobuf.Encode(packet)
	tools.Check(err)

	// Send to all peers except the last sender
	for _, address := range gossiper.Peers {
		if address != fromPeer {
			tools.SendPacketBytes(gossiper.conn, address, packetBytes)
		}
	}
}

// getGossipPacket listens to other peers and waits for a GossipPacket.
func (gossiper *Gossiper) getGossipPacket() (*messages.GossipPacket, string) {
	var packet messages.GossipPacket
	packetBytes, address := tools.GetPacketBytes(gossiper.conn)

	// Decode packet
	err := protobuf.Decode(packetBytes, &packet)
	tools.Check(err)

	return &packet, tools.AddressToString(address)
}

// sendGossipPacket sends a gossipPacket to the mentionned address.
func (gossiper *Gossiper) sendGossipPacket(address string, packet *messages.GossipPacket) {
	packetBytes, err := protobuf.Encode(packet)
	tools.Check(err)
	tools.SendPacketBytes(gossiper.conn, address, packetBytes)
}
