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
	ID     string `bson:"EventID"`
	UserID string `bson:"UserID"`
}

func NewWatchList(e *Event, m *mgo.Session) *WatchList {
	log.Info("Creating WatchList for User: ", e.UserID)
	user := &User{ID: e.UserID, Token: e.Token}
	watchList := &WatchList{User: user, Mgo: m}
	go watchList.Polling()
	return watchList
}

func (w *WatchList) Add(eventID string) {
	mgoSession := w.Mgo.Copy()
	defer mgoSession.Close()
	// Add mongodb doc for each watched event
	collection := mgoSession.DB(MongoDB).C(MongoCollection)
	if nb, _ := collection.Find(bson.M{"UserID": w.User.ID, "EventID": eventID}).Count(); nb > 0 {
		log.Warnf("AddWatchedEvent > %s Record (%s) already watched. Discarding.", w.User.ID, eventID)
		return
	}
	err := collection.Insert(&WatchedEvent{ID: eventID, UserID: w.User.ID})
	if err != nil {
		log.Errorf("AddWatchedEvent > %s Unable to insert (%s): %s. ", w.User.ID, eventID, err)
		return
	}
	log.Debugf("AddWatchedEvent > %s successfuly added event %s", w.User.ID, eventID)
}

func (w *WatchList) List() []WatchedEvent {
	var watchedEvents []WatchedEvent
	collection := w.Mgo.DB(MongoDB).C(MongoCollection)
	err := collection.Find(bson.M{"UserID": w.User.ID}).All(&watchedEvents)
	if err != nil {
		log.Error("Polling > %s Error during find %s ", w.User.ID, err)
	}
	return watchedEvents
}

func (w *WatchList) Polling() {
	sessionCopy := w.Mgo.Copy()
	defer sessionCopy.Close()
	for {
		watchedEvents := w.List()
		for _, event := range watchedEvents {
			enrollments, err := GetEventEnrollmentEligibility(w.User, event.ID)
			if err != nil {
				log.Error("Polling > Skipping: ", err)
				continue
			}
			w.cleanUpEnrollments(enrollments, event.ID)
			if enrollments.EnrollmentEligibilities[0].CanEnroll {
				log.Infof("Polling > %s eventID Open: %s (%d)", w.User.ID, event.ID)
				w.User.slackNotification(event.ID)
			}
			time.Sleep(10 * time.Second)
		}
		time.Sleep(30 * time.Second)
	}
}

func (w *WatchList) Delete(eventID string) error {
	mgoSession := w.Mgo.Copy()
	defer mgoSession.Close()
	collection := mgoSession.DB(MongoDB).C(MongoCollection)
	err := collection.Remove(bson.M{"UserID": w.User.ID, "EventID": eventID})
	return err
}

func (w *WatchList) cleanUpEnrollments(enrollments *FrontDeskEnrollments, eventID string) {
	for _, restriction := range enrollments.EnrollmentEligibilities[0].Restrictions {
		if restriction.Code == "already_enrolled" || restriction.Code == "in_the_past" {
			err := w.Delete(eventID)
			if err != nil {
				log.Error("cleanUpEnrollments > Unable to remove document ", eventID)
				continue
			}
			log.Infof("cleanUpEnrollments > %s Successfuly removed %s from watchList. Reason: %s", w.User.ID, eventID, restriction.Code)
		}
	}
}
