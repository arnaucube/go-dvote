package net

import (
	"testing"
	"encoding/json"
	"net/http"
	"fmt"
	"time"
	"bytes"
	"io/ioutil"
)

//func generateSubmission() submission {

//}

func TestListen(t *testing.T) {
	t.Log("Testing listener")

	testSubmission := submission {
		"package",
		[]byte("012345678"),
		[]byte("012345678"),
		[]byte("012345678"),
		time.Now(),
	}

	listen("8080")

	url := "http://localhost:8080/submit"
	fmt.Println("URL:>", url)

	j, err := json.Marshal(testSubmission)
	if err != nil {
		t.Errorf("Bad test JSON: %s", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(j))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("Error in client: %s", err)
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))
}
