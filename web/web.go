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
	http.HandleFunc("/chat", getLatestRumorMessages)
	http.HandleFunc("/peers", getKnownPeers)
	http.HandleFunc("/post", sendMessage)
	http.HandleFunc("/addpeer", addPeer)
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

func getLatestRumorMessages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		msgList := gossiper.GetLatestRumorMessagesList()
		var messages []string = nil
		// TODO put that in java script?
		for _, r := range msgList {
			msg := r.Origin + " (" + fmt.Sprint(r.ID) + "): " + r.Text
			msg = "<p>" + msg + "</p>"
			messages = append(messages, msg)
		}
		msgListJSON, err := json.Marshal(messages)
		if err != nil {
			log.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(msgListJSON)
	}
}

func getKnownPeers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		peerList := gossip.Peers
		peerListJSON, err := json.Marshal(peerList)
		if err != nil {
			log.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(peerListJSON)
	}
}

func sendMessage(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
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

func addPeer(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
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
