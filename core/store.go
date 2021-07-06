package core

import (
	"errors"
	"fmt"
	"github.com/hashicorp/raft"
	boltdb "github.com/hashicorp/raft-boltdb"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	retainSnapshotCount = 2
	raftTimeout         = 10 * time.Second
	applyTimeout        = 10 * time.Second
	openTimeout         = 120 * time.Second
	leaderWaitDelay     = 100 * time.Millisecond
	appliedWaitDelay    = 100 * time.Millisecond
)

var (
	// ErrNotLeader is returned when a node attempts to execute a leader-only
	// operation.
	ErrNotLeader = errors.New("not leader")

	// ErrOpenTimeout is returned when the Store does not apply its initial
	// logs within the specified time.
	ErrOpenTimeout = errors.New("timeout waiting for initial logs application")
)

type command struct {
	Op    string `json:"op,omitempty"`
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

type ConsistencyLevel int

const (
	Default ConsistencyLevel = iota
	Stale
	Consistent
)

type Store struct {
	RaftDataDir string
	RaftAddr    string
	RaftId      string

	m     map[string]string
	mutex sync.Mutex

	raft *raft.Raft

	logger *log.Logger
}

func NewStore() *Store {
	return &Store{
		m:      make(map[string]string),
		logger: log.New(os.Stderr, "[store]", log.LstdFlags),
	}
}

func (s *Store) StartRaft(bootstrap bool) error {
	c := raft.DefaultConfig()
	c.LocalID = raft.ServerID(s.RaftId)

	newNode := !pathExists(filepath.Join(s.RaftDataDir, "logs.dat"))

	// 用来存储Raft的日志
	logdb, err := boltdb.NewBoltStore(filepath.Join(s.RaftDataDir, "logs.dat"))
	if err != nil {
		return fmt.Errorf("boltdb.NewBoltStore(%q): %v", filepath.Join(s.RaftDataDir, "logs.dat"), err)
	}

	// 稳定存储，用来存储Raft节点信息，
	// 比如，当前任期编号、最新投票时的任期编号等，持久化存储数据
	stabledb, err := boltdb.NewBoltStore(filepath.Join(s.RaftDataDir, "stable.dat"))
	if err != nil {
		return fmt.Errorf("boltdb.NewBoltStore(%q): %v", filepath.Join(s.RaftDataDir, "stable.dat"), err)
	}

	// Snapshot存储压缩后的日志
	fss, err := raft.NewFileSnapshotStore(s.RaftDataDir, 3, os.Stderr)
	if err != nil {
		return fmt.Errorf(`raft.NewFileSnapshotStore(%q, ...): %v`, s.RaftDataDir, err)
	}

	addr, err := net.ResolveTCPAddr("tcp", s.RaftAddr)
	transport, err := raft.NewTCPTransport(s.RaftAddr, addr, 3, 10*time.Second, os.Stderr)

	ra, err := raft.NewRaft(c, (*fsm)(s), logdb, stabledb, fss, transport)
	if err != nil {
		return fmt.Errorf("raft.NewRaft: %v", err)
	}
	s.raft = ra

	if bootstrap && newNode {
		cfg := raft.Configuration{
			Servers: []raft.Server{
				{
					Suffrage: raft.Voter,
					ID:       raft.ServerID(s.RaftId),
					Address:  raft.ServerAddress(s.RaftAddr),
				},
			},
		}
		f := s.raft.BootstrapCluster(cfg)
		if err := f.Error(); err != nil {
			return fmt.Errorf("raft.Raft.BootstrapCluster: %v", err)
		}
	}

	return nil
}

func (s *Store) consistentRead() error {
	future := s.raft.VerifyLeader()
	if err := future.Error(); err != nil {
		return err
	}

	return nil
}

func (s *Store) SetMeta(key, value string) error {
	return s.Set(key, value)
}

func (s *Store) GetMeta(key string) (string, error) {
	return s.Get(key, Stale)
}

func (s *Store) DeleteMeta(key string) error {
	return s.Delete(key)
}

func (s *Store) LeaderAddr() string {
	return string(s.raft.Leader())
}

// WaitForLeader blocks until a leader is detected, or the timeout expires.
func (s *Store) WaitForLeader(timeout time.Duration) (string, error) {
	tck := time.NewTicker(leaderWaitDelay)
	defer tck.Stop()
	tmr := time.NewTimer(timeout)
	defer tmr.Stop()

	for {
		select {
		case <-tck.C:
			l := s.LeaderAddr()
			if l != "" {
				return l, nil
			}
		case <-tmr.C:
			return "", fmt.Errorf("timeout expired")
		}
	}
}

// WaitForAppliedIndex blocks until a given log index has been applied,
// or the timeout expires.
func (s *Store) WaitForAppliedIndex(idx uint64, timeout time.Duration) error {
	tck := time.NewTicker(appliedWaitDelay)
	defer tck.Stop()
	tmr := time.NewTimer(timeout)
	defer tmr.Stop()

	for {
		select {
		case <-tck.C:
			if s.raft.AppliedIndex() >= idx {
				return nil
			}
		case <-tmr.C:
			return fmt.Errorf("timeout expired")
		}
	}
}

// WaitForApplied waits for all Raft log entries to to be applied to the
// underlying database.
func (s *Store) WaitForApplied(timeout time.Duration) error {
	if timeout == 0 {
		return nil
	}
	s.logger.Printf("waiting for up to %s for application of initial logs", timeout)
	if err := s.WaitForAppliedIndex(s.raft.LastIndex(), timeout); err != nil {
		return ErrOpenTimeout
	}
	return nil
}

// LeaderID returns the node ID of the Raft leader. Returns a
// blank string if there is no leader, or an error.
func (s *Store) LeaderID() (string, error) {
	addr := s.LeaderAddr()

	configFuture := s.raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		s.logger.Printf("failed to get raft configuration: %v", err)
		return "", err
	}

	for _, srv := range configFuture.Configuration().Servers {
		if srv.Address == raft.ServerAddress(addr) {
			return string(srv.ID), nil
		}
	}
	return "", nil
}

// pathExists returns true if the given path exists.
func pathExists(p string) bool {
	if _, err := os.Lstat(p); err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}
