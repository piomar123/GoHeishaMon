package main

import (
	"testing"
)

func TestBuildSetCurvesCommandFull(t *testing.T) {
	payload := `{
		"zone1": {"heat": {"target": {"high": 35, "low": 25}, "outside": {"high": 15, "low": -15}},
		          "cool": {"target": {"high": 20, "low": 10}, "outside": {"high": 35, "low": 20}}},
		"zone2": {"heat": {"target": {"high": 40, "low": 30}, "outside": {"high": 12, "low": -10}},
		          "cool": {"target": {"high": 18, "low": 8}, "outside": {"high": 30, "low": 18}}}
	}`
	command, err := buildSetCurvesCommand([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	if len(command) != PANASONICQUERYSIZE {
		t.Fatalf("length %d, want %d", len(command), PANASONICQUERYSIZE)
	}
	if command[0] != 0xf1 || command[1] != 0x6c || command[2] != 0x01 || command[3] != 0x10 {
		t.Errorf("header % x, want f1 6c 01 10", command[:4])
	}

	want := map[int]int{
		75: 35, 76: 25, 77: -15, 78: 15, // zone1 heat
		79: 40, 80: 30, 81: -10, 82: 12, // zone2 heat
		86: 20, 87: 10, 88: 20, 89: 35, // zone1 cool
		90: 18, 91: 8, 92: 18, 93: 30, // zone2 cool
	}
	for i := 4; i < len(command); i++ {
		v, ok := want[i]
		if !ok {
			if command[i] != 0 {
				t.Errorf("byte %d = %d, want 0 (keep)", i, command[i])
			}
			continue
		}
		if got := int(command[i]) - 128; got != v {
			t.Errorf("byte %d = %d, want %d", i, got, v)
		}
	}
}

func TestBuildSetCurvesCommandPartialKeepsOthers(t *testing.T) {
	command, err := buildSetCurvesCommand([]byte(`{"zone1":{"heat":{"target":{"high":33}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	for i := 4; i < len(command); i++ {
		if i == 75 {
			if command[i] != 33+128 {
				t.Errorf("byte 75 = %d, want %d", command[i], 33+128)
			}
		} else if command[i] != 0 {
			t.Errorf("byte %d = %d, want 0 (keep)", i, command[i])
		}
	}
}

func TestBuildSetCurvesCommandRejects(t *testing.T) {
	for name, payload := range map[string]string{
		"invalid json":  `{"zone1":`,
		"empty object":  `{}`,
		"no values":     `{"zone1":{"heat":{"target":{}}}}`,
		"unknown key":   `{"zone 1":{"heat":{"target":{"high":35}}}}`,
		"too low":       `{"zone1":{"heat":{"outside":{"low":-128}}}}`,
		"too high":      `{"zone1":{"heat":{"target":{"high":128}}}}`,
		"not a number":  `{"zone1":{"heat":{"target":{"high":"35"}}}}`,
		"not integer":   `{"zone1":{"heat":{"target":{"high":35.5}}}}`,
		"trailing data": `{"zone1":{"heat":{"target":{"high":35}}}} x`,
	} {
		if _, err := buildSetCurvesCommand([]byte(payload)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// The heat pump reports the curves at the same bytes SetCurves writes, so the
// decode table must use the same map.
func TestCurveTopicsMatchSetCurvesBytes(t *testing.T) {
	ParseTopicList3()
	want := map[string]int{}
	for _, c := range curveBytes {
		want[c.topic] = c.byte
	}
	found := 0
	for _, topic := range AllTopics {
		b, ok := want[topic.TopicName]
		if !ok {
			continue
		}
		found++
		if topic.TopicBit != b {
			t.Errorf("%s decodes byte %d, SetCurves writes byte %d", topic.TopicName, topic.TopicBit, b)
		}
	}
	if found != len(curveBytes) {
		t.Errorf("found %d curve topics, want %d", found, len(curveBytes))
	}
}
