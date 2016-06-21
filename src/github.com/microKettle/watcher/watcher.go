package main

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"labix.org/v2/mgo"
)

var (
	RedisAddr    = os.Getenv("REDIS_ADDR")
	RedisQueue   = os.Getenv("REDIS_QUEUE")
	FrontDeskURL = os.Getenv("FRONTDESKURL")
	MongoHost    = os.Getenv("MONGODB_HOST")
	SlackWebHook = os.Getenv("SLACK_WEBHOOK")
	MongoDB      = "kettle-watcher"
	RedisPool    *redis.Pool
	MongoPool    *mgo.Session
)

type QueuedEvent struct {
	UserID  string `json:userID`
	EventID string `json:eventID`
	Token   string `json:token`
}

func init() {
	log.SetLevel(log.DebugLevel)
	if len(RedisAddr) == 0 {
		log.Info("Env REDIS_ADDR is not set. Fallback to localhost")
		RedisAddr = "localhost"
	}
	if len(RedisQueue) == 0 {
		log.Info("Env REDIS_QUEUE is not set. Fallback to watcher_queue")
		RedisQueue = "watcher_queue"
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
}

func main() {
	log.Info("Watcher started!")
	c := RedisPool.Get()
	defer c.Close()
	queuedEvents := make(chan *QueuedEvent, 100)
	wgDispatch := new(sync.WaitGroup)
	wgDispatch.Add(2)
	go watchQueue(c, queuedEvents)
	go dispatchEvents(queuedEvents)
	wgDispatch.Wait()
}

func watchQueue(c redis.Conn, ch chan *QueuedEvent) {
	for {

		redisEvents, err := redis.Strings(c.Do("BLPOP", RedisQueue, 0))
		if err != nil {
			log.Fatal("BLPOP error..", err)
		}
		queuedEvent := QueuedEvent{}
		err = json.Unmarshal([]byte(redisEvents[1]), &queuedEvent)
		if err != nil {
			log.Errorf("WatchQueue > Skipping this record. Unable to unmarshal json: %s, %s", redisEvents[1], err)
			continue
		}
		ch <- &queuedEvent
	}
}

func dispatchEvents(in chan *QueuedEvent) {

	users := make(map[string]*User)
	for queuedEvent := range in {
		log.Info("DispatchEvents > ReceivedEvents: ", queuedEvent)
		if _, ok := users[queuedEvent.UserID]; !ok {
			users[queuedEvent.UserID] = NewUser(queuedEvent, MongoPool)
		}
		// updating token just in case
		users[queuedEvent.UserID].Token = queuedEvent.Token
		// Sending event
		users[queuedEvent.UserID].AddWatchedEvent(queuedEvent.EventID)
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
