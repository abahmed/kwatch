package util

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

// JsonEscape escapes the json special characters in a string
func JsonEscape(i string) string {
	jm, _ := json.Marshal(i)

	s := string(jm)
	return s[1 : len(s)-1]
}
