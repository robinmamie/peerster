package main

import (
	"flag"
	"strings"

	"github.com/robinmamie/Peerster/gossiper"
	"github.com/robinmamie/Peerster/web"
)

const guiPort string = "8080"

func main() {

	// Parse flags
	var uiPort string
	var gossipAddr string
	var name string
	var peers []string = nil
	var simple bool
	var antiEntropy uint64

	var peersString string

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

	flag.Parse()

	if peersString != "" {
		peers = strings.Split(peersString, ",")
	}

	gossiper := gossiper.NewGossiper(gossipAddr, name, uiPort, simple, peers)

	go web.InitWebServer(gossiper, uiPort)
	gossiper.Run(antiEntropy)
}
