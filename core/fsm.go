package core

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/raft"
	"io"
)

type fsm Store

func (f *fsm) Apply(l *raft.Log) interface{} {
	var c command
	if err := json.Unmarshal(l.Data, &c); err != nil {
		panic(fmt.Sprintf("failed to unmarshal command: %s", err.Error()))
	}

	switch c.Op {
	case "set":
		return f.applySet(c.Key, c.Value)
	case "delete":
		return f.applyDelete(c.Key)
	default:
		panic(fmt.Sprintf("unrecognized command op: %s", c.Op))
	}
}

func (f *fsm) applySet(k, v string) interface{} {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.m[k] = v
	return nil
}

func (f *fsm) applyDelete(k string) interface{} {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	delete(f.m, k)
	return nil
}

func (f *fsm) Snapshot() (raft.FSMSnapshot, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	m := make(map[string]string)
	for k, v := range f.m {
		m[k] = v
	}

	return &fsmSnapshot{store: m}, nil
}

func (f *fsm) Restore(rc io.ReadCloser) error {
	m := make(map[string]string)
	if err := json.NewDecoder(rc).Decode(&m); err != nil {
		return err
	}

	f.m = m
	return nil
}

type fsmSnapshot struct {
	store map[string]string
}

func (f *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		b, err := json.Marshal(f.store)
		if err != nil {
			return err
		}

		if _, err := sink.Write(b); err != nil {
			return err
		}

		return sink.Close()
	}()

	if err != nil {
		sink.Close()
	}

	return err
}

func (f *fsmSnapshot) Release() {}
