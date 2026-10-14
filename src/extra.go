package main

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/pando85/GoHeishaMon/src/logger"
	"github.com/pando85/GoHeishaMon/src/serial"
)

// panasonicExtraQuery asks K/L series and newer heat pumps for the extra data block
// (more precise power values). It is the main query with byte 3 set to 0x21.
var panasonicExtraQuery = func() []byte {
	q := append([]byte(nil), panasonicQuery...)
	q[3] = 0x21
	return q
}()

// extraBlockAvailable is set, as in HeishaMon, once a main packet has byte 0xC7 >= 3 or an
// extra block reply arrives. Never send the extra query otherwise: an older heat pump
// stopped answering all queries after receiving it.
var extraBlockAvailable atomic.Bool

// extraTopics are HeishaMon's XTOP0-XTOP5: uint16 little endian at Byte, minus 1.
var extraTopics = []struct {
	Name string
	Byte int
}{
	{"Heat_Power_Consumption_Extra", 14},
	{"Cool_Power_Consumption_Extra", 16},
	{"DHW_Power_Consumption_Extra", 18},
	{"Heat_Power_Production_Extra", 20},
	{"Cool_Power_Production_Extra", 22},
	{"DHW_Power_Production_Extra", 24},
}

var actDataExtra = make([]string, len(extraTopics))
var actDataExtraMu sync.Mutex

// extraTopicBase is Mqtt_extra_base, or by default the sibling "extra" of a topic base
// ending in "/main" (HeishaMon's layout), otherwise "<topic base>/extra".
func extraTopicBase() string {
	if config.Mqtt_extra_base != "" {
		return config.Mqtt_extra_base
	}
	if base := strings.TrimSuffix(config.Mqtt_topic_base, "/main"); base != config.Mqtt_topic_base {
		return base + "/extra"
	}
	return config.Mqtt_topic_base + "/extra"
}

// handlePacket decodes a 203-byte datagram from the heat pump and publishes its topics.
func handlePacket(packet []byte, mclient mqtt.Client) {
	if len(packet) != serial.DataMessageLength {
		return
	}
	if packet[3] == 0x21 {
		extraBlockAvailable.Store(true)
		decode_heatpump_data_extra(packet, mclient)
		return
	}
	if packet[0xc7] >= 3 && !extraBlockAvailable.Load() {
		logger.Info("Extra data block available on this heat pump")
		extraBlockAvailable.Store(true)
	}
	decode_heatpump_data(packet, mclient)
}

func decode_heatpump_data_extra(data []byte, mclient mqtt.Client) {
	base := extraTopicBase()
	for k, v := range extraTopics {
		value := fmt.Sprintf("%d", (int(data[v.Byte+1])<<8|int(data[v.Byte]))-1)

		actDataExtraMu.Lock()
		changed := actDataExtra[k] != value
		if changed {
			actDataExtra[k] = value
		}
		actDataExtraMu.Unlock()

		if changed {
			logger.Info("received XTOP%d %s: %s", k, v.Name, value)
			topic := fmt.Sprintf("%s/%s", base, v.Name)
			token := mclient.Publish(topic, byte(0), false, value)
			if token.Wait() && token.Error() != nil {
				logger.Error("Fail to publish, %v", token.Error())
			}
		}
	}
}
