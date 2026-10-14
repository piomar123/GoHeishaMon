package main

import (
	"bytes"
	"fmt"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type valueRecorder struct {
	mqtt.Client
	values map[string]string
}

func (r *valueRecorder) Publish(topic string, _ byte, _ bool, payload interface{}) mqtt.Token {
	r.values[topic] = fmt.Sprint(payload)
	return &mqtt.DummyToken{}
}

func newValueRecorder() *valueRecorder { return &valueRecorder{values: map[string]string{}} }

func useTopicBases(t *testing.T, main, extra string) {
	saved := config
	t.Cleanup(func() { config = saved })
	config.Mqtt_topic_base = main
	config.Mqtt_extra_base = extra
}

// HeishaMon asks for the extra block only when byte 0xC7 of the main packet is >= 3
// (K/L series and newer). Older units stop answering after an unexpected 0x21 query.
func TestExtraBlockDetectedFromByteC7(t *testing.T) {
	useTopicBases(t, "hp/main", "")
	extraBlockAvailable.Store(false)
	t.Cleanup(func() { extraBlockAvailable.Store(false) })
	packet := mustDecodeHex(t, capturedPacket)

	handlePacket(packet, newValueRecorder())
	if extraBlockAvailable.Load() {
		t.Fatal("extra block marked available for byte 0xC7 = 2")
	}

	packet[0xc7] = 3
	handlePacket(packet, newValueRecorder())
	if !extraBlockAvailable.Load() {
		t.Fatal("extra block not marked available for byte 0xC7 = 3")
	}
}

func TestExtraTopicsDecodedFromExtraPacket(t *testing.T) {
	useTopicBases(t, "hp/main", "")
	t.Cleanup(func() { extraBlockAvailable.Store(false) })
	packet := make([]byte, 203)
	copy(packet, []byte{0x71, 0xc8, 0x01, 0x21})
	for i, v := range []uint16{1001, 0, 1, 2501, 301, 65535} { // uint16 little endian, value + 1
		packet[14+2*i] = byte(v)
		packet[15+2*i] = byte(v >> 8)
	}
	clearActData()
	r := newValueRecorder()

	handlePacket(packet, r)

	want := map[string]string{
		"hp/extra/Heat_Power_Consumption_Extra": "1000",
		"hp/extra/Cool_Power_Consumption_Extra": "-1",
		"hp/extra/DHW_Power_Consumption_Extra":  "0",
		"hp/extra/Heat_Power_Production_Extra":  "2500",
		"hp/extra/Cool_Power_Production_Extra":  "300",
		"hp/extra/DHW_Power_Production_Extra":   "65534",
	}
	for topic, w := range want {
		if got, ok := r.values[topic]; !ok || got != w {
			t.Errorf("%s = %q (published %v), want %q", topic, got, ok, w)
		}
	}
	if len(r.values) != len(want) {
		t.Errorf("published %d topics, want %d: %v", len(r.values), len(want), r.values)
	}
	if !extraBlockAvailable.Load() {
		t.Error("an extra block reply should mark the extra block available")
	}
}

func TestExtraTopicBase(t *testing.T) {
	for _, tc := range []struct{ main, extra, want string }{
		{"panasonic_heat_pump/main", "", "panasonic_heat_pump/extra"},
		{"heatpump", "", "heatpump/extra"},
		{"panasonic_heat_pump/main", "custom/x", "custom/x"},
	} {
		useTopicBases(t, tc.main, tc.extra)
		if got := extraTopicBase(); got != tc.want {
			t.Errorf("main %q extra %q: got %q, want %q", tc.main, tc.extra, got, tc.want)
		}
	}
}

func TestExtraQueryDiffersFromMainQueryOnlyInByte3(t *testing.T) {
	if len(panasonicExtraQuery) != len(panasonicQuery) || panasonicExtraQuery[3] != 0x21 {
		t.Fatalf("extra query = % x", panasonicExtraQuery[:4])
	}
	if !bytes.Equal(panasonicExtraQuery[:3], panasonicQuery[:3]) || !bytes.Equal(panasonicExtraQuery[4:], panasonicQuery[4:]) {
		t.Error("extra query differs from the main query outside byte 3")
	}
}
