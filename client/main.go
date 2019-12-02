package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/robinmamie/Peerster/files"

	"github.com/robinmamie/Peerster/messages"
	"github.com/robinmamie/Peerster/tools"

	"github.com/dedis/protobuf"
)

func main() {
	var uiPort string
	var textMsg string
	var dest string
	var file string
	var request string
	var keywords string
	var budget uint64
	flag.StringVar(&uiPort, "UIPort", "8080", "port for the UI client")
	flag.StringVar(&textMsg, "msg", "", "message to be sent; if the -dest flag is present, this is a private message, otherwise it's a rumor message")
	flag.StringVar(&dest, "dest", "", "destination for the private message; can be omitted")
	flag.StringVar(&file, "file", "", "file to be indexed by the gossiper")
	flag.StringVar(&request, "request", "", "request a chunk or metafile of this hash")
	flag.StringVar(&keywords, "keywords", "", "keywords used for the file search")
	flag.Uint64Var(&budget, "budget", 2, "budget used for the file search")
	flag.Parse()

	checkFlags(textMsg, dest, file, request)

	var byteRequest []byte = nil
	if request != "" {
		byteRequest = checkRequest(request)
	}

	var splitKeywords []string = nil
	if keywords != "" {
		splitKeywords = strings.Split(keywords, ",")
	}

	// Create and encode packet
	msg := messages.Message{
		Text:        textMsg,
		Destination: &dest,
		File:        &file,
		Request:     &byteRequest,
		Keywords:    &splitKeywords,
		Budget:      budget,
	}
	packetBytes, err := protobuf.Encode(&msg)
	tools.Check(err)

	// Create local UDP connection
	conn, err := net.Dial("udp4", ":"+uiPort)
	tools.Check(err)

	// Send packet and close connection
	bytes, err := conn.Write(packetBytes)
	tools.Check(err)
	if bytes != len(packetBytes) {
		log.Fatal(bytes, "bytes were sent instead of", len(packetBytes),
			"bytes.")
	}

	err = conn.Close()
	tools.Check(err)
}

// checkFlags checks if the flag combination is a valid one. If not, it panics.
func checkFlags(textMsg string, dest string, file string, request string) {
	textDefined := textMsg != ""
	destDefined := dest != ""
	fileDefined := file != ""
	requestDefined := request != ""

	// Destination defined without anything else
	impossibleCombination := destDefined && !textDefined && !fileDefined && !requestDefined
	// Text and file/request defined
	impossibleCombination = impossibleCombination || (textDefined && (fileDefined || requestDefined))
	// Request defined without file name
	impossibleCombination = impossibleCombination || (requestDefined && !fileDefined)
	// Destintation defined when only file defined
	impossibleCombination = impossibleCombination || (fileDefined && destDefined && !requestDefined)

	panicIfTrue(impossibleCombination, "ERROR (Bad argument combination)")
}

// checkRequest checks if the request has the right format. If not, it panics.
func checkRequest(request string) []byte {
	byteRequest, err := hex.DecodeString(request)
	invalidRequest := err != nil
	// Request has not the required size
	invalidRequest = invalidRequest || len(request) != files.SHA256ByteSize*2
	// Parsed request has not the required size
	invalidRequest = invalidRequest || len(byteRequest) != files.SHA256ByteSize
	panicIfTrue(invalidRequest, "ERROR (Unable to decode hex hash)")
	return byteRequest
}

// panicIfTrue panics if the given condition is true.
func panicIfTrue(b bool, errorMessage string) {
	if b {
		fmt.Println(errorMessage)
		os.Exit(1)
	}
}
