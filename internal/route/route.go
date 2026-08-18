package route

import (
	"time"

	"github.com/lacsar712/spillway/internal/idgen"
	"github.com/lacsar712/spillway/internal/plant"
)

type PlanItem struct {
	PLCID      string
	DeliveryID string
	URL        string
	Ordered    bool
}

type Plan struct {
	CommandID  string
	Type       string
	Items      []PlanItem
	DroppedOff int
}

func Fanout(eventID, eventType string, body []byte, dests []plant.PLC, now time.Time) Plan {
	_ = body
	p := Plan{CommandID: eventID, Type: eventType, Items: make([]PlanItem, 0, len(dests))}
	for _, d := range dests {
		if !d.Matches(eventType) {
			p.DroppedOff++
			continue
		}
		p.Items = append(p.Items, PlanItem{
			PLCID:      d.ID,
			DeliveryID: idgen.New("dlv", now),
			URL:        d.URL,
			Ordered:    d.Ordered,
		})
	}
	return p
}
