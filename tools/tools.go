package tools

import (
	"fmt"
	"log"
	"math/rand"
	"net"
)

// UDPSize is the maximum number of bytes sent by a UDP message
const UDPSize = 1 << 13 // 8KB

// AddressToString converts a UDPAddr to a string.
func AddressToString(address *net.UDPAddr) string {
	return address.IP.String() + ":" + fmt.Sprintf("%d", address.Port)
}

// Check panics if the error given is not nil.
func Check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// FlipCoin returns true one time out of two,
func FlipCoin() bool {
	return rand.Int()%2 == 0
}

// GetPacketBytes receives any UDP packet sent to the corresponding connection.
func GetPacketBytes(connection *net.UDPConn) ([]byte, *net.UDPAddr) {
	var packetBytes []byte = make([]byte, UDPSize)

	n, address, err := connection.ReadFromUDP(packetBytes)
	Check(err)
	return packetBytes[:n], address
}

// SendPacketBytes send a UDP packet to the mentionned address.
func SendPacketBytes(connection *net.UDPConn, address string, packetBytes []byte) {
	udpDest, err := net.ResolveUDPAddr("udp4", address)
	Check(err)
	bytes, err := connection.WriteToUDP(packetBytes, udpDest)
	Check(err)
	if bytes != len(packetBytes) {
		log.Fatal(bytes, "bytes were sent instead of", len(packetBytes),
			"bytes.")
	}
}

// BytesToHexString converts a hash from its byte representation to a string.
func BytesToHexString(hash []byte) string {
	return fmt.Sprintf("%x", hash)
}
