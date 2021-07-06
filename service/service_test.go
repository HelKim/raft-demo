package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
)

const (
	node1 = "http://localhost:51000"
	node2 = "http://localhost:51001"
	node3 = "http://localhost:51002"
)

func TestApi(t *testing.T) {
	res := doGet(t, node1, "test")
	fmt.Println(res)

	doPost(t, node2, "test", "value")

	res = doGet(t, node3, "test")
	fmt.Println(res)

	doDelete(t, node1, "test")

	res = doGet(t, node2, "test")
	fmt.Println(res)
}

func TestApi2(t *testing.T)  {
	res := doGet(t, node1, "2")
	fmt.Println(res)
}

func doGet(t *testing.T, url, key string) string {
	resp, err := http.Get(fmt.Sprintf("%s/key/%s", url, key))
	if err != nil {
		t.Fatalf("failed to GET key: %s", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response: %s", err)
	}
	return string(body)
}

func doPost(t *testing.T, url, key, value string) {
	b, err := json.Marshal(map[string]string{key: value})
	if err != nil {
		t.Fatalf("failed to encode key and value for POST: %s", err)
	}
	resp, err := http.Post(fmt.Sprintf("%s/key", url), "application-type/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST request failed: %s", err)
	}
	defer resp.Body.Close()
}

func doDelete(t *testing.T, u, key string) {
	ru, err := url.Parse(fmt.Sprintf("%s/key/%s", u, key))
	if err != nil {
		t.Fatalf("failed to parse URL for delete: %s", err)
	}
	req := &http.Request{
		Method: "DELETE",
		URL:    ru,
	}

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to GET key: %s", err)
	}
	defer resp.Body.Close()
}
