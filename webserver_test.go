package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/sergi/go-diff/diffmatchpatch"
)

const port = "8080"

func TestMain(m *testing.M) {
	server := createServer(port)
	go startServer(server)
	exitCode := m.Run()
	stopServer(server)
	os.Exit(exitCode)
}

func TestGameOfLife(t *testing.T) {
	client := &http.Client{}
	headers := make(map[string]string)
	testGet(t, client, "http://localhost:"+port+"/gameoflife", headers)
}

func TestComponents(t *testing.T) {
	client := &http.Client{}
	headers := make(map[string]string)
	testGet(t, client, "http://localhost:"+port+"/components", headers)
}

func TestUserInfo(t *testing.T) {
	client := &http.Client{}
	headers := make(map[string]string)
	testGet(t, client, "http://localhost:"+port+"/user/info", headers)
}

func testGet(t *testing.T, client *http.Client, url string, headers map[string]string) {
	req, _ := http.NewRequest("GET", url, nil)
	for k, v := range headers {
		req.Header.Add(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	handleResponse(t, resp, err)
}

func handleResponse(t *testing.T, resp *http.Response, err error) (data map[string]interface{}) {
	if err != nil {
		fmt.Println(err)
	} else {
		defer resp.Body.Close()
		data := resp.Status
		if resp.StatusCode == 200 {
			body, _ := ioutil.ReadAll(resp.Body)
			if len(body) > 0 {
				data += "\n" + string(body)
			}
		}
		same, diff := snapshotEquals(t.Name(), data)
		if !same {
			t.Fatal(diff)
		}
	}
	return
}

func snapshotName(testname string) string {
	return testname + ".snap"
}

func snapshotEquals(testname string, text string) (bool, string) {
	dir := "__snapshots__"
	_, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			err := os.MkdirAll(dir, os.FileMode(0777))
			if err != nil {
				log.Fatal(err)
			}
		} else {
			log.Fatal(err)
		}
	}
	oldData, err := ioutil.ReadFile(path.Join(dir, snapshotName(testname)))
	if err != nil {
		if os.IsNotExist(err) {
			err = ioutil.WriteFile(path.Join(dir, snapshotName(testname)), []byte(text), os.FileMode(0777))
			if err != nil {
				log.Fatal(err)
			}
		} else {
			log.Fatal(err)
		}
	}
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(string(oldData), text, false)
	same := true
	for _, diff := range diffs {
		if diff.Type != diffmatchpatch.DiffEqual {
			same = false
			break
		}
	}
	return same, dmp.DiffPrettyText(diffs)
}
