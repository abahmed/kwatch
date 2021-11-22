package util

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetPodFailedEvents(c kubernetes.Interface, name, namespace string) string {
	evnts, err := getPodEvents(c, name, namespace)

	// get only failed events
	eventsString := ""

	if err != nil {
		logrus.Warnf("failed to get events for %s: %s", name, err.Error())
		return eventsString
	}

	for _, ev := range evnts.Items {
		if ev.Reason == "Failed" {
			eventsString += fmt.Sprintf("[%s] %s\n\n", ev.LastTimestamp.String(), ev.Message)
		}
	}

	return strings.TrimSpace(eventsString)
}

func GetPodContainerLogs(c kubernetes.Interface, name, container, namespace string) string {
	log, err := c.CoreV1().
		Pods(namespace).
		GetLogs(name, &v1.PodLogOptions{Container: container}).
		Do(context.TODO()).Get()
	if err != nil {
		return err.Error()
	}

	logObj, ok := log.(*metav1.Status)
	if !ok {
		logrus.Warnf("failed to cast log for %s: %v", container, logObj)
		return ""
	}

	return logObj.Message
}

func getPodEvents(c kubernetes.Interface, name, namespace string) (*v1.EventList, error) {
	return c.CoreV1().
		Events(namespace).
		List(context.TODO(), metav1.ListOptions{
			FieldSelector: "involvedObject.name=" + name,
		})
}
