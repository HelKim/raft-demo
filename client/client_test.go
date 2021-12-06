package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
)

const raftClientAddr = "127.0.0.1:50000"

func TestOnApi(t *testing.T) {
	t.Run("test on Api1", func(t *testing.T) {
		res := doGet(t, "test")
		m := map[string]string{}
		json.Unmarshal([]byte(res), &m)
		assert.Equal(t, "", m["test"])

		doPost(t, "test", "value")

		res = doGet(t, "test")
		json.Unmarshal([]byte(res), &m)
		assert.Equal(t, "value", m["test"])

		doDelete(t, "test")

		res = doGet(t, "test")
		json.Unmarshal([]byte(res), &m)
		assert.Equal(t, "", m["test"])
	})

	t.Run("test on Api2", func(t *testing.T) {
		res := doGet(t, "2")
		m := map[string]string{}
		json.Unmarshal([]byte(res), &m)
		assert.Equal(t, "", m["2"])
	})
}

func doGet(t *testing.T, key string) string {
	resp, err := http.Get(fmt.Sprintf("http://%s/key/%s", raftClientAddr, key))
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

func doPost(t *testing.T, key, value string) {
	b, err := json.Marshal(map[string]string{key: value})
	if err != nil {
		t.Fatalf("failed to encode key and value for POST: %s", err)
	}
	resp, err := http.Post(fmt.Sprintf("http://%s/key", raftClientAddr), "application-type/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST request failed: %s", err)
	}
	defer resp.Body.Close()
}

func doDelete(t *testing.T, key string) {
	ru, err := url.Parse(fmt.Sprintf("http://%s/key/%s", raftClientAddr, key))
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
