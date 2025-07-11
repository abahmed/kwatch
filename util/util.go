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
func GetPodEventsStr(events *[]v1.Event) string {
	if events == nil {
		return ""
	}

	eventsString := ""

	for _, ev := range *events {
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
	events, err := GetPodEvents(c, name, namespace)
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

// GetPodEvents retrieves the events for a specific pod
func GetPodEvents(
	c kubernetes.Interface,
	name,
	namespace string) (*v1.EventList, error) {
	return c.CoreV1().
		Events(namespace).
		List(context.TODO(), metav1.ListOptions{
			FieldSelector: "involvedObject.name=" + name,
		})
}

// GetNodes gets a list of nodes
func GetNodes(c kubernetes.Interface) (*v1.NodeList, error) {
	return c.CoreV1().
		Nodes().
		List(context.TODO(), metav1.ListOptions{})
}

// GetNodeSummary gets a list of nodes
func GetNodeSummary(c kubernetes.Interface, name string) ([]byte, error) {
	return c.CoreV1().
		RESTClient().
		Get().
		Resource("nodes").
		Name(name).
		SubResource("proxy").
		Suffix("stats/summary").
		DoRaw(context.TODO())
}

// GetPVNameFromPVC returns the name of persistent volume given a namespace and
// persistent volume claim name
func GetPVNameFromPVC(
	c kubernetes.Interface,
	namespace, pvcName string) (string, error) {
	pvc, err :=
		c.CoreV1().
			PersistentVolumeClaims(namespace).
			Get(context.TODO(), pvcName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	return pvc.Spec.VolumeName, nil
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
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := range b {
		b[i] = availableCharacterBytes[r.Intn(len(availableCharacterBytes))]
	}

	return string(b)
}
