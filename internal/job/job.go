package job

import "time"

type Job struct {
	CommandID  string    `json:"command_id"`
	DeliveryID string    `json:"delivery_id"`
	PLCID      string    `json:"plc_id"`
	Type       string    `json:"type"`
	Body       []byte    `json:"body"`
	Attempt    int       `json:"attempt"`
	NotBefore  time.Time `json:"not_before"`
	CreatedAt  time.Time `json:"created_at"`
	ReplayOf   string    `json:"replay_of,omitempty"`
}

func (j Job) Clone() Job {
	c := j
	if j.Body != nil {
		c.Body = append([]byte(nil), j.Body...)
	}
	return c
}
