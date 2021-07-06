package core

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/raft"
)

func (s *Store) Get(k string, level ConsistencyLevel) (string, error) {
	if level != Stale {
		if s.raft.State() != raft.Leader {
			return "", ErrNotLeader
		}
	}

	if level == Consistent {
		if err := s.consistentRead(); err != nil {
			return "", err
		}
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.m[k], nil
}

func (s *Store) Set(k, v string) error {
	if s.raft.State() != raft.Leader {
		return ErrNotLeader
	}

	c := &command{
		Op:    "set",
		Key:   k,
		Value: v,
	}

	b, err := json.Marshal(c)
	if err != nil {
		return err
	}

	f := s.raft.Apply(b, raftTimeout)
	return f.Error()
}

func (s *Store) Delete(key string) error {
	if s.raft.State() != raft.Leader {
		return ErrNotLeader
	}

	c := &command{
		Op:  "delete",
		Key: key,
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}

	f := s.raft.Apply(b, raftTimeout)
	return f.Error()
}

func (s *Store) Join(nodeID, httpAddr, raftAddr string) error {
	s.logger.Printf("received join request for remote node %s at %s", nodeID, raftAddr)

	configFuture := s.raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		s.logger.Printf("failed to get raft configuration: %v", err)
		return err
	}

	for _, srv := range configFuture.Configuration().Servers {
		if srv.ID == raft.ServerID(nodeID) || srv.Address == raft.ServerAddress(raftAddr) {
			if srv.Address == raft.ServerAddress(raftAddr) && srv.ID == raft.ServerID(nodeID) {
				s.logger.Printf("node %s at %s already member of cluster, ignoring join request", nodeID, raftAddr)
				return nil
			}

			future := s.raft.RemoveServer(srv.ID, 0, 0)
			if err := future.Error(); err != nil {
				return fmt.Errorf("error removing existing node %s at %s: %s", nodeID, raftAddr, err)
			}
		}
	}

	f := s.raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(raftAddr), 0, 0)
	if err := f.Error(); err != nil {
		return err
	}

	if err := s.SetMeta(nodeID, httpAddr); err != nil {
		return err
	}

	s.logger.Printf("node %s at %s joined successfully, http addr is %s", nodeID, raftAddr, httpAddr)

	return nil
}

func (s *Store) LeaderAPIAddr() string {
	id, err := s.LeaderID()
	if err != nil {
		return ""
	}

	s.logger.Printf("Leader id: %s", id)

	httpAddr, err := s.GetMeta(id)
	if err != nil {
		return ""
	}

	return httpAddr
}
