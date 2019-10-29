package main

import (
	"flag"
	"os"
	"strings"

	"github.com/robinmamie/Peerster/gossiper"
	"github.com/robinmamie/Peerster/web"
)

func main() {

	// Parse flags
	var uiPort string
	var gossipAddr string
	var name string
	var peersString string
	var simple bool
	var antiEntropy uint64
	var rtimer uint64

	flag.StringVar(&uiPort, "UIPort", "8080", "port for the UI client")
	flag.StringVar(&gossipAddr, "gossipAddr", "127.0.0.1:5000",
		"ip:port for the gossiper")
	flag.StringVar(&name, "name", "", "name of the gossiper")
	flag.StringVar(&peersString, "peers", "",
		"comma separated list of peers of the form ip:port")
	flag.BoolVar(&simple, "simple", false,
		"run gossiper in simple broadcast mode")
	flag.Uint64Var(&antiEntropy, "antiEntropy", 10,
		"use the given timeout in seconds for anti-entropy")
	flag.Uint64Var(&rtimer, "rtimer", 0,
		"timeout in seconds to send route rumors; 0 means disable sending route rumors")

	flag.Parse()

	var peers []string = nil
	if peersString != "" {
		peers = strings.Split(peersString, ",")
	}

	// Create directories for file handling
	os.Mkdir("_Downloads", 0644) // TODO magic string
	os.Mkdir("_SharedFiles", 0644)

	gossiper := gossiper.NewGossiper(gossipAddr, name, uiPort, simple, peers)

	go web.InitWebServer(gossiper, uiPort)
	gossiper.Run(antiEntropy, rtimer)
}
