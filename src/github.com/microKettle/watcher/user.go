package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	log "github.com/Sirupsen/logrus"
)

type User struct {
	UserID int
}

type SlackMessage struct {
	Channel    string `json:"channel"`
	Text       string `json:"text"`
	Username   string `json:"username"`
	Icon_emoji string `json:"icon_emoji"`
}

func (u *User) slackNotification(eventID int) {
	// TODO: Create microservice for notification
	m := &SlackMessage{Channel: "#general", Text: strconv.Itoa(eventID) + " open!", Username: "Kettle", Icon_emoji: ":computer:"}
	b, _ := json.Marshal(m)
	v := url.Values{}
	v.Set("payload", string(b))
	_, err := http.PostForm("https://hooks.slack.com/services/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", v)
	log.Error("SlackNotification > err: ", err)
}
