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
	var antiEntropy uint64
	var rtimer uint64
	var n uint64
	var stubbornTimeout uint64
	var hopLimit uint
	var simple bool
	var hw3ex2 bool
	var hw3ex3 bool
	var hw3ex4 bool
	var ackAll bool

	flag.StringVar(&uiPort, "UIPort", "8080", "port for the UI client")
	flag.StringVar(&gossipAddr, "gossipAddr", "127.0.0.1:5000",
		"ip:port for the gossiper")
	flag.StringVar(&name, "name", "", "name of the gossiper")
	flag.StringVar(&peersString, "peers", "",
		"comma separated list of peers of the form ip:port")
	flag.Uint64Var(&antiEntropy, "antiEntropy", 10,
		"use the given timeout in seconds for anti-entropy")
	flag.Uint64Var(&rtimer, "rtimer", 0,
		"timeout in seconds to send route rumors; 0 means disable sending route rumors")
	flag.Uint64Var(&n, "N", 1,
		"total number of peers in the network")
	flag.Uint64Var(&stubbornTimeout, "stubbornTimeout", 5,
		"timeout for BlockPublish messages")
	flag.UintVar(&hopLimit, "hopLimit", 10,
		"hop limit for point-to-point messages")
	flag.BoolVar(&simple, "simple", false,
		"run gossiper in simple broadcast mode")
	flag.BoolVar(&hw3ex2, "hw3ex2", false,
		"run gossiper in HW 3, ex 2 mode")
	flag.BoolVar(&hw3ex3, "hw3ex3", false,
		"run gossiper in HW 3, ex 3 mode")
	flag.BoolVar(&hw3ex4, "hw3ex4", false,
		"run gossiper in HW 3, ex 4 mode")
	flag.BoolVar(&ackAll, "ackAll", false,
		"ack all messages irrespective of their ID (used for HW 3, ex 3)")

	flag.Parse()

	var peers []string = nil
	if peersString != "" {
		peers = strings.Split(peersString, ",")
	}

	// Create directories for file handling
	os.Mkdir("_Downloads", 0755)
	os.Mkdir("_SharedFiles", 0755)

	hw3ex3 = hw3ex3 || hw3ex4
	hw3ex2 = hw3ex2 || hw3ex3
	gossiper := gossiper.NewGossiper(gossipAddr, name, uiPort, peers, n, stubbornTimeout,
		(uint32)(hopLimit), simple, hw3ex2, hw3ex3, hw3ex4, ackAll)

	go web.InitWebServer(gossiper, uiPort)
	gossiper.Run(antiEntropy, rtimer)
}
