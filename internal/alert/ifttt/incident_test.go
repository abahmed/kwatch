package ifttt

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSendIncidentFillsLeadNoteAndStatus(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewIfttt(map[string]interface{}{"key": "abc123"}, "dev",
		rec.Dependencies())
	c.url = rec.URL()

	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := c.SendIncident(
				context.Background(), tc.Message,
			); err != nil {
				t.Fatalf("SendIncident() error = %v", err)
			}
			body := rec.Last(t).JSON(t)
			short, _ := body["value1"].(string)
			note, _ := body["value2"].(string)
			if short != tc.Message.Short || note != tc.Message.Note ||
				body["value3"] != tc.Message.Status.String() {
				t.Fatalf("payload = %v", body)
			}
			providertest.AssertOneLeadingEmoji(t, short)
			providertest.AssertOneLeadingEmoji(t, note)
		})
	}
	if last := rec.Last(t).JSON(t); last["value3"] != "resolved" {
		t.Fatalf("resolve status = %v", last["value3"])
	}
}
