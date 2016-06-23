package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/bmizerany/pat"
	"github.com/garyburd/redigo/redis"
	"labix.org/v2/mgo"
)

var (
	RedisAddr    = os.Getenv("REDIS_ADDR")
	FrontDeskURL = os.Getenv("FRONTDESKURL")
	MongoHost    = os.Getenv("MONGODB_HOST")
	SlackWebHook = os.Getenv("SLACK_WEBHOOK")
	MongoDB      = "kettle-watcher"
	Watcher      *Watchers
	RedisPool    *redis.Pool
	MongoPool    *mgo.Session
)

type Event struct {
	UserID  string `json:userID`
	EventID string `json:eventID`
	Token   string `json:token`
}

type Watchers struct {
	WatchLists map[string]*WatchList
	sync.Mutex
}

func init() {

	log.SetLevel(log.DebugLevel)
	if len(RedisAddr) == 0 {
		log.Info("Env REDIS_ADDR is not set. Fallback to localhost")
		RedisAddr = "localhost"
	}
	if len(FrontDeskURL) == 0 {
		log.Info("Env FRONTDESKURL is not set. Fallback to https://lutece.frontdeskhq.com")
		FrontDeskURL = "https://lutece.frontdeskhq.com"
	}
	if len(MongoHost) == 0 {
		log.Info("Env MONGODB_HOST is not set. Fallback to localhost")
		MongoHost = "localhost"
	}
	if len(SlackWebHook) == 0 {
		log.Warn("Env SLACK_WEBHOOK is not set. Notifications will fail.")
	}
	RedisPool = newRedisPool()
	MongoPool = newMongoPool()

	// TODO: get watchlist from MongoDB
	Watcher = &Watchers{WatchLists: make(map[string]*WatchList)}
}

func main() {
	log.Info("Watcher started!")
	c := RedisPool.Get()
	defer c.Close()
	mux := registerRoutes()
	http.Handle("/", mux)
	http.ListenAndServe("localhost:8080", nil)

}

func getWatchList(e *Event) *WatchList {
	Watcher.Lock()
	defer Watcher.Unlock()
	if _, ok := Watcher.WatchLists[e.UserID]; !ok {
		Watcher.WatchLists[e.UserID] = NewWatchList(e, MongoPool)
	}
	return Watcher.WatchLists[e.UserID]
}

func getWatchListFromUserID(userID string) *WatchList {
	Watcher.Lock()
	defer Watcher.Unlock()
	if _, ok := Watcher.WatchLists[userID]; ok {
		return Watcher.WatchLists[userID]
	}
	return nil
}

func registerRoutes() *pat.PatternServeMux {
	mux := pat.New()
	mux.Post("/watchedEvents", http.HandlerFunc(AddEvent))
	mux.Del("/watchedEvent/:resourceID", http.HandlerFunc(delEvent))
	mux.Get("/watchedEvents/:resourceID", http.HandlerFunc(listEvents))
	return mux
}

func AddEvent(w http.ResponseWriter, r *http.Request) {
	event := &Event{}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"status": "error","description":"unable to read body"}`))
		return
	}
	err = json.Unmarshal(body, event)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"status": "error","description":"unable to decode json"}`))
		return
	}
	getWatchList(event).Add(event.EventID)
	w.Write([]byte(`{"status": "success"}\r\n`))
}

func listEvents(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	userID := params.Get(":userID")
	WatchList := getWatchListFromUserID(userID)
	if WatchList == nil {
		w.WriteHeader(404)
		w.Write([]byte(`{"status": "error","description":"user not found"}`))
		return
	}
	json, err := json.Marshal(WatchList.List())
	if err != nil {
		w.WriteHeader(500)
		log.Error("listEvents > Unable to json: ", err)
		return
	}
	w.Write(json)
}

func delEvent(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	userID := params.Get(":id")
	WatchList := getWatchListFromUserID(userID)
	if WatchList == nil {
		w.WriteHeader(404)
		w.Write([]byte(`{"status": "error","description":"user not found"}`))
		return
	}
	json, err := json.Marshal(WatchList.List())
	if err != nil {
		w.WriteHeader(500)
		log.Error("listEvents > Unable to json: ", err)
		return
	}
}

func newRedisPool() *redis.Pool {
	return &redis.Pool{
		MaxIdle:     400,
		IdleTimeout: 10 * time.Second,
		MaxActive:   0,
		Wait:        true,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", RedisAddr+":6379")
			if err != nil {
				panic(err.Error())
			}
			return c, err
		},
	}
}

func newMongoPool() *mgo.Session {
	mongoDBDialInfo := &mgo.DialInfo{
		Addrs:    []string{MongoHost},
		Timeout:  60 * time.Second,
		Database: MongoDB,
	}
	mongoSession, err := mgo.DialWithInfo(mongoDBDialInfo)
	if err != nil {
		log.Fatalf("Unable to create MongoSession: %s\n", err)
	}
	return mongoSession
}
