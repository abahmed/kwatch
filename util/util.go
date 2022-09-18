package util

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/event"

	"github.com/abahmed/kwatch/provider"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GetPodEventsStr returns formatted events as a string for specified pod
func GetPodEventsStr(c kubernetes.Interface, name, namespace string) string {
	events, err := getPodEvents(c, name, namespace)

	eventsString := ""
	if err != nil {
		logrus.Warnf(
			"failed to get events for %s@%s: %s",
			name,
			namespace,
			err.Error())
		return eventsString
	}

	for _, ev := range events.Items {
		eventsString +=
			fmt.Sprintf(
				"[%s] %s %s\n",
				ev.LastTimestamp.String(),
				ev.Reason,
				ev.Message)
	}

	return strings.TrimSpace(eventsString)
}

// ContainsKillingStoppingContainerEvents checks if the events contain an event
// with "Killing Stopping container" which indicates that a container could not
// be gracefully shutdown
func ContainsKillingStoppingContainerEvents(
	c kubernetes.Interface,
	name,
	namespace string) bool {
	events, err := getPodEvents(c, name, namespace)
	if err != nil {
		return false
	}

	for _, ev := range events.Items {
		if strings.ToLower(ev.Reason) == "killing" &&
			strings.Contains(
				strings.ToLower(ev.Message),
				"stopping container") {
			return true
		}
	}

	return false
}

// GetPodContainerLogs returns logs for specified container in pod
func GetPodContainerLogs(
	c kubernetes.Interface, name, container, namespace string,
	previous bool) string {
	options := v1.PodLogOptions{
		Container: container,
		Previous:  previous,
	}

	// get max recent log lines
	var maxLogs int64 = viper.GetInt64("maxRecentLogLines")
	if maxLogs != 0 {
		options.TailLines = &maxLogs
	}

	// get logs
	logs, err := c.CoreV1().
		Pods(namespace).
		GetLogs(name, &options).
		DoRaw(context.TODO())

	if err != nil {
		logrus.Warnf(
			"failed to get logs for container %s in pod %s@%s: %s",
			name,
			container,
			namespace,
			err.Error())

		// try to decode response
		var status metav1.Status
		parseErr := json.Unmarshal(logs, &status)
		if parseErr == nil {
			return status.Message
		}

		logrus.Warnf(
			"failed to parse logs for container %s in pod %s@%s: %s",
			name,
			container,
			namespace,
			parseErr.Error())
	}

	return string(logs)
}

func getPodEvents(
	c kubernetes.Interface,
	name,
	namespace string) (*v1.EventList, error) {
	return c.CoreV1().
		Events(namespace).
		List(context.TODO(), metav1.ListOptions{
			FieldSelector: "involvedObject.name=" + name,
		})
}

// GetProviders returns slice of provider objects after parsing config
func GetProviders() []provider.Provider {
	var providers []provider.Provider
	const isPresent = false
	telegram := []bool{isPresent, isPresent}
	email := []bool{isPresent, isPresent, isPresent, isPresent, isPresent}

	for key, value := range viper.Get("alert").(map[string]interface{}) {
		for c, v := range value.(map[string]interface{}) {
			if key == "slack" &&
				c == "webhook" &&
				len(strings.TrimSpace(v.(string))) > 0 {
				providers = append(
					providers,
					provider.NewSlack(viper.GetString("alert.slack.webhook")))
			}
			if key == "pagerduty" &&
				c == "integrationkey" &&
				len(strings.TrimSpace(v.(string))) > 0 {
				providers = append(
					providers,
					provider.NewPagerDuty(
						viper.GetString("alert.pagerduty.integrationKey")))
			}
			if key == "discord" &&
				c == "webhook" &&
				len(strings.TrimSpace(v.(string))) > 0 {
				providers = append(
					providers,
					provider.NewDiscord(
						viper.GetString("alert.discord.webhook")))
			}
			if key == "telegram" &&
				c == "token" &&
				len(strings.TrimSpace(v.(string))) > 0 {
				telegram[0] = true
			}
			if key == "telegram" &&
				c == "chatid" &&
				len(strings.TrimSpace(v.(string))) > 0 {
				telegram[1] = true
			}
			if key == "teams" &&
				c == "webhook" &&
				len(strings.TrimSpace(v.(string))) > 0 {
				providers = append(
					providers,
					provider.NewTeams(viper.GetString("alert.teams.webhook")))
			}
			if key == "email" &&
				c == "from" &&
				len(strings.TrimSpace(v.(string))) > 0 {
				email[0] = true
			}
			if key == "email" &&
				c == "to" &&
				len(strings.TrimSpace(v.(string))) > 0 {
				email[1] = true
			}
			if key == "email" &&
				c == "port" &&
				len(strings.TrimSpace(v.(string))) > 0 {
				email[2] = true
			}
			if key == "email" &&
				c == "host" &&
				len(strings.TrimSpace(v.(string))) > 0 {
				email[3] = true
			}
			if key == "email" &&
				c == "password" &&
				len(strings.TrimSpace(v.(string))) > 0 {
				email[4] = true
				if key == "rocketchat" &&
					c == "webhook" &&
					len(strings.TrimSpace(v.(string))) > 0 {
					providers = append(
						providers,
						provider.NewRocketChat(
							viper.GetString("alert.rocketchat.webhook")))
				}
				if key == "mattermost" &&
					c == "webhook" &&
					len(strings.TrimSpace(v.(string))) > 0 {
					providers = append(
						providers,
						provider.NewMattermost(
							viper.GetString("alert.mattermost.webhook")))
				}
				if key == "opsgenie" &&
					c == "apikey" &&
					len(strings.TrimSpace(v.(string))) > 0 {
					providers = append(
						providers,
						provider.NewOpsgenie(
							viper.GetString("alert.opsgenie.apikey")))
				}
			}
			if key == "telegram" &&
				IsListAllBool(true, telegram) {
				providers = append(
					providers,
					provider.NewTelegram(
						viper.GetString("alert.telegram.token"),
						viper.GetString("alert.telegram.chatId")))
			}
			if key == "email" &&
				IsListAllBool(true, email) {
				providers = append(
					providers,
					provider.NewEmail(viper.GetString("alert.email.from"),
						viper.GetString("alert.email.password"),
						viper.GetString("alert.email.host"),
						viper.GetInt("alert.email.port"),
						viper.GetString("alert.email.to")))
			}
		}
	}

	return providers
}

// SendProvidersMsg sends string msg to all providers
func SendProvidersMsg(p []provider.Provider, msg string) {
	logrus.Infof("sending message: %s", msg)
	for _, prv := range p {
		err :=
			prv.SendMessage(msg)
		if err != nil {
			logrus.Errorf(
				"failed to send msg with %s: %s",
				prv.Name(),
				err.Error())
		}
	}
}

// SendProvidersEvent sends event to all providers
func SendProvidersEvent(p []provider.Provider, event event.Event) {
	logrus.Infof("sending event: %+v", event)
	for _, prv := range p {
		if err := prv.SendEvent(&event); err != nil {
			logrus.Errorf(
				"failed to send event with %s: %s",
				prv.Name(),
				err.Error(),
			)
		}
	}
}

// IsStrInSlice checks if string is existing in a slice of string
func IsStrInSlice(str string, strList []string) bool {
	if len(strList) == 0 {
		return false
	}

	for _, s := range strList {
		if s == str {
			return true
		}
	}

	return false
}

// IsListAllBool checks if all elements in a boolean list have the same value
func IsListAllBool(v bool, l []bool) bool {
	for _, x := range l {
		if x != v {
			return false
		}
	}
	return true
}
