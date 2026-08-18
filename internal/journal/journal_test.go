package journal_test

import (
	"testing"
	"time"

	"github.com/lacsar712/spillway/internal/journal"
)

func TestGetPreservesBodyForReplay(t *testing.T) {
	log := journal.New(50)
	body := []byte(`{"type":"gate.raise","payload":{"password":"hunter2"}}`)
	log.Append(journal.Entry{
		At:         time.Unix(1, 0),
		CommandID:  "evt1",
		DeliveryID: "dlv1",
		PLCID:      "dst1",
		Attempt:    1,
		Kind:       "terminal",
		Type:       "gate.raise",
		Body:       body,
	})
	got, ok := log.Get("dlv1")
	if !ok {
		t.Fatal("missing entry")
	}
	if string(got.Body) != string(body) {
		t.Fatalf("Get stripped body: %q", got.Body)
	}
	listed := log.List("", 10)
	if len(listed) != 1 {
		t.Fatalf("list n=%d", len(listed))
	}
	if listed[0].Body != nil {
		t.Fatalf("List must hide raw body, got %q", listed[0].Body)
	}
}
