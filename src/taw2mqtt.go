package main

import (
	"crypto/md5"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/pando85/GoHeishaMon/src/logger"
	"github.com/pando85/GoHeishaMon/src/serial"
)

var panasonicQuery []byte = []byte{0x71, 0x6c, 0x01, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
var PANASONICQUERYSIZE int = 110

var MqttKeepalive time.Duration
// Commands are in arrival order, and are deliberately ephemeral: replaying a
// heating command hours after the requester sent it would be unsafe.
const (
	maxQueuedCommands = 256
	commandTTL = 2 * time.Minute
)

type queuedCommand struct {
	data []byte
	queuedAt time.Time
}

var commandQueue []queuedCommand
var commandsMu sync.Mutex

// actData is accessed by the serial reader and refresh goroutine.
var actData = make([]string, len(AllTopics))
var actDataMu sync.Mutex
var config Config
var serialComms *serial.Comms
var err error
var SwitchTopics map[string]AutoDiscoverStruct
var ClimateTopics map[string]AutoDiscoverStruct

type TopicData struct {
	TopicNumber        int
	TopicName          string
	TopicBit           int
	TopicFunction      string
	TopicUnit          string
	TopicA2M           string
	TopicType          string
	TopicDisplayUnit   string
	TopicValueTemplate string
}

type Config struct {
	Readonly               bool
	LogLevel               string
	Device                 string
	ReadInterval           int
	MqttServer             string
	MqttPort               string
	MqttLogin              string
	Aquarea2mqttCompatible bool
	Mqtt_topic_base        string
	Mqtt_set_base          string
	Aquarea2mqttPumpID     string
	MqttPass               string
	MqttClientID           string
	MqttKeepalive          int
	MqttUseTLS             bool
	MqttTLSInsecure        bool
	MqttCaCertFile         string
	MqttClientCertFile     string
	MqttClientKeyFile      string
	ForceRefreshTime       int
	EnableCommand          bool
	SleepAfterCommand      int
	HAAutoDiscover         bool
}

var configfile string

func ReadConfig() Config {

	_, err := os.Stat(configfile)
	if err != nil {
		logger.Error("Config file is missing: %s", configfile)
	}

	var config Config
	if _, err := toml.DecodeFile(configfile, &config); err != nil {
		logger.Error("%v", err)
	}
	return config
}

func UpdateConfig(configFilePath string) bool {
	logger.Info("update-config: try to update config file: %s", configFilePath)
	out, err := exec.Command("/usr/bin/usb_mount.sh").Output()
	if err != nil {
		logger.Error("update-config: /usr/bin/usb_mount.sh: failed: %s: %s", err.Error(), string(out))
	}
	_, err = os.Stat("/mnt/usb/GoHeishaMonConfig.new")
	if err != nil {
		_, _ = exec.Command("/usr/bin/usb_umount.sh").Output()
		logger.Error("update-config: no new configfile found: %s, unmounting USB", err.Error())
		return false
	}
	if GetFileChecksum(configFilePath) != GetFileChecksum("/mnt/usb/GoHeishaMonConfig.new") {
		logger.Info("update-config: checksum of configfile and new configfile different: %s", configFilePath)

		_, err = exec.Command("/bin/cp", "/mnt/usb/GoHeishaMonConfig.new", configFilePath).Output()
		if err != nil {
			logger.Error("update-config: can't update configfile %s", configFilePath)
			return false
		}
		_, _ = exec.Command("sync").Output()

		_, _ = exec.Command("/usr/bin/usb_umount.sh").Output()
		_, _ = exec.Command("reboot").Output()
		return true
	}
	_, _ = exec.Command("/usr/bin/usb_umount.sh").Output()

	return true
}

func UpdatePassword() bool {
	_, err = os.Stat("/mnt/usb/GoHeishaMonPassword.new")
	if err != nil {
		return true
	} else {
		_, _ = exec.Command("chmod", "+x", "/root/pass.sh").Output()
		dat, _ := os.ReadFile("/mnt/usb/GoHeishaMonPassword.new")
		logger.Info("updates the password to: %s", string(dat))
		o, err := exec.Command("/root/pass.sh", string(dat)).Output()
		if err != nil {
			logger.Error("%v", err)
			logger.Error("%s", string(o))

			return false
		}
		logger.Info("%s", string(o))

		_, _ = exec.Command("/bin/rm", "/mnt/usb/GoHeishaMonPassword.new").Output()
	}
	return true
}

func GetFileChecksum(f string) string {
	input := strings.NewReader(f)

	hash := md5.New()
	if _, err := io.Copy(hash, input); err != nil {
		logger.Error("failed to calculate checksum: %v", err)
	}
	sum := hash.Sum(nil)

	return fmt.Sprintf("%x\n", sum)

}

func UpdateConfigLoop(configfile string) {
	for {
		UpdateConfig(configfile)
		time.Sleep(time.Minute * 5)

	}
}

func PublishTopicsToAutoDiscover(mclient mqtt.Client) {
	var token mqtt.Token
	for k, v := range AllTopics {
		var m AutoDiscoverStruct
		m.UID = fmt.Sprintf("Aquarea-%s-%d", config.MqttLogin, k)
		if v.TopicType == "" {
			v.TopicType = "sensor"
		}
		m.ValueTemplate = v.TopicValueTemplate

		m.UnitOfM = v.TopicDisplayUnit

		if v.TopicType == "binary_sensor" {
			m.UnitOfM = ""
			m.PayloadOn = "1"
			m.PayloadOff = "0"
			m.ValueTemplate = `{{ value }}`
		}
		if v.TopicDisplayUnit == "°C" {
			m.DeviceClass = "temperature"
		}
		if v.TopicDisplayUnit == "W" {
			m.DeviceClass = "power"
		}
		m.StateTopic = fmt.Sprintf("%s/%s", config.Mqtt_topic_base, v.TopicName)
		m.Name = fmt.Sprintf("TEST-%s", v.TopicName)
		Topic_Value, err := json.Marshal(m)
		//Topic_Value = []byte("")
		//v.TopicType = "sensor"

		if err != nil {
			logger.Error("%v", err)
		}
		TOP := fmt.Sprintf("%s/%s/%s/config", config.Mqtt_topic_base, v.TopicType, strings.ReplaceAll(m.Name, " ", "_"))
		logger.Info("It publishes to %s warosc %s", TOP, string(Topic_Value))
		token = mclient.Publish(TOP, byte(0), false, Topic_Value)
		if token.Wait() && token.Error() != nil {
			logger.Error("Fail to publish: %v", token.Error())
		}

	}

	for _, vs := range SwitchTopics {
		if vs.ValueTemplate == "" {
			vs.PayloadOff = "0"
			vs.PayloadOn = "1"
		}
		vs.Optimistic = "true"
		Topic_Value, err := json.Marshal(vs)
		//Topic_Value = []byte("")

		if err != nil {
			logger.Error("%v", err)
		}
		TOP := fmt.Sprintf("%s/%s/%s/config", config.Mqtt_topic_base, "switch", strings.ReplaceAll(vs.Name, " ", "_"))
		logger.Info("It publishes to %s warosc %s", TOP, string(Topic_Value))
		token = mclient.Publish(TOP, byte(0), false, Topic_Value)
		if token.Wait() && token.Error() != nil {
			logger.Error("Fail to publish: %v", token.Error())
		}
	}

}

type AutoDiscoverStruct struct {
	DeviceClass   string `json:"device_class,omitempty"`
	Name          string `json:"name,omitempty"`
	StateTopic    string `json:"state_topic,omitempty"`
	UnitOfM       string `json:"unit_of_measurement,omitempty"`
	ValueTemplate string `json:"value_template,omitempty"`
	CommandTopic  string `json:"command_topic,omitempty"`
	UID           string `json:"unique_id,omitempty"`
	PayloadOn     string `json:"payload_on,omitempty"`
	PayloadOff    string `json:"payload_off,omitempty"`
	Optimistic    string `json:"optimistic,omitempty"`
	StateON       string `json:"state_on,omitempty"`
	StateOff      string `json:"state_off,omitempty"`
}

func main() {
	SwitchTopics = make(map[string]AutoDiscoverStruct)

	flag.Parse()
	if runtime.GOOS != "windows" {
		configfile = "/etc/config/goheishamon.toml"
	} else {
		configfile = "config"
	}

	_, err := os.Stat(configfile)
	if err != nil {
		logger.Error("Config file is missing: %s", configfile)
		UpdateConfig(configfile)
	}

	go UpdateConfigLoop(configfile)
	config = ReadConfig()
	go ClearActData()

	// Initialize logger with configured level
	logger.SetLevelString(config.LogLevel)

	// Initialize the heat pump serial communication
	serialComms = &serial.Comms{}
	err = serialComms.Open(config.Device, time.Millisecond*100)
	if err != nil {
		logger.Error("failed to open serial port: %v", err)
	}
	defer func() {
		if err := serialComms.Close(); err != nil {
			logger.Error("Failed to close serial port: %v", err)
		}
	}()

	PoolInterval := time.Second * time.Duration(config.ReadInterval)
	// the heat pump answers every query, so silence for several intervals
	// means the serial link is gone
	if stall := 5 * PoolInterval; stall > 30*time.Second {
		serialComms.StallTimeout = stall
	}
	MqttKeepalive = time.Second * time.Duration(config.MqttKeepalive)
	MC, MT := MakeMQTTConn()

	if config.HAAutoDiscover {
		PublishTopicsToAutoDiscover(MC)
	}

	go readSerial(MC, MT)

	for {
		attempted, sendErr := sendNextQueuedCommand(func(command []byte) error {
			return send_command(command, len(command))
		})
		if attempted {
			if !errors.Is(sendErr, serial.ErrNotOpen) {
				time.Sleep(time.Second * time.Duration(config.SleepAfterCommand))
			}
		} else {
			_ = send_command(panasonicQuery, PANASONICQUERYSIZE)
		}

		time.Sleep(PoolInterval)
	}
}

func ClearActData() {
	for {
		time.Sleep(time.Second * time.Duration(config.ForceRefreshTime))
		clearActData()
	}
}

func clearActData() {
	actDataMu.Lock()
	defer actDataMu.Unlock()
	for k := range actData {
		actData[k] = "nil" //funny i know ;)
	}
}

// expireQueuedCommandsLocked is called while commandsMu is held.
func expireQueuedCommandsLocked(now time.Time) {
	expired := 0
	for len(commandQueue) > 0 && now.Sub(commandQueue[0].queuedAt) >= commandTTL {
		commandQueue[0] = queuedCommand{}
		commandQueue = commandQueue[1:]
		expired++
	}
	if expired > 0 {
		logger.Info("Discarded %d expired queued commands", expired)
	}
}

func queueCommand(command []byte) {
	if len(command) == 0 {
		return
	}
	commandsMu.Lock()
	defer commandsMu.Unlock()
	now := time.Now()
	// Expire only when the sender inspects the FIFO. Expiring here could
	// remove the in-flight head between peekCommand and consumeCommand.
	if len(commandQueue) >= maxQueuedCommands {
		logger.Error("Command queue full (%d), dropping the newest command", maxQueuedCommands)
		return
	}
	// Own the bytes: MQTT callbacks can safely return after enqueuing.
	commandQueue = append(commandQueue, queuedCommand{
		data: append([]byte(nil), command...),
		queuedAt: now,
	})
}

// nextCommand is also used by callers that unconditionally consume a command.
func nextCommand() ([]byte, int) {
	commandsMu.Lock()
	defer commandsMu.Unlock()
	expireQueuedCommandsLocked(time.Now())
	if len(commandQueue) == 0 {
		return nil, 0
	}
	command := commandQueue[0].data
	commandQueue[0] = queuedCommand{}
	commandQueue = commandQueue[1:]
	return command, len(commandQueue)
}

func peekCommand() ([]byte, int) {
	commandsMu.Lock()
	defer commandsMu.Unlock()
	expireQueuedCommandsLocked(time.Now())
	if len(commandQueue) == 0 {
		return nil, 0
	}
	return commandQueue[0].data, len(commandQueue) - 1
}

func consumeCommand() {
	commandsMu.Lock()
	defer commandsMu.Unlock()
	if len(commandQueue) > 0 {
		commandQueue[0] = queuedCommand{}
		commandQueue = commandQueue[1:]
	}
}

// Only an unopened port guarantees that no bytes of the command were sent.
// For other write errors delivery is ambiguous; do not blindly replay.
func sendNextQueuedCommand(send func([]byte) error) (bool, error) {
	command, pending := peekCommand()
	if command == nil {
		return false, nil
	}
	err := send(command)
	if errors.Is(err, serial.ErrNotOpen) {
		return true, err
	}
	consumeCommand()
	if err == nil {
		logger.Info("sent queued command, %d more pending for next cycles", pending)
	}
	return true, err
}

func MakeMQTTConn() (mqtt.Client, mqtt.Token) {
	opts := mqtt.NewClientOptions()

	protocol := "tcp"
	if config.MqttUseTLS {
		protocol = "ssl"
	}

	opts.AddBroker(fmt.Sprintf("%s://%s:%s", protocol, config.MqttServer, config.MqttPort))
	opts.SetPassword(config.MqttPass)
	opts.SetUsername(config.MqttLogin)
	opts.SetClientID(config.MqttClientID)
	opts.SetWill(fmt.Sprintf("%s/Status", config.Mqtt_topic_base), "offline", 1, true)
	opts.SetKeepAlive(MqttKeepalive)
	opts.SetOnConnectHandler(startsub)
	opts.SetConnectRetry(true)
	opts.SetAutoReconnect(true)

	if config.MqttUseTLS {
		tlsConfig := &tls.Config{}

		if config.MqttCaCertFile != "" {
			caCert, err := os.ReadFile(config.MqttCaCertFile)
			if err == nil {
				caCertPool := x509.NewCertPool()
				caCertPool.AppendCertsFromPEM(caCert)
				tlsConfig.RootCAs = caCertPool
			} else {
				logger.Info("Error loading CA certificate: %v", err)
			}
		}

		if config.MqttClientCertFile != "" && config.MqttClientKeyFile != "" {
			cert, err := tls.LoadX509KeyPair(config.MqttClientCertFile, config.MqttClientKeyFile)
			if err == nil {
				tlsConfig.Certificates = []tls.Certificate{cert}
			} else {
				logger.Info("Error loading client certificate/key: %v", err)
			}
		}

		if config.MqttTLSInsecure {
			tlsConfig.InsecureSkipVerify = true
		}

		opts.SetTLSConfig(tlsConfig)
	}

	// connect to broker
	client := mqtt.NewClient(opts)
	//defer client.Disconnect(uint(2))

	token := client.Connect()
	if token.Wait() && token.Error() != nil {
		logger.Info("Fail to connect broker, %v", token.Error())
	}
	return client, token
}

func MakeSwitchTopic(name string, state string) {
	var t AutoDiscoverStruct
	t.Name = fmt.Sprintf("TEST-%s", name)
	t.StateTopic = config.Mqtt_topic_base + "/" + state
	t.CommandTopic = config.Mqtt_set_base + "/" + name
	t.UID = fmt.Sprintf("Aquarea-%s-%s", config.MqttLogin, t.Name)
	SwitchTopics[name] = t

}
func startsub(c mqtt.Client) {
	var t AutoDiscoverStruct
	c.Subscribe("aquarea/+/+/set", 2, HandleMSGfromMQTT)
	c.Subscribe(config.Mqtt_set_base+"/SetHeatpump", 2, HandleSetHeatpump)
	MakeSwitchTopic("SetHeatpump", "Heatpump_State")
	c.Subscribe(config.Mqtt_set_base+"/SetQuietMode", 2, HandleSetQuietMode)
	c.Subscribe(config.Mqtt_set_base+"/SetZ1HeatRequestTemperature", 2, HandleSetZ1HeatRequestTemperature)
	c.Subscribe(config.Mqtt_set_base+"/SetZ1CoolRequestTemperature", 2, HandleSetZ1CoolRequestTemperature)
	c.Subscribe(config.Mqtt_set_base+"/SetZ2HeatRequestTemperature", 2, HandleSetZ2HeatRequestTemperature)
	c.Subscribe(config.Mqtt_set_base+"/SetZ2CoolRequestTemperature", 2, HandleSetZ2CoolRequestTemperature)
	c.Subscribe(config.Mqtt_set_base+"/SetOperationMode", 2, HandleSetOperationMode)
	c.Subscribe(config.Mqtt_set_base+"/SetForceDHW", 2, HandleSetForceDHW)
	MakeSwitchTopic("SetForceDHW", "Force_DHW_State")
	c.Subscribe(config.Mqtt_set_base+"/SetForceDefrost", 2, HandleSetForceDefrost)
	MakeSwitchTopic("SetForceDefrost", "Defrosting_State")
	c.Subscribe(config.Mqtt_set_base+"/SetForceSterilization", 2, HandleSetForceSterilization)
	MakeSwitchTopic("SetForceSterilization", "Sterilization_State")
	c.Subscribe(config.Mqtt_set_base+"/SetHolidayMode", 2, HandleSetHolidayMode)
	MakeSwitchTopic("SetHolidayMode", "Holiday_Mode_State")
	c.Subscribe(config.Mqtt_set_base+"/SetPowerfulMode", 2, HandleSetPowerfulMode)

	t.Name = "TEST-SetPowerfulMode-30min"
	t.CommandTopic = config.Mqtt_set_base + "/SetPowerfulMode"
	t.StateTopic = config.Mqtt_topic_base + "/Powerful_Mode_Time"
	t.UID = fmt.Sprintf("Aquarea-%s-%s", config.MqttLogin, t.Name)
	t.PayloadOn = "1"
	t.StateON = "on"
	t.StateOff = "off"
	t.ValueTemplate = `{%- if value == "1" -%} on {%- else -%} off {%- endif -%}`
	SwitchTopics["SetPowerfulMode1"] = t
	t = AutoDiscoverStruct{}
	t.Name = "TEST-SetPowerfulMode-60min"
	t.CommandTopic = config.Mqtt_set_base + "/SetPowerfulMode"
	t.StateTopic = config.Mqtt_topic_base + "/Powerful_Mode_Time"
	t.UID = fmt.Sprintf("Aquarea-%s-%s", config.MqttLogin, t.Name)
	t.PayloadOn = "2"
	t.StateON = "on"
	t.StateOff = "off"
	t.ValueTemplate = `{%- if value == "2" -%} on {%- else -%} off {%- endif -%}`
	SwitchTopics["SetPowerfulMode2"] = t
	t = AutoDiscoverStruct{}
	t.Name = "TEST-SetPowerfulMode-90min"
	t.CommandTopic = config.Mqtt_set_base + "/SetPowerfulMode"
	t.StateTopic = config.Mqtt_topic_base + "/Powerful_Mode_Time"
	t.UID = fmt.Sprintf("Aquarea-%s-%s", config.MqttLogin, t.Name)
	t.PayloadOn = "3"
	t.StateON = "on"
	t.StateOff = "off"
	t.ValueTemplate = `{%- if value == "3" -%} on {%- else -%} off {%- endif -%}`
	SwitchTopics["SetPowerfulMode3"] = t
	t = AutoDiscoverStruct{}

	c.Subscribe(config.Mqtt_set_base+"/SetDHWTemp", 2, HandleSetDHWTemp)
	c.Subscribe(config.Mqtt_set_base+"/SendRawValue", 2, HandleSendRawValue)
	if config.EnableCommand {
		c.Subscribe(config.Mqtt_set_base+"/OSCommand", 2, HandleOSCommand)
	}

	//Perform additional action...
}

func HandleMSGfromMQTT(mclient mqtt.Client, msg mqtt.Message) {

}

func HandleOSCommand(mclient mqtt.Client, msg mqtt.Message) {
	var cmd *exec.Cmd
	var out2 string
	s := strings.Split(string(msg.Payload()), " ")
	if len(s) < 2 {
		cmd = exec.Command(s[0])
	} else {
		cmd = exec.Command(s[0], s[1:]...)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		// TODO: handle error more gracefully
		out2 = fmt.Sprintf("%s", err)
	}
	comout := fmt.Sprintf("%s - %s", out, out2)
	TOP := fmt.Sprintf("%s/OSCommand/out", config.Mqtt_set_base)
	logger.Info("Published to %s warosc %s", TOP, string(comout))
	token := mclient.Publish(TOP, byte(0), false, comout)
	if token.Wait() && token.Error() != nil {
		logger.Error("Fail to publish, %v", token.Error())
	}

}

func HandleSendRawValue(mclient mqtt.Client, msg mqtt.Message) {
	var command []byte
	cts := strings.TrimSpace(string(msg.Payload()))
	command, err = hex.DecodeString(cts)
	if err != nil {
		logger.Error("%v", err)
	}

	queueCommand(command)
}

func HandleSetOperationMode(mclient mqtt.Client, msg mqtt.Message) {
	var command []byte
	var set_mode byte
	a, _ := strconv.Atoi(string(msg.Payload()))

	switch a {
	case 0:
		set_mode = 18 // Heat
	case 1:
		set_mode = 19 // Cool
	case 2:
		set_mode = 24 // Auto(heat) -> Auto
	case 3:
		set_mode = 33 // DHW
	case 4:
		set_mode = 34 // Heat+DHW
	case 5:
		set_mode = 35 // Cool+DHW
	case 6:
		set_mode = 40 // Auto(heat)+DHW -> Auto+DHW
	case 7:
		set_mode = 24 // Auto(cool) -> Auto
	case 8:
		set_mode = 40 // Auto(cool)+DHW -> Auto+DHW
	case 9:
		set_mode = 24 // Auto
	case 10:
		set_mode = 40 // Auto+DHW
	default:
		set_mode = 0
	}

	logger.Info("set heat pump mode to %d", set_mode)
	command = []byte{0xf1, 0x6c, 0x01, 0x10, 0x00, 0x00, set_mode, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	logger.DebugHex("Set operation mode command", command)
	queueCommand(command)
}

func HandleSetDHWTemp(mclient mqtt.Client, msg mqtt.Message) {
	var command []byte
	var heatpump_state byte

	a, er := strconv.Atoi(string(msg.Payload()))
	if er != nil {
		f, _ := strconv.ParseFloat(string(msg.Payload()), 64)
		a = int(f)
	}

	e := a + 128
	heatpump_state = byte(e)
	logger.Info("set DHW temperature to %d", a)
	command = []byte{0xf1, 0x6c, 0x01, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, heatpump_state, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	logger.DebugHex("Set DHW temp command", command)
	queueCommand(command)
}

func HandleSetPowerfulMode(mclient mqtt.Client, msg mqtt.Message) {
	var command []byte
	var heatpump_state byte

	a, er := strconv.Atoi(string(msg.Payload()))
	if er != nil {
		f, _ := strconv.ParseFloat(string(msg.Payload()), 64)
		a = int(f)
	}

	e := (a + 1) & 0b111
	heatpump_state = byte(e)
	logger.Info("set powerful mode to %d", a)
	command = []byte{0xf1, 0x6c, 0x01, 0x10, 0x00, 0x00, 0x00, heatpump_state, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	logger.DebugHex("Set powerful mode command", command)
	queueCommand(command)
}

func HandleSetHolidayMode(mclient mqtt.Client, msg mqtt.Message) {
	var command []byte
	var heatpump_state byte
	e := 16
	a, er := strconv.Atoi(string(msg.Payload()))
	if er != nil {
		f, _ := strconv.ParseFloat(string(msg.Payload()), 64)
		a = int(f)
	}

	if a == 1 {
		e = 32
	}
	heatpump_state = byte(e)
	logger.Info("set holiday mode to %d", heatpump_state)
	command = []byte{0xf1, 0x6c, 0x01, 0x10, 0x00, heatpump_state, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	logger.DebugHex("Set holiday mode command", command)
	queueCommand(command)
}

func HandleSetForceSterilization(mclient mqtt.Client, msg mqtt.Message) {
	var command []byte
	var heatpump_state byte
	e := 0
	a, er := strconv.Atoi(string(msg.Payload()))
	if er != nil {
		f, _ := strconv.ParseFloat(string(msg.Payload()), 64)
		a = int(f)
	}

	if a == 1 {
		e = 4
	}
	heatpump_state = byte(e)
	logger.Info("set force sterilization mode to %d", heatpump_state)
	command = []byte{0xf1, 0x6c, 0x01, 0x10, 0x00, 0x00, 0x00, 0x00, heatpump_state, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	logger.DebugHex("Set force sterilization command", command)
	queueCommand(command)
}

func HandleSetForceDefrost(mclient mqtt.Client, msg mqtt.Message) {
	var command []byte
	var heatpump_state byte
	e := 0
	a, er := strconv.Atoi(string(msg.Payload()))
	if er != nil {
		f, _ := strconv.ParseFloat(string(msg.Payload()), 64)
		a = int(f)
	}

	if a == 1 {
		e = 2
	}
	heatpump_state = byte(e)
	logger.Info("set force defrost mode to %d", heatpump_state)
	command = []byte{0xf1, 0x6c, 0x01, 0x10, 0x00, 0x00, 0x00, 0x00, heatpump_state, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	logger.DebugHex("Set force defrost command", command)
	queueCommand(command)
}

func HandleSetForceDHW(mclient mqtt.Client, msg mqtt.Message) {
	var command []byte
	var heatpump_state byte
	e := 64
	a, er := strconv.Atoi(string(msg.Payload()))
	if er != nil {
		f, _ := strconv.ParseFloat(string(msg.Payload()), 64)
		a = int(f)
	}

	if a == 1 {
		e = 128
	}
	heatpump_state = byte(e)
	logger.Info("set force DHW mode to %d", heatpump_state)
	command = []byte{0xf1, 0x6c, 0x01, 0x10, heatpump_state, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	logger.DebugHex("Set force DHW command", command)
	queueCommand(command)
}

func HandleSetZ1HeatRequestTemperature(mclient mqtt.Client, msg mqtt.Message) {
	var command []byte
	var request_temp byte
	e, er := strconv.Atoi(string(msg.Payload()))
	if er != nil {
		f, _ := strconv.ParseFloat(string(msg.Payload()), 64)
		e = int(f)
	}

	e = e + 128
	request_temp = byte(e)
	logger.Info("set z1 heat request temperature to %d", request_temp-128)
	command = []byte{0xf1, 0x6c, 0x01, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, request_temp, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	logger.DebugHex("Set Z1 heat request temp command", command)
	queueCommand(command)
}

func HandleSetZ1CoolRequestTemperature(mclient mqtt.Client, msg mqtt.Message) {
	var command []byte
	var request_temp byte
	e, er := strconv.Atoi(string(msg.Payload()))
	if er != nil {
		f, _ := strconv.ParseFloat(string(msg.Payload()), 64)
		e = int(f)
	}
	e = e + 128
	request_temp = byte(e)
	logger.Info("set z1 cool request temperature to %d", request_temp-128)
	command = []byte{0xf1, 0x6c, 0x01, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, request_temp, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	logger.DebugHex("Set Z1 cool request temp command", command)
	queueCommand(command)
}

func HandleSetZ2HeatRequestTemperature(mclient mqtt.Client, msg mqtt.Message) {
	var command []byte
	var request_temp byte
	e, er := strconv.Atoi(string(msg.Payload()))
	if er != nil {
		f, _ := strconv.ParseFloat(string(msg.Payload()), 64)
		e = int(f)
	}
	e = e + 128
	request_temp = byte(e)
	logger.Info("set z2 heat request temperature to %d", request_temp-128)
	command = []byte{0xf1, 0x6c, 0x01, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, request_temp, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	logger.DebugHex("Set Z2 heat request temp command", command)
	queueCommand(command)
}

func HandleSetZ2CoolRequestTemperature(mclient mqtt.Client, msg mqtt.Message) {
	var command []byte
	var request_temp byte
	e, er := strconv.Atoi(string(msg.Payload()))
	if er != nil {
		f, _ := strconv.ParseFloat(string(msg.Payload()), 64)
		e = int(f)
	}
	e = e + 128
	request_temp = byte(e)
	logger.Info("set z2 cool request temperature to %d", request_temp-128)
	command = []byte{0xf1, 0x6c, 0x01, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, request_temp, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	logger.DebugHex("Set Z2 cool request temp command", command)
	queueCommand(command)
}

func HandleSetQuietMode(mclient mqtt.Client, msg mqtt.Message) {
	var command []byte
	var quiet_mode byte

	e, er := strconv.Atoi(string(msg.Payload()))
	if er != nil {
		f, _ := strconv.ParseFloat(string(msg.Payload()), 64)
		e = int(f)
	}
	e = (e + 1) * 8

	quiet_mode = byte(e)
	logger.Info("set Quiet mode to %d", quiet_mode/8-1)
	command = []byte{0xf1, 0x6c, 0x01, 0x10, 0x00, 0x00, 0x00, quiet_mode, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	logger.DebugHex("Set quiet mode command", command)
	queueCommand(command)
}

func HandleSetHeatpump(mclient mqtt.Client, msg mqtt.Message) {
	var command []byte
	var heatpump_state byte

	e := 1
	a, er := strconv.Atoi(string(msg.Payload()))
	if er != nil {
		f, _ := strconv.ParseFloat(string(msg.Payload()), 64)
		a = int(f)
	}
	if a == 1 {
		e = 2
	}

	heatpump_state = byte(e)
	logger.Info("set heatpump state to %d", heatpump_state)
	command = []byte{0xf1, 0x6c, 0x01, 0x10, heatpump_state, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	logger.DebugHex("Set heatpump command", command)
	queueCommand(command)
}

func send_command(command []byte, length int) error {
	err := serialComms.SendCommand(command[:length])
	if err != nil {
		// while the port is down this would log every cycle
		if !errors.Is(err, serial.ErrNotOpen) {
			log.Printf("Failed to send command: %v\n", err)
		}
		return err
	}
	logger.Debug(fmt.Sprintf("sent bytes: %d\n", length))
	return nil
}

// func pushCommandBuffer(command []byte , length int) {
// 	if (commandsInBuffer < MAXCOMMANDSINBUFFER) {
// 	  command_struct* newCommand = new command_struct;
// 	  newCommand->length = length;
// 	  for (int i = 0 ; i < length ; i++) {
// 		newCommand->value[i] = command[i];
// 	  }
// 	  newCommand->next = commandBuffer;
// 	  commandBuffer = newCommand;
// 	  commandsInBuffer++;
// 	}
// 	else {
// 	  log_message("Too much commands already in buffer. Ignoring this commands.");
// 	}
//   }

// serialAvailability observes link-loss generations, so a fast reopen
// cannot hide a disconnect from MQTT clients.
type serialAvailability struct {
	initialized bool
	online bool
	lastLosses uint64
}

func publishSerialStatus(client mqtt.Client, status string) {
	// Match the retained offline Last Will and use QoS 1 on both transitions.
	token := client.Publish(fmt.Sprintf("%s/Status", config.Mqtt_topic_base), byte(1), true, status)
	if token.WaitTimeout(time.Second) && token.Error() != nil {
		logger.Error("Failed to publish serial availability: %v", token.Error())
	}
}

func (a *serialAvailability) update(client mqtt.Client, losses uint64, goodReply bool) {
	if !a.initialized {
		a.initialized = true
		a.lastLosses = losses
		publishSerialStatus(client, "offline")
	}
	if losses != a.lastLosses {
		a.lastLosses = losses
		if a.online {
			publishSerialStatus(client, "offline")
			a.online = false
		}
	}
	if goodReply {
		// Republish retained online after each valid reply, so a broker reconnect
		// also repairs the availability state without another serial transition.
		publishSerialStatus(client, "online")
		a.online = true
	}
}

func readSerial(MC mqtt.Client, MT mqtt.Token) {
	availability := serialAvailability{}
	for {
		// Read packet from serial port using the state machine
		packet := serialComms.Read()

		if packet != nil {
			// Only process 203-byte data packets
			if len(packet) == serial.DataMessageLength {
				// Log statistics periodically
				stats := serialComms.GetStatistics()
				if stats.TotalReads%150 == 0 && stats.TotalReads > 0 {
					log_msg := fmt.Sprintf("Total reads: %d, good reads: %d (%.2f%%)",
						stats.TotalReads, stats.GoodReads, stats.ReadPercentage)
					logger.Info(log_msg)
				}

				// Decode and publish the data
				decode_heatpump_data(packet, MC)

 			} else if len(packet) == serial.OptionalMessageLength {
				// Optional 20-byte packets - could be handled here in the future
				log.Printf("Received optional packet (20 bytes) - not yet implemented")
			}
		}

		availability.update(MC, serialComms.LinkLossCount(), packet != nil && len(packet) == serial.DataMessageLength)

		// Brief pause to avoid tight loop
		time.Sleep(10 * time.Millisecond)
	}
}

func getBit7and8(input byte) string {
	return fmt.Sprintf("%d", (input&0b11)-1)
}

func getBit3and4and5(input byte) string {
	return fmt.Sprintf("%d", ((input>>3)&0b111)-1)
}

func getIntMinus1Times10(input byte) string {
	value := int(input) - 1
	return fmt.Sprintf("%d", value*10)

}

func getIntMinus1Times50(input byte) string {
	value := int(input) - 1
	return fmt.Sprintf("%d", value*50)

}

func unknown(input byte) string {
	return "-1"
}

func getIntMinus128(input byte) string {
	value := int(input) - 128
	return fmt.Sprintf("%d", value)
}

func getIntMinus1Div5(input byte) string {
	value := int(input) - 1
	out := float32(value) / 5
	return fmt.Sprintf("%.2f", out)

}

func getRight3bits(input byte) string {
	return fmt.Sprintf("%d", (input&0b111)-1)

}

func getBit1and2(input byte) string {
	return fmt.Sprintf("%d", (input>>6)-1)

}

func getOpMode(input byte) string {
	switch int(input & 0b111111) {
	case 18:
		return "0" // Heat
	case 19:
		return "1" // Cool
	case 25:
		return "2" // Auto(heat)
	case 33:
		return "3" // DHW
	case 34:
		return "4" // Heat+DHW
	case 35:
		return "5" // Cool+DHW
	case 41:
		return "6" // Auto(heat)+DHW
	case 26:
		return "7" // Auto(cool)
	case 42:
		return "8" // Auto(cool)+DHW
	case 24:
		return "9" // Auto
	case 40:
		return "10" // Auto+DHW
	default:
		return "-1"
	}
}

func getIntMinus1(input byte) string {
	value := int(input) - 1
	return fmt.Sprintf("%d", value)
}

func getPower(input byte) string {
	value := (int(input) - 1) * 200
	return fmt.Sprintf("%d", value)
}

func getBit3and4(input byte) string {
	return fmt.Sprintf("%d", ((input>>4)&0b11)-1)

}

func getBit5and6(input byte) string {
	return fmt.Sprintf("%d", ((input>>2)&0b11)-1)

}

func getPumpFlow(data []byte) string { // TOP1 //
	PumpFlow1 := int(data[170])
	PumpFlow2 := ((float64(data[169]) - 1) / 256)
	PumpFlow := float64(PumpFlow1) + PumpFlow2
	//return String(PumpFlow,2);
	return fmt.Sprintf("%.2f", PumpFlow)
}

func getErrorInfo(data []byte) string { // TOP44 //
	Error_type := int(data[113])
	Error_number := int(data[114]) - 17
	var Error_string string
	switch Error_type {
	case 177: //B1=F type error
		Error_string = fmt.Sprintf("F%02X", Error_number)

	case 161: //A1=H type error
		Error_string = fmt.Sprintf("H%02X", Error_number)

	default:
		Error_string = "No error"

	}
	return Error_string
}

var topicFunctions = map[string]func(byte) string{
	"getBit7and8":         getBit7and8,
	"unknown":             unknown,
	"getRight3bits":       getRight3bits,
	"getIntMinus1Div5":    getIntMinus1Div5,
	"getIntMinus1Times50": getIntMinus1Times50,
	"getIntMinus1Times10": getIntMinus1Times10,
	"getBit3and4and5":     getBit3and4and5,
	"getIntMinus128":      getIntMinus128,
	"getBit1and2":         getBit1and2,
	"getOpMode":           getOpMode,
	"getIntMinus1":        getIntMinus1,
	"getPower":            getPower,
	"getBit5and6":         getBit5and6,
	"getBit3and4":         getBit3and4,
}

// decodeTopic returns the value of topic k in a 203-byte main data packet.
func decodeTopic(data []byte, k int) string {
	v := AllTopics[k]
	switch k {
	case 1:
		return getPumpFlow(data)
	case 11:
		return fmt.Sprintf("%d", int(binary.BigEndian.Uint16([]byte{data[183], data[182]}))-1)
	case 12:
		return fmt.Sprintf("%d", int(binary.BigEndian.Uint16([]byte{data[180], data[179]}))-1)
	case 90:
		return fmt.Sprintf("%d", int(binary.BigEndian.Uint16([]byte{data[186], data[185]}))-1)
	case 91:
		return fmt.Sprintf("%d", int(binary.BigEndian.Uint16([]byte{data[189], data[188]}))-1)
	case 44:
		return getErrorInfo(data)
	}
	f, ok := topicFunctions[v.TopicFunction]
	if !ok {
		logger.Info("NO FUNCTIONS %s", v.TopicFunction)
		return ""
	}
	return f(data[v.TopicBit])
}

func decode_heatpump_data(data []byte, mclient mqtt.Client) {

	updatenow := false

	// 	if (millis() > nextalldatatime) {
	// 	  updatenow = true;
	// 	  nextalldatatime = millis() + UPDATEALLTIME;
	// 	}
	for k, v := range AllTopics {
		Topic_Value := decodeTopic(data, k)

		actDataMu.Lock()
		changed := updatenow || actData[k] != Topic_Value
		if changed {
			actData[k] = Topic_Value
		}
		actDataMu.Unlock()

		if changed {
			logger.Info("received TOP%d %s: %s", k, v.TopicName, Topic_Value)
			if config.Aquarea2mqttCompatible {
				TOP := "aquarea/state/" + fmt.Sprintf("%s/%s", config.Aquarea2mqttPumpID, v.TopicA2M)
				value := strings.ToUpper(strings.TrimSpace(Topic_Value))
				logger.Info("It publishes to %s warosc %s", TOP, string(value))
				token := mclient.Publish(TOP, byte(0), false, value)
				if token.Wait() && token.Error() != nil {
					logger.Error("Fail to publish, %v", token.Error())
				}
			}
			TOP := fmt.Sprintf("%s/%s", config.Mqtt_topic_base, v.TopicName)
			logger.Info("It publishes to %s warosc %s", TOP, string(Topic_Value))
			token := mclient.Publish(TOP, byte(0), false, Topic_Value)
			if token.Wait() && token.Error() != nil {
				logger.Error("Fail to publish, %v", token.Error())
			}

		}

	}

}

