package main

import (
    "flag"
    "fmt"
    "messages"
    "net"
    "strings"
    "github.com/dedis/protobuf"
)

var uiPort string
var gossipAddr string
var name string
var peers []string
var simple bool

var err error

func parseFlags() {
    flag.StringVar(&uiPort, "UIPort", "8080", "port for the UI client")
    flag.StringVar(&gossipAddr, "gossipAddr", "127.0.0.1:5000", "ip:port for the gossiper")
    flag.StringVar(&name, "name", "", "name of the gossiper")
    var peersString string
    flag.StringVar(&peersString, "peers", "", "comma separated list of peers of the form ip:port")
    flag.BoolVar(&simple, "simple", false, "run gossiper in simple broadcast mode")

    flag.Parse()

    if peersString == "" {
        peers = []string{}
    } else {
        peers = strings.Split(peersString, ",")
    }
}

type Gossiper struct {
    address *net.UDPAddr
    conn *net.UDPConn
    Name string
}

func NewGossiper(address, name string) *Gossiper {
    // TODO use error for robustness
    udpAddr, _ := net.ResolveUDPAddr("udp4", address)
    udpConn, _ := net.ListenUDP("udp4", udpAddr)
    return &Gossiper{
        address: udpAddr,
        conn:    udpConn,
        Name:    name,
    }
}


func listenGossipers(gossiper *Gossiper) {
    for {
        packet := getPacket(gossiper.conn)
        if simple {
            fromPeer := packet.Simple.RelayPeerAddr
            fmt.Println("SIMPLE MESSAGE origin", packet.Simple.OriginalName,
                        "from", fromPeer, "contents", packet.Simple.Contents)
            if sendPacket(gossiper.conn, packet, fromPeer) {
                peers = append(peers, fromPeer)
            }
            fmt.Println("PEERS", strings.Join(peers, ","))
        }
    }
}

func listenClient() {

    udpAddr, _ := net.ResolveUDPAddr("udp4", "127.0.0.1:" + uiPort)
    udpConn, _ := net.ListenUDP("udp4", udpAddr)
    for {
        packet := getPacket(udpConn)
        if simple {
            packet.Simple.OriginalName = name
            fmt.Println("CLIENT MESSAGE", packet.Simple.Contents)
            sendPacket(udpConn, packet, "")
        }
    }
}

func getPacket(connection *net.UDPConn) *messages.GossipPacket {
    var packetBytes []byte = make([]byte, 1024) // TODO fix a sensible value
    var packet messages.GossipPacket
    n, _, _ := connection.ReadFromUDP(packetBytes)
    err = protobuf.Decode(packetBytes[:n], &packet)
    if err != nil {
        panic("Decode failed: "+err.Error())
    }
    return &packet
}

func sendPacket(connection *net.UDPConn, packet *messages.GossipPacket, fromPeer string) bool {
    packet.Simple.RelayPeerAddr = gossipAddr
    packetBytes, _ := protobuf.Encode(packet)
    fromPeerShallBeAdded := true
    for _, address := range peers {
        if (address != fromPeer) {
            udpDest, _ := net.ResolveUDPAddr("udp4", address)
            connection.WriteToUDP(packetBytes, udpDest)
        } else {
            fromPeerShallBeAdded = false
        }
    }
    return fromPeerShallBeAdded
}

func main() {

    parseFlags()

    go listenClient()
    gossiper := NewGossiper(gossipAddr, name)
    listenGossipers(gossiper)
}
