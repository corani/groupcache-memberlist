package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"text/template"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/mailgun/groupcache/v2"
)

//go:embed templates/*
var files embed.FS

type User struct {
	ID       string
	User     string
	Instance string
}

func newGroup() *groupcache.Group {
	return groupcache.NewGroup("users", 3<<20, groupcache.GetterFunc(
		func(_ context.Context, id string, dest groupcache.Sink) error {
			me, err := os.Hostname()
			if err != nil {
				log.Fatalf("[FATAL] groupcache get hostname: %v", err)
			}

			log.Printf("[INFO] groupcache create user-%s on instance %s", id, me)

			user := User{
				ID:       id,
				User:     fmt.Sprintf("user-%s", id),
				Instance: me,
			}

			bs, err := json.Marshal(user)
			if err != nil {
				log.Fatalf("[FATAL] groupcache marshal result: %v", err)
			}

			// Set the user in the groupcache to expire after one minute
			return dest.SetBytes(bs, time.Now().Add(time.Minute))
		},
	),
	)
}

func indexHandler(group *groupcache.Group) func(http.ResponseWriter, *http.Request) {
	bs, err := files.ReadFile("templates/index.html")
	if err != nil {
		log.Fatalf("[FATAL] app reading template: %v", err)
	}

	index := template.Must(template.New("index").Parse(string(bs)))

	return func(w http.ResponseWriter, r *http.Request) {
		var user User

		if err := r.ParseForm(); err == nil {
			user.ID = r.PostForm.Get("id")

			if user.ID != "" {
				var bs []byte

				ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
				defer cancel()

				if err := group.Get(ctx, user.ID, groupcache.AllocatingByteSliceSink(&bs)); err != nil {
					log.Fatalf("[FATAL] app groupcache get: %v", err)
				}

				if err := json.Unmarshal(bs, &user); err != nil {
					log.Fatalf("[FATAL] app unmarshal: %v", err)
				}
			}
		}

		if err := index.Execute(w, user); err != nil {
			log.Fatalf("[FATAL] app render template: %v", err)
		}
	}
}

func membersChanged(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return true
	}

	for key := range a {
		if _, ok := b[key]; !ok {
			return true
		}
	}

	return false
}

func updateMembers(pool *groupcache.HTTPPool) {
	list, err := memberlist.Create(memberlist.DefaultLocalConfig())
	if err != nil {
		log.Fatalf("[FATAL] memberlist create: %v", err)
	}

	defer list.Shutdown()

	me, err := os.Hostname()
	if err != nil {
		log.Fatalf("[FATAL] memberlist get hostname: %v", err)
	}

	log.Printf("[INFO] app hostname: %v", me)

	if me != "app1" {
		log.Println("[INFO] memberlist trying to join `app1`")

		_, err := list.Join([]string{"app1"})
		if err != nil {
			log.Fatalf("[FATAL] memberlist failed to join cluster: %v", err)
		}
	}

	known := make(map[string]bool)

	for {
		time.Sleep(5 * time.Second)

		current := make(map[string]bool)

		for _, member := range list.Members() {
			current[member.Name] = true
		}

		if membersChanged(known, current) {
			var peers []string

			for member := range current {
				peers = append(peers, fmt.Sprintf("http://%s:8080", member))
			}

			log.Printf("[INFO] app update groupcache peers: %v", peers)
			pool.Set(peers...)
		}

		known = current
	}
}

func newPool() *groupcache.HTTPPool {
	me, err := os.Hostname()
	if err != nil {
		log.Fatalf("[FATAL] groupcache get hostname: %s", err)
	}

	me = fmt.Sprintf("http://%s:8080", me)

	pool := groupcache.NewHTTPPoolOpts(me, &groupcache.HTTPPoolOptions{
		BasePath: "/_groupcache/",
		Transport: func(context.Context) http.RoundTripper {
			return &loggingTransport{http.DefaultTransport}
		},
	})

	return pool
}

func main() {
	groupcache.SetLoggerFromLogger(newLogger())

	pool := newPool()
	group := newGroup()

	http.HandleFunc("/", indexHandler(group))
	http.Handle("/_groupcache/", pool)

	go updateMembers(pool)

	log.Fatal(http.ListenAndServe(":8080", nil))
}
