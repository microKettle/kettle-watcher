package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	log "github.com/Sirupsen/logrus"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

const COLLECTION_EVENTS = "watchedevents"

type FrontDeskEnrollments struct {
	EnrollmentEligibilities []struct {
		CanEnroll    bool `json:"can_enroll"`
		PersonID     int  `json:"person_id"`
		Restrictions []struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"restrictions"`
	} `json:"enrollment_eligibilities"`
}

type User struct {
	ID    string
	Token string
	Mgo   *mgo.Session
}

type Event struct {
	ID     string `bson:"EventID"`
	UserID string `bson:"UserID"`
}

func NewUser(q *QueuedEvent, m *mgo.Session) *User {
	log.Info("dispatchEvents > Adding User ", q.UserID)
	user := &User{Token: q.Token, ID: q.UserID, Mgo: m}
	go user.Polling()
	return user
}

func (u *User) AddWatchedEvent(eventID string) {
	// Add mongodb doc for each watched event
	sessionCopy := u.Mgo.Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(MongoDB).C(COLLECTION_EVENTS)
	if nb, _ := collection.Find(bson.M{"UserID": u.ID, "EventID": eventID}).Count(); nb > 0 {
		log.Warnf("AddWatchedEvent > %s Record (%s) already watched. Discarding.", u.ID, eventID)
		return
	}
	err := collection.Insert(&Event{ID: eventID, UserID: u.ID})
	if err != nil {
		log.Errorf("AddWatchedEvent > %s Unable to insert (%s): %s. ", u.ID, eventID, err)
		return
	}
	log.Debugf("AddWatchedEvent > %s successfuly added event %s", u.ID, eventID)
}

func (u *User) Polling() {
	sessionCopy := u.Mgo.Copy()
	defer sessionCopy.Close()
	for {
		var watchEvents []Event
		collection := sessionCopy.DB(MongoDB).C(COLLECTION_EVENTS)
		err := collection.Find(bson.M{"UserID": u.ID}).All(&watchEvents)
		if err != nil {
			log.Error("Polling > %s Error during find %s ", u.ID, err)
		}
		for _, event := range watchEvents {
			enrollments, err := u.getEventEnrollmentEligibility(event.ID)
			if err != nil {
				continue
			}
			for _, restriction := range enrollments.EnrollmentEligibilities[0].Restrictions {

				log.Debug(restriction.Code)
				if restriction.Code == "already_enrolled" || restriction.Code == "in_the_past" {
					err := collection.Remove(bson.M{"UserID": u.ID, "EventID": event.ID})
					if err != nil {
						log.Error("Polling Unable to remove document ", event.ID)
						continue
					}
					log.Infof("Polling > %s Successfuly removed %s from watchList. Reason: %s", u.ID, event.ID, restriction.Code)
				}
			}
			if enrollments.EnrollmentEligibilities[0].CanEnroll {
				log.Infof("Polling > %s eventID Open: %s (%d)", u.ID, event.ID)
			}
			u.slackNotification(event.ID)
			time.Sleep(10 * time.Second)
		}
		time.Sleep(30 * time.Second)
	}
}

func (u User) getEventEnrollmentEligibility(eventID string) (*FrontDeskEnrollments, error) {
	client := &http.Client{}
	url := FrontDeskURL + "/api/v2/front/event_occurrences/" + eventID + "/enrollment_eligibilities"
	log.Debugf("Polling > %s url: %s", u.ID, url)
	req, err := http.NewRequest("GET", url, nil)
	req.Header.Add("Authorization", " Bearer "+u.Token)
	log.Debug("getEnrollmentEligibility > request: ", req)
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("Polling > Unable to fetch for user %s, err: %s", u.ID, err)
		return nil, err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Polling > %s Unable to readBody err: %s", u.ID, err)
		return nil, err
	}
	if resp.StatusCode != 200 {
		log.Errorf("Polling > %s Frontdesk replied with Statuscode: %d (%s)", u.ID, resp.StatusCode, body)
		return nil, errors.New("Bad Status code")
	}
	f := &FrontDeskEnrollments{}
	err = json.Unmarshal(body, f)
	if err != nil {
		log.Errorf("Polling > %s Unable to unmarshal json: %s", u.ID, err)
		return nil, err
	}
	resp.Body.Close()
	return f, nil
}

type SlackMessage struct {
	Channel    string `json:"channel"`
	Text       string `json:"text"`
	Username   string `json:"username"`
	Icon_emoji string `json:"icon_emoji"`
}

func (u *User) slackNotification(eventID string) {
	// TODO: Create microservice for notification
	m := &SlackMessage{Channel: "#general", Text: eventID + " open!", Username: "Kettle", Icon_emoji: ":computer:"}
	b, _ := json.Marshal(m)
	v := url.Values{}
	v.Set("payload", string(b))
	_, err := http.PostForm("https://hooks.slack.com/services/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", v)
	log.Error("SlackNotification > err: ", err)
}
