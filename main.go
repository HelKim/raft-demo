package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"raft-demo/core"
	"raft-demo/service"
	"time"
)

var (
	httpAddr    = flag.String("svc", "localhost:51000", "service host:port for this node")
	raftId      = flag.String("id", "", "node id used by Raft")
	raftDataDir = flag.String("data", "data/", "raft data dir")
	raftAddr    = flag.String("raft", "localhost:52000", "raft host:port for this node")
	joinAddr    = flag.String("join", "", "join address")
	clientAddr  = flag.String("service_join", "localhost:50000", "raft client port")
)

func main() {
	flag.Parse()

	if *raftId == "" {
		log.Fatalf("raft id is required")
	}
	os.MkdirAll(*raftDataDir, 0700)

	s := core.NewStore()
	s.RaftAddr = *raftAddr
	s.RaftId = *raftId
	s.RaftDataDir = *raftDataDir

	if err := s.StartRaft(*joinAddr == ""); err != nil {
		log.Fatalf("s.StartRaft: %v", err)
	}

	// If join was specified, make the join request.
	if *joinAddr != "" {
		if err := join(*joinAddr, *httpAddr, *raftAddr, *raftId); err != nil {
			log.Fatalf("failed to join node at %s: %s", joinAddr, err.Error())
		}
	} else {
		log.Println("no join addresses set")
	}

	// Wait until the store is in full consensus.
	openTimeout := 120 * time.Second
	s.WaitForLeader(openTimeout)
	s.WaitForApplied(openTimeout)

	if err := s.SetMeta(*raftId, *httpAddr); err != nil && err != core.ErrNotLeader {
		// Non-leader errors are OK, since metadata will then be set through
		// consensus as a result of a join. All other errors indicate a problem.
		log.Fatalf("failed to SetMeta at %s: %s", *raftId, err.Error())
	}
	h := service.New(*httpAddr, s)
	if err := h.Start(); err != nil {
		log.Fatalf("failed to start HTTP service: %s", err.Error())
	}
	b, err := json.Marshal(map[string]string{"serviceAddr": *httpAddr})
	resp, err := http.Post(fmt.Sprintf("http://%s/service_join", *clientAddr), "application-type/json", bytes.NewReader(b))
	if err != nil {
		log.Fatalf("join service to client fail %s", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("join service to client fail %s", err)
	}

	log.Println("started successfully")

	terminate := make(chan os.Signal, 1)
	signal.Notify(terminate, os.Interrupt)
	<-terminate
	log.Println("exiting")
}

func join(joinAddr, httpAddr, raftAddr, nodeID string) error {
	b, err := json.Marshal(map[string]string{"httpAddr": httpAddr, "raftAddr": raftAddr, "id": nodeID})
	if err != nil {
		return err
	}
	resp, err := http.Post(fmt.Sprintf("http://%s/join", joinAddr), "application-type/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
