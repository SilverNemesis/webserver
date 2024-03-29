package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/alexbrainman/sspi"
	"github.com/alexbrainman/sspi/ntlm"
)

type application struct {
	filePath string
}

func (handler application) ServeHTTP(w http.ResponseWriter, r *request) {
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
	var err error
	auth := r.Header.Get("Authorization")
	if auth == "" || (len(strings.SplitN(auth, " ", 2)) < 2) {
		initiateNTLM(w)
		return
	}
	parts := strings.SplitN(auth, " ", 2)
	authType := parts[0]
	if authType != "NTLM" {
		initiateNTLM(w)
		return
	}
	var authPayload []byte
	authPayload, err = base64.StdEncoding.DecodeString(parts[1])
	context, ok := contexts[r.RemoteAddr]
	if !ok {
		sendChallenge(authPayload, w, r)
		return
	}
	defer delete(contexts, r.RemoteAddr)
	var u *user.User
	u, err = authenticate(context, authPayload)
	if err != nil {
		log.Println("auth error:", err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if !authorize(u, r) {
		http.Error(w, u.Username+" is not authorized to do that", http.StatusUnauthorized)
	}
	name := u.Name
	if name == "" {
		name = u.Username
	}
	fmt.Fprint(w, "Hello, "+name)
}

func main() {
	var port string
	if envPort, exists := os.LookupEnv("PORT"); exists {
		port = envPort
	} else {
		port = "8080"
	}

	server := createServer(port)
	go startServer(server)
	fmt.Println("listening on http://localhost:" + port + "/gameoflife")
	fmt.Println("listening on http://localhost:" + port + "/components")
	fmt.Println("listening on http://localhost:" + port + "/user/info")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}

func startServer(server *http.Server) {
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func createServer(port string) *http.Server {
	mux := router{}
	mux.Handle("gameoflife", application{filePath: "gameoflife"})
	mux.Handle("components", application{filePath: "components"})

	userMux := subrouter{}
	userMux.Handle("info", handlerFunc(userInfo))
	mux.Handle("user", userMux)

	return &http.Server{Addr: ":" + port, Handler: mux}
}

func stopServer(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
}

var (
	contexts    map[string]*ntlm.ServerContext
	serverCreds *sspi.Credentials
)

func init() {
	contexts = make(map[string]*ntlm.ServerContext)
	var err error
	serverCreds, err = ntlm.AcquireServerCredentials()
	if err != nil {
		panic(err)
	}
}

func initiateNTLM(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "NTLM")
	http.Error(w, "Authorization required", http.StatusUnauthorized)
	return
}

func authenticate(c *ntlm.ServerContext, authenticate []byte) (u *user.User, err error) {
	defer c.Release()
	err = c.Update(authenticate)
	if err != nil {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	err = c.ImpersonateUser()
	if err != nil {
		return
	}
	defer c.RevertToSelf()
	u, err = user.Current()
	return
}

func authorize(u *user.User, r *request) bool {
	return true
}

func sendChallenge(negotiate []byte, w http.ResponseWriter, r *request) {
	sc, ch, err := ntlm.NewServerContext(serverCreds, negotiate)
	if err != nil {
		http.Error(w, "NTLM error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	contexts[r.RemoteAddr] = sc
	w.Header().Set("WWW-Authenticate", "NTLM "+base64.StdEncoding.EncodeToString(ch))
	http.Error(w, "Respond to challenge", http.StatusUnauthorized)
	return
}
