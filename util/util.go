package util

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
		logrus.Warnf("failed to get events for %s@%s: %s", name, namespace, err.Error())
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

func getPodEvents(c kubernetes.Interface, name, namespace string) (*v1.EventList, error) {
	return c.CoreV1().
		Events(namespace).
		List(context.TODO(), metav1.ListOptions{
			FieldSelector: "involvedObject.name=" + name,
		})
}

// GetProviders returns slice of provider objects after parsing config
func GetProviders() []provider.Provider {
	pAttributes := populateProvidersAttributes()

	var providers []provider.Provider
	for key, value := range viper.Get("alert").(map[string]interface{}) {
		// checks if the provider name exists
		att, found := pAttributes[key]
		if !found {
			continue
		}

		// checks if the corresponding attributes exists
		for c, v := range value.(map[string]interface{}) {
			if !IsStrInSlice(c, att) || len(strings.TrimSpace(v.(string))) <= 0 {
				found = false
			}
		}

		// add a new object from corresponding provider
		if found {
			providers = append(providers, provider.New(key))
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

// populateProvidersAttributes populates and maps providers names and required attributes from json file
func populateProvidersAttributes() map[string][]string {
	js, err := ioutil.ReadFile("provider/providers.json")
	if err != nil {
		logrus.Error(err)
	}
	activeProviders := make(map[string]interface{})
	parseErr := json.Unmarshal(js, &activeProviders)
	if parseErr != nil {
		logrus.Error(parseErr)
	}
	pAttributes := make(map[string][]string)
	for key, value := range activeProviders {
		var attList []string
		for att := range value.(map[string]interface{}) {
			attList = append(attList, att)
		}
		pAttributes[key] = attList
	}
	return pAttributes
}
