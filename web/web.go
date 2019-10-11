package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/robinmamie/Peerster/messages"

	"github.com/robinmamie/Peerster/gossiper"

	"github.com/dedis/protobuf"
)

var gossip *gossiper.Gossiper
var localAddress string

// InitWebServer starts the web server.
func InitWebServer(g *gossiper.Gossiper, uiPort string) {
	gossip = g
	http.Handle("/", http.FileServer(http.Dir("./frontend")))
	http.HandleFunc("/id", getNodeID)
	http.HandleFunc("/chat", chatHandler)
	http.HandleFunc("/peers", peerHandler)
	localAddress = ":" + uiPort
	for {
		// Port number hard-coded
		err := http.ListenAndServe(localAddress, nil)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func getNodeID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		nodeNameList := []string{gossip.Name}
		nodeNameListJSON, err := json.Marshal(nodeNameList)
		if err != nil {
			log.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(nodeNameListJSON)
	}
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		msgList := gossip.GetLatestRumorMessagesList()
		var messages []string = nil
		// TODO put that in java script?
		for _, r := range msgList {
			msg := r.Origin + " (" + fmt.Sprint(r.ID) + "): " + r.Text
			msg = "<p id=\"msg\">" + msg + "</p>"
			messages = append(messages, msg)
		}
		msgListJSON, err := json.Marshal(messages)
		if err != nil {
			log.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(msgListJSON)

	case "POST":
		if err := r.ParseForm(); err != nil {
			log.Fatal(err)
		}
		conn, err := net.Dial("udp4", localAddress)
		if err != nil {
			log.Fatal(err)
		}
		packet := messages.Message{
			Text: r.PostForm["msg"][0],
		}
		packetBytes, err := protobuf.Encode(&packet)
		if err != nil {
			log.Fatal(err)
		}
		conn.Write(packetBytes)
		conn.Close()
	}
}

func peerHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		peerList := gossip.Peers
		var peers []string = nil
		for _, p := range peerList {
			peer := "<p id=\"msg\">" + p + "</p>"
			peers = append(peers, peer)
		}
		peersJSON, err := json.Marshal(peers)
		if err != nil {
			log.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(peersJSON)

	case "POST":
		if err := r.ParseForm(); err != nil {
			log.Fatal(err)
		}
		// TODO sanitize input?
		newPeer := r.PostForm["node"][0]
		gossip.Peers = append(gossip.Peers, newPeer)
	}
}
