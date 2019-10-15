package web

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"

	"github.com/robinmamie/Peerster/gossiper"

	"github.com/dedis/protobuf"
)

var gossip *gossiper.Gossiper
var localAddress string

// InitWebServer starts the web server.
func InitWebServer(g *gossiper.Gossiper, uiPort string) {
	localAddress = ":" + uiPort
	gossip = g
	http.Handle("/", http.FileServer(http.Dir("./frontend")))
	http.HandleFunc("/id", getNodeID)
	http.HandleFunc("/chat", chatHandler)
	http.HandleFunc("/fullchat", fullChatHandler)
	http.HandleFunc("/peers", peerHandler)
	guiPort := 8080
	for {
		serverAddress := ":" + strconv.Itoa(guiPort)
		err := http.ListenAndServe(serverAddress, nil)
		if err != nil {
			guiPort++
		}
	}
}

func getNodeID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		nodeNameList := []string{gossip.Name, gossip.Address, gossip.UIPort}
		nodeNameListJSON, err := json.Marshal(nodeNameList)
		tools.Check(err)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(nodeNameListJSON)
	}
}

func parseRumorList(msgList []*messages.RumorMessage) []string {
	var messages []string = nil
	for _, r := range msgList {
		msg := r.Origin + " (" + fmt.Sprint(r.ID) + "): <code>" + r.Text + "</code>"
		msg = "<p id=\"msg\">" + msg + "</p>"
		messages = append(messages, msg)
	}
	return messages
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		msgList := gossip.GetLatestRumorMessagesList()
		messages := parseRumorList(msgList)
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
		messages := parseRumorList(msgList)
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
		newPeer := r.PostForm["node"][0]
		gossip.Peers = append(gossip.Peers, newPeer)
	}
}
