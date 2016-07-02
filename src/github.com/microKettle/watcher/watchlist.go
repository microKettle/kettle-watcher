package main

import (
	"time"

	log "github.com/Sirupsen/logrus"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

const MongoCollection = "watchedevents"

type WatchList struct {
	User *User
	Mgo  *mgo.Session
}

type WatchedEvent struct {
	ID      bson.ObjectId `json:"id" bson:"_id,omitempty"`
	EventID int           `json:"eventID" bson:"EventID"`
	UserID  int           `json:"userID" bson:"UserID"`
}

func NewWatchList(e *Event, m *mgo.Session) *WatchList {
	log.Info("Creating WatchList for User: ", e.UserID)
	user := &User{UserID: e.UserID}
	watchList := &WatchList{User: user, Mgo: m}
	go watchList.polling()
	return watchList
}

func (w *WatchList) Add(eventID int) {
	mgoSession := w.Mgo.Copy()
	defer mgoSession.Close()
	// Add mongodb doc for each watched event
	collection := mgoSession.DB(MongoDB).C(MongoCollection)
	if nb, _ := collection.Find(bson.M{"UserID": w.User.UserID, "EventID": eventID}).Count(); nb > 0 {
		log.Warnf("AddWatchedEvent > %s Record (%d) already watched. Discarding.", w.User.UserID, eventID)
		return
	}
	err := collection.Insert(&WatchedEvent{UserID: w.User.UserID, EventID: eventID})
	if err != nil {
		log.Errorf("AddWatchedEvent > %s Unable to insert (%d): %s. ", w.User.UserID, eventID, err)
		return
	}
	log.Debugf("AddWatchedEvent > %s successfuly added event %s", w.User.UserID, eventID)
	return
}

func (w *WatchList) List() []int {
	var watchedEvents []WatchedEvent
	collection := w.Mgo.DB(MongoDB).C(MongoCollection)
	err := collection.Find(bson.M{"UserID": w.User.UserID}).All(&watchedEvents)
	if err != nil {
		log.Error("Polling > %d Error during find %s ", w.User.UserID, err)
	}
	arrayOfEvents := make([]int, len(watchedEvents))
	for i, event := range watchedEvents {
		arrayOfEvents[i] = event.EventID
	}
	return arrayOfEvents
}

func (w *WatchList) polling() {
	sessionCopy := w.Mgo.Copy()
	defer sessionCopy.Close()
	for {
		watchedEvents := w.List()
		for _, eventID := range watchedEvents {
			enrollments, err := GetEventEnrollmentEligibility(w.User, eventID)
			if err != nil {
				log.Error("Polling > Skipping: ", err)
				continue
			}
			w.cleanUpEnrollments(enrollments, eventID)
			if enrollments.EnrollmentEligibilities[0].CanEnroll {
				log.Infof("Polling > %d eventID Open: %s (%d)", w.User.UserID, eventID)
				w.User.slackNotification(eventID)
			}
			time.Sleep(10 * time.Second)
		}
		time.Sleep(30 * time.Second)
	}
}

func (w *WatchList) Delete(eventID int) error {
	mgoSession := w.Mgo.Copy()
	defer mgoSession.Close()
	collection := mgoSession.DB(MongoDB).C(MongoCollection)
	err := collection.Remove(bson.M{"UserID": w.User.UserID, "EventID": eventID})
	if err != nil {
		log.Errorf("Delete > Unable to remove document %d: %s ", eventID, err)
	}
	return err
}

func (w *WatchList) cleanUpEnrollments(enrollments *FrontDeskEnrollments, eventID int) {
	for _, restriction := range enrollments.EnrollmentEligibilities[0].Restrictions {
		if restriction.Code == "already_enrolled" || restriction.Code == "in_the_past" {
			err := w.Delete(eventID)
			if err != nil {
				log.Errorf("cleanUpEnrollments > Unable to remove document %d: %s", eventID, err)
				continue
			}
			log.Infof("cleanUpEnrollments > %s Successfuly removed %d from watchList. Reason: %s", w.User.UserID, eventID, restriction.Code)
		}
	}
}
