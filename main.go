package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/PagerDuty/go-pagerduty"
	"github.com/slack-go/slack"
)

var (
	pagerdutyToken  = os.Getenv("PAGERDUTY_AUTH_TOKEN")
	slackWebhookURL = os.Getenv("SLACK_WEBHOOK_URL")
	scheduleID      = os.Getenv("PAGERDUTY_SCHEDULE_ID")
)

func ParseStartTime(timezone string) (start_time string, end_time string, err error) {
	now := time.Now()
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return "", "", fmt.Errorf("unable to load specified timezone : %s", timezone)
	}
	startTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, loc)
	endTime := startTime.Add(24 * time.Hour)

	return startTime.Format(time.RFC3339), endTime.Format(time.RFC3339), nil
}

func formatTimestamp(timestamp string) (formattedTime string) {
	parsedTime, _ := time.Parse(time.RFC3339, timestamp)
	formattedTime = parsedTime.Format("Jan 2, 3 PM 2006")
	return formattedTime
}

func GetOncallUserDetails(schedule_id string, timezone string) (oncallUserData map[string]map[string]string, err error) {
	if pagerdutyToken == "" || scheduleID == "" {
		log.Fatalf("PAGERDUTY_AUTH_TOKEN or PAGERDUTY_SCHEDULE_ID is missing")
	}
	client := pagerduty.NewClient(pagerdutyToken)

	startTime, endTime, _ := ParseStartTime(timezone)
	oncall, err := client.ListOnCallsWithContext(context.Background(), pagerduty.ListOnCallOptions{ScheduleIDs: []string{schedule_id}, TimeZone: timezone, Since: startTime, Until: endTime})
	if err != nil {
		var aerr pagerduty.APIError
		if errors.As(err, &aerr) {
			return nil, fmt.Errorf("%s", aerr.Error())
		}
	}
	oncallUserData = make(map[string]map[string]string)
	for _, oncall := range oncall.OnCalls {
		oncallUserData[oncall.User.Summary] = make(map[string]string)
		oncallUserData[oncall.User.Summary]["schedule"] = oncall.Schedule.Summary
		oncallUserData[oncall.User.Summary]["start_time"] = formatTimestamp(oncall.Start)
		oncallUserData[oncall.User.Summary]["end_time"] = formatTimestamp(oncall.End)
		oncallUserData[oncall.User.Summary]["schedule_url"] = oncall.Schedule.HTMLURL
	}
	return oncallUserData, nil
}

func SendSlackMessage(channel, message, schedule_url string) error {
	if slackWebhookURL == "" {
		log.Fatalf("SLACK_WEBHOOK_URL is missing")
	}

	attachment := slack.Attachment{
		Color: "good",
		Text:  message,
		Actions: []slack.AttachmentAction{
			{
				Name: "manage_schedule",
				Text: ":pagerduty: Manage Schedule",
				Type: "button",
				URL:  schedule_url,
			},
		},
	}

	msg := &slack.WebhookMessage{
		Attachments: []slack.Attachment{attachment},
	}

	log.Printf("Sending message to Slack channel %s: %s", channel, message)
	err := slack.PostWebhook(slackWebhookURL, msg)
	if err != nil {
		return fmt.Errorf("failed to send message to Slack: %v", err)
	}
	log.Println("Message sent to Slack successfully")
	return nil
}

func main() {
	channel := "devops-alerts"
	timezone := "Asia/Jerusalem"
	oncallUserData, err := GetOncallUserDetails(scheduleID, timezone)
	if err != nil {
		log.Fatalf("Error getting on-call user details: %v", err)
	}
	for user, details := range oncallUserData {
		message := fmt.Sprintf("*%s* is on call from *%s* to *%s*", user, details["start_time"], details["end_time"])
		err = SendSlackMessage(channel, message, details["schedule_url"])
		if err != nil {
			log.Fatalf("Error sending Slack message: %v\n", err)
		}
	}
	log.Println("Slack message sent successfully")
}
