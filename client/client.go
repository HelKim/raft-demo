package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const maxFailOnRequestOnService int64 = 5

var (
	// ErrNoAvailableService is returned when there is no service available
	ErrNoAvailableService = errors.New("no service available")
)

type RaftClient struct {
	h        *http.Client
	addr     string
	services map[string]int64
	ln       net.Listener
	logger   *log.Logger
}

func NewRaftClient(addr string) *RaftClient {
	return &RaftClient{
		h:        &http.Client{},
		addr:     addr,
		services: map[string]int64{},
	}
}

func (r *RaftClient) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	getKey := func() string {
		parts := strings.Split(req.URL.Path, "/")
		if len(parts) != 3 {
			return ""
		}
		return parts[2]
	}
	if strings.HasPrefix(req.URL.Path, "/key") {
		switch req.Method {
		case "GET":
			k := getKey()
			if k == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			v, err := r.doGet(k)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			b, err := json.Marshal(map[string]string{k: v})
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			io.WriteString(w, string(b))
		case "POST":
			m := map[string]string{}
			if err := json.NewDecoder(req.Body).Decode(&m); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			err := r.doPost(m)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			}
		case "DELETE":
			k := getKey()
			if k == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			r.doDelete(k)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	} else if strings.HasPrefix(req.URL.Path, "/service_join") {
		r.handleServiceJoin(w, req)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (r *RaftClient) handleServiceJoin(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "POST":
		m := map[string]string{}
		if err := json.NewDecoder(req.Body).Decode(&m); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if addr, ok := m["serviceAddr"]; !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		} else {
			r.services[addr] = 0
			log.Printf("service join addr: %s", addr)
			w.WriteHeader(http.StatusOK)
			return
		}
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
	}

}

func (r *RaftClient) Start() error {
	if len(r.addr) == 0 {
		log.Fatalf("raft client addr is required")
	}
	server := http.Server{
		Handler: r,
	}
	ln, err := net.Listen("tcp", r.addr)
	if err != nil {
		log.Printf("init listener fail")
		return err
	}
	r.ln = ln

	http.Handle("/", r)
	go func() {
		err := server.Serve(r.ln)
		if err != nil {
			log.Fatalf("HTTP serve: %s", err)
		}
	}()
	return nil
}

func (r *RaftClient) addService(addr string) {
	r.services[addr] = 0
}

func (r *RaftClient) removeService(addr string) {
	delete(r.services, addr)
}

func (r *RaftClient) doGet(key string) (string, error) {
	for serviceAddr := range r.services {
		resp, err := r.h.Get(fmt.Sprintf("http://%s/key/%s", serviceAddr, key))
		if err != nil {
			log.Println(err.Error())
			if r.services[serviceAddr] > maxFailOnRequestOnService {
				r.removeService(serviceAddr)
			} else {
				r.services[serviceAddr]++
			}
			continue
		}
		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("failed to read response: %s", err)
			continue
		}
		m := map[string]string{}
		err = json.Unmarshal(body, &m)
		if err != nil {
			log.Printf("failed to unmarshal response: %s", err)
			continue
		}
		return m[key], nil
	}
	return "", ErrNoAvailableService
}

func (r *RaftClient) doPost(m map[string]string) error {
	b, err := json.Marshal(m)
	if err != nil {
		log.Printf("failed to encode key and value for POST: %s", err)
		return err
	}
	for serviceAddr := range r.services {
		resp, err := r.h.Post(fmt.Sprintf("http://%s/key", serviceAddr), "application-type/json", bytes.NewReader(b))
		if err != nil {
			log.Printf("failed to encode key and value for POST: %s", err)
			if r.services[serviceAddr] > maxFailOnRequestOnService {
				r.removeService(serviceAddr)
			} else {
				r.services[serviceAddr]++
			}
			continue
		}
		resp.Body.Close()
		return nil
	}
	return ErrNoAvailableService

}

func (r *RaftClient) doDelete(key string) {
	for serviceAddr := range r.services {
		ru, err := url.Parse(fmt.Sprintf("http://%s/key/%s", serviceAddr, key))
		if err != nil {
			log.Printf("failed to parse URL for delete: %s", err)
			continue
		}
		req := &http.Request{
			Method: "DELETE",
			URL:    ru,
		}
		resp, err := r.h.Do(req)
		if err != nil {
			log.Printf("failed to GET key: %s", err)
			if r.services[serviceAddr] > maxFailOnRequestOnService {
				r.removeService(serviceAddr)
			} else {
				r.services[serviceAddr]++
			}
		}
		defer resp.Body.Close()
	}
}
