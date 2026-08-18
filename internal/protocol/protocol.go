package protocol

import (
	"encoding/json"
	"fmt"
	"github.com/lacsar712/spillway/internal/analog"
	"strconv"
	"strings"
	"time"
)

// Frame is the JSON body written to a downstream PLC HTTP endpoint.
// The relay signs the raw bytes; this helper only builds them.
type Frame struct {
	Schema    string         `json:"schema"`
	IssuedAt  int64          `json:"issued_at"`
	CommandID string         `json:"command_id"`
	Bay       string         `json:"bay"`
	Action    string         `json:"action"`
	OpeningM  float64        `json:"opening_m"`
	Registers map[string]int `json:"registers"`
}

func ActionFromType(t string) string {
	t = strings.TrimSpace(t)
	switch t {
	case "gate.raise":
		return "RAISE"
	case "gate.lower":
		return "LOWER"
	case "gate.hold":
		return "HOLD"
	case "gate.stop":
		return "ESTOP"
	default:
		return strings.ToUpper(strings.ReplaceAll(t, ".", "_"))
	}
}

func OpeningToCounts(openingM float64, maxM float64) int {
	if maxM <= 0 {
		maxM = 8
	}
	if openingM < 0 {
		openingM = 0
	}
	if openingM > maxM {
		openingM = maxM
	}
	return int((openingM / maxM) * 10000)
}

func Build(commandID, bay, action string, openingM float64, now time.Time, bayReg int) ([]byte, error) {
	if commandID == "" {
		return nil, fmt.Errorf("command id required")
	}
	if bay == "" {
		bay = "S1"
	}
	if action == "" {
		action = "HOLD"
	}
	if bayReg < 1 {
		bayReg = 40001
	}
	counts, err := analog.Scale(openingM, 0, 8, 0, 10000)
	if err != nil {
		return nil, err
	}
	fr := Frame{
		Schema:    "spillway.plc.v1",
		IssuedAt:  now.Unix(),
		CommandID: commandID,
		Bay:       bay,
		Action:    action,
		OpeningM:  openingM,
		Registers: map[string]int{
			strconv.Itoa(bayReg): counts,
			"40010":              actionCode(action),
		},
	}
	return json.Marshal(fr)
}

func actionCode(action string) int {
	switch action {
	case "RAISE":
		return 1
	case "LOWER":
		return 2
	case "HOLD":
		return 3
	case "ESTOP":
		return 9
	default:
		return 0
	}
}

func ParseAction(raw []byte) (string, error) {
	var fr Frame
	if err := json.Unmarshal(raw, &fr); err != nil {
		return "", err
	}
	if fr.Action == "" {
		return "", fmt.Errorf("missing action")
	}
	return fr.Action, nil
}
