package main

import (
	"flag"
	"strings"

	"github.com/robinmamie/Peerster/gossiper"
)

func main() {

	var uiPort string
	var gossipAddr string
	var name string
	var peers []string = nil
	var simple bool

	var peersString string

	flag.StringVar(&uiPort, "UIPort", "8080", "port for the UI client")
	flag.StringVar(&gossipAddr, "gossipAddr", "127.0.0.1:5000",
		"ip:port for the gossiper")
	flag.StringVar(&name, "name", "", "name of the gossiper")
	flag.StringVar(&peersString, "peers", "",
		"comma separated list of peers of the form ip:port")
	flag.BoolVar(&simple, "simple", false,
		"run gossiper in simple broadcast mode")

	flag.Parse()

	if peersString != "" {
		peers = strings.Split(peersString, ",")
	}

	gossiper := gossiper.NewGossiper(gossipAddr, name, uiPort, simple, peers)

	// Listen to client and other gossipers
	go gossiper.AntiEntropy()
	go gossiper.ListenClient()
	gossiper.Listen()
}
