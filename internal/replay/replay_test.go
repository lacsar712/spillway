package replay_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/lacsar712/spillway/internal/journal"
	"github.com/lacsar712/spillway/internal/redact"
	"github.com/lacsar712/spillway/internal/replay"
)

func TestFromJournalUsesOriginalBody(t *testing.T) {
	body := []byte(`{"type":"gate.raise","payload":{"password":"hunter2","order_id":"A"}}`)
	e := journal.Entry{
		CommandID:    "evt1",
		DeliveryID:   "dlv1",
		PLCID:        "dst1",
		Type:         "gate.raise",
		Body:         body,
		BodyRedacted: redact.JSON(body),
	}
	j, err := replay.FromJournal(e, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(j.Body, body) {
		t.Fatalf("replay body %s want original, not redacted %s", j.Body, e.BodyRedacted)
	}
	if bytes.Contains(j.Body, []byte("***")) {
		t.Fatal("replay used masked payload")
	}
}

func TestFromJournalRejectsEmptyBody(t *testing.T) {
	_, err := replay.FromJournal(journal.Entry{
		CommandID:  "evt1",
		DeliveryID: "dlv-empty",
		PLCID:      "dst1",
		Type:       "gate.raise",
	}, time.Unix(1, 0))
	if err == nil {
		t.Fatal("empty journal body must not become a replay job")
	}
}
