package web

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"

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
	http.HandleFunc("/fullchat", fullChatHandler)
	http.HandleFunc("/peers", peerHandler)
	localAddress = ":" + uiPort
	for {
		// Port number hard-coded
		err := http.ListenAndServe(localAddress, nil)
		tools.Check(err)
	}
}

func getNodeID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		nodeNameList := []string{gossip.Name}
		nodeNameListJSON, err := json.Marshal(nodeNameList)
		tools.Check(err)

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
		tools.Check(err)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(msgListJSON)

	case "POST":
		err := r.ParseForm()
		tools.Check(err)
		conn, err := net.Dial("udp4", localAddress)
		tools.Check(err)

		packet := messages.Message{
			Text: r.PostForm["msg"][0],
		}
		packetBytes, err := protobuf.Encode(&packet)
		tools.Check(err)

		conn.Write(packetBytes)
		conn.Close()
	}
}

func fullChatHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		msgList := gossip.GetRumorMessagesList()
		var messages []string = nil
		for _, r := range msgList {
			msg := r.Origin + " (" + fmt.Sprint(r.ID) + "): " + r.Text
			msg = "<p id=\"msg\">" + msg + "</p>"
			messages = append(messages, msg)
		}
		msgListJSON, err := json.Marshal(messages)
		tools.Check(err)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(msgListJSON)
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
		tools.Check(err)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(peersJSON)

	case "POST":
		err := r.ParseForm()
		tools.Check(err)
		// TODO sanitize input?
		newPeer := r.PostForm["node"][0]
		gossip.Peers = append(gossip.Peers, newPeer)
	}
}
