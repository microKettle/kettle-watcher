package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strconv"

	log "github.com/Sirupsen/logrus"
)

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

func GetEventEnrollmentEligibility(u *User, event int) (*FrontDeskEnrollments, error) {
	eventID := strconv.Itoa(event)
	client := &http.Client{}
	url := KettleFrontDesk + "/users/" + strconv.Itoa(u.UserID) + "/events/" + eventID + "/availibity"
	log.Debugf("Polling > url: %s", url)
	req, err := http.NewRequest("GET", url, nil)
	log.Debug("getEnrollmentEligibility > request: ", req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New("Unexpected http status code: " + strconv.Itoa(resp.StatusCode))
	}
	f := &FrontDeskEnrollments{}
	err = json.Unmarshal(body, f)
	if err != nil {
		return nil, err
	}
	return f, nil
}
