package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type application struct {
	filePath string
}

func (handler application) ServeHTTP(w http.ResponseWriter, r *request) {
	fmt.Printf("basePath=%v\nsegments=%v\n", r.basePath, r.segments)
	in := r.URL.Path[len(r.basePath):]
	out := filepath.Join(handler.filePath, filepath.Clean(in))
	if fileInfo, err := os.Stat(out); err != nil && os.IsNotExist(err) {
		out = handler.filePath
	} else if fileInfo.IsDir() {
		if _, err := os.Stat(filepath.Join(out, "index.html")); err != nil && os.IsNotExist(err) {
			out = handler.filePath
		}
	}
	http.ServeFile(w, r.Request, out)
}

type request struct {
	*http.Request
	basePath string
	segments []string
}

type handler interface {
	ServeHTTP(http.ResponseWriter, *request)
}

type router struct {
	routes []subroute
}

func (s router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req := request{r, "", strings.FieldsFunc(r.URL.Path, func(c rune) bool { return c == '/' })}
	for i := range s.routes {
		if s.routes[i].pattern == req.segments[0] {
			req.basePath += "/" + req.segments[0]
			req.segments = req.segments[1:]
			s.routes[i].handler.ServeHTTP(w, &req)
			return
		}
	}
}

func (s *router) Handle(pattern string, handler handler) {
	subroute := subroute{pattern: pattern, handler: handler}
	s.routes = append(s.routes, subroute)
}

type subroute struct {
	pattern string
	handler handler
}

type subrouter struct {
	routes []subroute
}

func (s subrouter) ServeHTTP(w http.ResponseWriter, req *request) {
	for i := range s.routes {
		if s.routes[i].pattern == req.segments[0] {
			req.basePath += "/" + req.segments[0]
			req.segments = req.segments[1:]
			s.routes[i].handler.ServeHTTP(w, req)
			return
		}
	}
}

func (s *subrouter) Handle(pattern string, handler handler) {
	subroute := subroute{pattern: pattern, handler: handler}
	s.routes = append(s.routes, subroute)
}

type handlerFunc func(http.ResponseWriter, *request)

func (f handlerFunc) ServeHTTP(w http.ResponseWriter, r *request) {
	f(w, r)
}

func userInfo(w http.ResponseWriter, r *request) {
	fmt.Printf("basePath=%v\nsegments=%v\n", r.basePath, r.segments)
	fmt.Fprint(w, "userInfo")
}

func main() {
	mux := router{}
	mux.Handle("gameoflife", application{filePath: "gameoflife"})
	mux.Handle("components", application{filePath: "components"})

	userMux := subrouter{}
	userMux.Handle("info", handlerFunc(userInfo))
	mux.Handle("user", userMux)

	fmt.Println("listening on http://localhost:8080/gameoflife")
	fmt.Println("listening on http://localhost:8080/components")

	test := false

	if test {
		go func() {
			err := http.ListenAndServe(":8080", mux)
			log.Fatal(err)
		}()

		client := &http.Client{}
		headers := make(map[string]string)

		testGet(client, "http://localhost:8080/gameoflife", headers)
		testGet(client, "http://localhost:8080/components", headers)
		testGet(client, "http://localhost:8080/user/info", headers)
		testGet(client, "http://localhost:8080/user/info/1", headers)
	} else {
		err := http.ListenAndServe(":8080", mux)
		log.Fatal(err)
	}
}

func testGet(client *http.Client, url string, headers map[string]string) {
	fmt.Println(url)
	req, _ := http.NewRequest("GET", url, nil)
	for k, v := range headers {
		req.Header.Add(k, v)
	}
	handleResponse(client.Do(req))
}

func handleResponse(resp *http.Response, err error) (data map[string]interface{}) {
	if err != nil {
		fmt.Println(err)
	} else {
		defer resp.Body.Close()
		fmt.Println(resp.Status)
		if resp.StatusCode == 200 {
			body, _ := ioutil.ReadAll(resp.Body)
			if len(body) > 0 {
				fmt.Println(string(body))
			}
		}
	}
	return
}
