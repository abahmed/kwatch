package util

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
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
	previous bool,
	maxRecentLogLines int64) string {
	options := v1.PodLogOptions{
		Container: container,
		Previous:  previous,
	}

	// get max recent log lines
	if maxRecentLogLines != 0 {
		options.TailLines = &maxRecentLogLines
	}

	// get logs
	logs, err := getContainerLogs(c, name, namespace, &options)
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

func getContainerLogs(
	c kubernetes.Interface,
	name string,
	namespace string,
	options *v1.PodLogOptions) ([]byte, error) {
	return c.CoreV1().
		Pods(namespace).
		GetLogs(name, options).
		DoRaw(context.TODO())
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

// RandomString generates random string with provided n size
func RandomString(n int) string {
	const availableCharacterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLM" +
		"NOPQRSTUVWXYZ0123456789"

	b := make([]byte, n)
	rand.Seed(time.Now().UnixNano())
	for i := range b {
		b[i] = availableCharacterBytes[rand.Intn(len(availableCharacterBytes))]
	}

	return string(b)
}
