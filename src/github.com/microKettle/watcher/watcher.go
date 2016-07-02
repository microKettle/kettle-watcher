package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/bmizerany/pat"
	"github.com/garyburd/redigo/redis"
	"labix.org/v2/mgo"
)

var (
	KettleFrontDesk = os.Getenv("KETTLE_FRONTDESK")
	MongoHost       = os.Getenv("MONGODB_HOST")
	SlackWebHook    = os.Getenv("SLACK_WEBHOOK")
	MongoDB         = "kettle-watcher"
	Watcher         *Watchers
	RedisPool       *redis.Pool
	MongoPool       *mgo.Session
)

type Event struct {
	UserID  int
	EventID int `json:"eventId"`
	Token   string
}

type Events struct {
	Events []int `json:"events"`
}

type Watchers struct {
	Users map[int]*WatchList
	sync.Mutex
}

func init() {

	log.SetLevel(log.InfoLevel)
	if len(KettleFrontDesk) == 0 {
		log.Info("Env KETTLE_FRONTDESK is not set. Fallback to https://localhost")
		KettleFrontDesk = "https://localhost"
	}
	if len(MongoHost) == 0 {
		log.Info("Env MONGODB_HOST is not set. Fallback to localhost")
		MongoHost = "localhost"
	}
	if len(SlackWebHook) == 0 {
		log.Warn("Env SLACK_WEBHOOK is not set. Notifications will fail.")
	}
	MongoPool = newMongoPool()

	// TODO: get watchlist from MongoDB
	Watcher = &Watchers{Users: make(map[int]*WatchList)}
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
	if _, ok := Watcher.Users[e.UserID]; !ok {
		Watcher.Users[e.UserID] = NewWatchList(e, MongoPool)
	}
	return Watcher.Users[e.UserID]
}

func getWatchListFromUserID(userID int) *WatchList {
	Watcher.Lock()
	defer Watcher.Unlock()
	if _, ok := Watcher.Users[userID]; ok {
		return Watcher.Users[userID]
	}
	return nil
}

func registerRoutes() *pat.PatternServeMux {
	mux := pat.New()
	mux.Post("/users/:userID/events", http.HandlerFunc(AddEvent))
	mux.Get("/users/:userID/events", http.HandlerFunc(ListEvents))
	mux.Del("/users/:userID/events/:eventID", http.HandlerFunc(DeleteEvent))
	return mux
}

func AddEvent(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	userID := params.Get(":userID")
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"status": "error","description":"unable to read body"}`))
		return
	}
	event := &Event{}
	err = json.Unmarshal(body, event)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"status": "error","description":"unable to decode json"}`))
		return
	}
	uid, err := strconv.Atoi(userID)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"status": "error", "description":"unable to cast userID to int"}`))
	}
	event.UserID = uid
	getWatchList(event).Add(event.EventID)
	w.WriteHeader(201)
	w.Write([]byte(`{"event":` + strconv.Itoa(event.EventID) + `}`))
}

func ListEvents(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	userID := params.Get(":userID")
	uid, err := strconv.Atoi(userID)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"status": "error", "description":"unable to cast userID to int"}`))
	}
	watchList := getWatchListFromUserID(uid)
	if watchList == nil {
		w.WriteHeader(404)
		w.Write([]byte(`{"status": "error","description":"user not found"}`))
		return
	}

	json, err := json.Marshal(&Events{Events: watchList.List()})
	if err != nil {
		w.WriteHeader(500)
		log.Error("listEvents > Unable to json: ", err)
		return
	}

	w.Write(json)
}

func DeleteEvent(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	userID := params.Get(":userID")
	reqEventID := params.Get(":eventID")
	uid, err := strconv.Atoi(userID)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"status": "error", "description":"unable to cast userID to int"}`))
	}
	eventID, err := strconv.Atoi(reqEventID)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"error": "EventID is not an integer"}`))
	}
	watchList := getWatchListFromUserID(uid)
	if watchList == nil {
		w.WriteHeader(404)
		w.Write([]byte(`{"status": "error","description":"user not found"}`))
		return
	}
	err = watchList.Delete(eventID)
	if err != nil && err.Error() == "not found" {
		w.WriteHeader(404)
		w.Write([]byte(`{"status": "error","` + err.Error() + `"}`))
		return
	}
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"status": "error","` + err.Error() + `"}`))
		return
	}
	w.Write([]byte(`{"event":` + strconv.Itoa(eventID) + `}`))
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
