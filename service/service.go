package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"raft-demo/core"
	"strings"
)

type StoreApi interface {
	Get(key string, level core.ConsistencyLevel) (string, error)

	Set(key, value string) error

	Delete(key string) error

	Join(nodeID, httpAddr, raftAddr string) error

	LeaderAPIAddr() string
}

type Service struct {
	addr string
	ln   net.Listener

	store StoreApi

	logger *log.Logger
}

func New(addr string, store StoreApi) *Service {
	return &Service{
		addr:   addr,
		store:  store,
		logger: log.New(os.Stderr, "[Http Service]", log.LstdFlags),
	}
}

func (s *Service) Start() error {
	server := http.Server{
		Handler: s,
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln

	http.Handle("/", s)

	go func() {
		err := server.Serve(s.ln)
		if err != nil {
			log.Fatalf("HTTP serve: %s", err)
		}
	}()

	return nil
}

func (s *Service) Close() {
	s.ln.Close()
	return
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/key") {
		s.handleKeyRequest(w, r)
	} else if r.URL.Path == "/join" {
		s.HandleJoin(w, r)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *Service) HandleJoin(w http.ResponseWriter, r *http.Request) {
	m := map[string]string{}
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	httpAddr, ok := m["httpAddr"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	raftAddr, ok := m["raftAddr"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	nodeID, ok := m["id"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.store.Join(nodeID, httpAddr, raftAddr); err != nil {
		if err == core.ErrNotLeader {
			leader := s.store.LeaderAPIAddr()
			s.logger.Println(leader)
			if leader == "" {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}

			redirect := s.FormRedirect(r, leader)
			http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func level(req *http.Request) (core.ConsistencyLevel, error) {
	q := req.URL.Query()
	lvl := strings.TrimSpace(q.Get("level"))

	switch strings.ToLower(lvl) {
	case "default":
		return core.Default, nil
	case "stale":
		return core.Stale, nil
	case "consistent":
		return core.Consistent, nil
	default:
		return core.Default, nil
	}
}

func (s *Service) handleKeyRequest(w http.ResponseWriter, r *http.Request) {
	getKey := func() string {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 3 {
			return ""
		}
		return parts[2]
	}

	switch r.Method {
	case "GET":
		k := getKey()
		if k == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		lvl, err := level(r)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		v, err := s.store.Get(k, lvl)
		if err != nil {
			if err == core.ErrNotLeader {
				leaderHttpAddr := s.store.LeaderAPIAddr()

				if leaderHttpAddr == "" {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
				redirect := s.FormRedirect(r, leaderHttpAddr)
				http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		b, err := json.Marshal(map[string]string{k: v})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		io.WriteString(w, string(b))

	case "POST":
		// Read the value from the POST body.
		m := map[string]string{}
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for k, v := range m {
			if err := s.store.Set(k, v); err != nil {
				if err == core.ErrNotLeader {
					leader := s.store.LeaderAPIAddr()
					if leader == "" {
						http.Error(w, err.Error(), http.StatusServiceUnavailable)
						return
					}

					redirect := s.FormRedirect(r, leader)
					http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
					return
				}

				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

	case "DELETE":
		k := getKey()
		if k == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := s.store.Delete(k); err != nil {
			if err == core.ErrNotLeader {
				leader := s.store.LeaderAPIAddr()
				if leader == "" {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
				redirect := s.FormRedirect(r, leader)
				http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
				return
			}

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}

	return
}

// FormRedirect returns the value for the "Location" header for a 301 response.
func (s *Service) FormRedirect(r *http.Request, host string) string {
	protocol := "http"
	rq := r.URL.RawQuery
	if rq != "" {
		rq = fmt.Sprintf("?%s", rq)
	}
	return fmt.Sprintf("%s://%s%s%s", protocol, host, r.URL.Path, rq)
}

func (s *Service) Addr() net.Addr {
	return s.ln.Addr()
}
