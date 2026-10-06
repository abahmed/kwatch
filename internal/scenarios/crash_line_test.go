package scenarios

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/replay"
)

const dotnetLine = "System.IO.FileNotFoundException: Could not load file " +
	"or assembly 'Trella.Models.Common, Version=3.1.120.0, " +
	"Culture=neutral, PublicKeyToken=null'. The system cannot find " +
	"the file specified."

// The first line of the crash log names the failure; a Deployment with
// one replica that aborts on start quotes it, as one with two does.
func TestSingleReplicaCrashQuotesTheFirstLogLine(t *testing.T) {
	c := newCluster(scenarioStart, "")
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("staging", "warehouse",
		"registry.example.com/warehouse:1.178", 1)
	c.list(w.objects())
	c.list(w.pod(0, "n1"))
	c.after(time.Minute)
	for restarts := int32(1); restarts <= 8; restarts++ {
		pod := w.pod(0, "n1", crashLoop(134, "Error", "", restarts))
		c.update(pod)
		id := kube.ContainerID(pod.Namespace, pod.Name, "app")
		c.emit(kube.CrashLogObservation(id, c.now,
			[]string{dotnetLine, "", "File name: 'Trella.Models.Common'"}))
		w.setReady(0)
		c.update(w.objects())
		c.after(2 * time.Minute)
	}
	result := replayLog(t, c.log(), replay.Options{Tail: 10 * time.Minute})

	var all []string
	for _, m := range result.Messages {
		all = append(all, m.Title+" "+strings.Join(m.Lines, " "))
	}
	text := strings.Join(all, "\n")
	// The message shortens a long line, with an ellipsis, after the part
	// that names the missing assembly.
	want := `It fails with "` + dotnetLine[:strings.Index(dotnetLine,
		"Culture")-2]
	if !strings.Contains(text, want) {
		t.Errorf("the crash line is not quoted:\n%s", text)
	}
}
