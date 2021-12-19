package util

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/provider"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GetPodEventsStr returns formatted events as a string for specified pod
func GetPodEventsStr(c kubernetes.Interface, name, namespace string) string {
	evnts, err := getPodEvents(c, name, namespace)

	eventsString := ""
	if err != nil {
		logrus.Warnf("failed to get events for %s@%s: %s", name, namespace, err.Error())
		return eventsString
	}

	for _, ev := range evnts.Items {
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
	var providers []provider.Provider

	for key, value := range viper.Get("providers").(map[string]interface{}) {
		for c, v := range value.(map[string]interface{}) {
			if key == "slack" && c == "webhook" && len(strings.TrimSpace(v.(string))) > 0 {
				providers = append(providers, provider.NewSlack(viper.GetString("providers.slack.webhook")))
			}
			if key == "pagerduty" && c == "integrationkey" && len(strings.TrimSpace(v.(string))) > 0 {
				providers = append(providers, provider.NewPagerDuty(viper.GetString("providers.pagerduty.integrationKey")))
			}
			if key == "discord" && c == "webhook" && len(strings.TrimSpace(v.(string))) > 0 {
				providers = append(providers, provider.NewDiscord(viper.GetString("providers.discord.webhook")))
			}
		}
	}

	return providers
}
