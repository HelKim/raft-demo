package main

import (
	"log"
	"os"
	"os/signal"
	"raft-demo/client"
)

func main() {
	raftClient := client.NewRaftClient(":50000")
	err := raftClient.Start()
	if err != nil {
		log.Fatalf("raft client start fail", err.Error())
	}
	terminate := make(chan os.Signal, 1)
	signal.Notify(terminate, os.Interrupt)
	<-terminate
	log.Println("exiting")
}
