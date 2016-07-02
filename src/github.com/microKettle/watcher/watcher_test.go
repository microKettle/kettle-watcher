package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

func setUp() {
	mgoSession := MongoPool.Copy()
	defer mgoSession.Close()
	collection := mgoSession.DB(MongoDB).C(MongoCollection)
	err := collection.DropCollection()
	if err != nil {
		log.Fatal("Unable to delete collection")
	}
	err = collection.Create(&mgo.CollectionInfo{})
	if err != nil {
		log.Fatal("Unable to create collection")
	}
}

func tearDown() {

}

func TestPost(t *testing.T) {
	setUp()
	req, _ := http.NewRequest("POST", "?:userID=15", bytes.NewBuffer([]byte(`{"eventId":3000}`)))
	w := httptest.NewRecorder()
	mockHTTP := http.HandlerFunc(AddEvent)
	mockHTTP.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Error("Expecting 201 got ", w.Code)
	}
	mgoSession := MongoPool.Copy()
	defer mgoSession.Close()
	collection := mgoSession.DB(MongoDB).C(MongoCollection)
	nb, err := collection.Find(bson.M{"EventID": 3000, "UserID": 15}).Count()
	if err != nil {
		t.Error("Expecting Find to actually works, got ", err)
	}
	if nb != 1 {
		t.Error("Expecting 1 record got ", nb)
	}
	body, _ := ioutil.ReadAll(w.Body)
	if string(body) != `{"event":3000}` {
		t.Error(`Expecting {"event":3000} got `, string(body))
	}
}

func TestGet(t *testing.T) {
	setUp()
	mgoSession := MongoPool.Copy()
	defer mgoSession.Close()

	watchList := WatchList{User: &User{UserID: 16}, Mgo: mgoSession}
	Watcher.Users[16] = &watchList
	watchList.Add(4000)
	watchList.Add(4001)
	watchList.Add(4002)
	req, _ := http.NewRequest("GET", "?:userID=16", nil)
	w := httptest.NewRecorder()
	mockHTTP := http.HandlerFunc(ListEvents)
	mockHTTP.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Error("Expecting 200 got ", w.Code)
	}
	body, _ := ioutil.ReadAll(w.Body)
	expectedResponse := `{"events":[4000,4001,4002]}`
	if string(body) != expectedResponse {
		t.Error(`Expecting {"events":[4000,4001,4002]} got`, string(body))
	}

}

func TestDelete(t *testing.T) {
	setUp()
	mgoSession := MongoPool.Copy()
	defer mgoSession.Close()
	watchList := WatchList{User: &User{UserID: 17}, Mgo: mgoSession}
	Watcher.Users[17] = &watchList
	watchList.Add(4000)
	req, _ := http.NewRequest("DELETE", "?:userID=17&:eventID=4000", nil)
	w := httptest.NewRecorder()
	mockHTTP := http.HandlerFunc(DeleteEvent)
	mockHTTP.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Error("Expecting 200 got ", w.Code, w.Body)
	}

	body, _ := ioutil.ReadAll(w.Body)
	if string(body) != `{"event":4000}` {
		t.Error(`Expecting {"event":4000} got `, string(body))
	}
}
