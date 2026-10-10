package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/pando85/GoHeishaMon/src/logger"
	"github.com/rs/xid"
)

// SetCurves takes the HeishaMon JSON format, e.g.
//
//	{"zone1":{"heat":{"target":{"high":35,"low":25},"outside":{"high":15,"low":-15}}}}
//
// Only the values present are changed: the other bytes of the set frame stay
// 0x00, which the heat pump treats as "keep the current setting".

type curveLimits struct {
	High *int `json:"high"`
	Low  *int `json:"low"`
}

type curve struct {
	Target  curveLimits `json:"target"`
	Outside curveLimits `json:"outside"`
}

type zoneCurves struct {
	Heat curve `json:"heat"`
	Cool curve `json:"cool"`
}

type setCurvesPayload struct {
	Zone1 zoneCurves `json:"zone1"`
	Zone2 zoneCurves `json:"zone2"`
}

// curveBytes maps each curve value to its byte in the set frame, which is also
// where the heat pump reports it (value + 128), and to its state topic.
var curveBytes = []struct {
	topic string
	byte  int
	value func(p *setCurvesPayload) *int
}{
	{"Z1_Heat_Curve_Target_High_Temp", 75, func(p *setCurvesPayload) *int { return p.Zone1.Heat.Target.High }},
	{"Z1_Heat_Curve_Target_Low_Temp", 76, func(p *setCurvesPayload) *int { return p.Zone1.Heat.Target.Low }},
	{"Z1_Heat_Curve_Outside_Low_Temp", 77, func(p *setCurvesPayload) *int { return p.Zone1.Heat.Outside.Low }},
	{"Z1_Heat_Curve_Outside_High_Temp", 78, func(p *setCurvesPayload) *int { return p.Zone1.Heat.Outside.High }},
	{"Z2_Heat_Curve_Target_High_Temp", 79, func(p *setCurvesPayload) *int { return p.Zone2.Heat.Target.High }},
	{"Z2_Heat_Curve_Target_Low_Temp", 80, func(p *setCurvesPayload) *int { return p.Zone2.Heat.Target.Low }},
	{"Z2_Heat_Curve_Outside_Low_Temp", 81, func(p *setCurvesPayload) *int { return p.Zone2.Heat.Outside.Low }},
	{"Z2_Heat_Curve_Outside_High_Temp", 82, func(p *setCurvesPayload) *int { return p.Zone2.Heat.Outside.High }},
	{"Z1_Cool_Curve_Target_High_Temp", 86, func(p *setCurvesPayload) *int { return p.Zone1.Cool.Target.High }},
	{"Z1_Cool_Curve_Target_Low_Temp", 87, func(p *setCurvesPayload) *int { return p.Zone1.Cool.Target.Low }},
	{"Z1_Cool_Curve_Outside_Low_Temp", 88, func(p *setCurvesPayload) *int { return p.Zone1.Cool.Outside.Low }},
	{"Z1_Cool_Curve_Outside_High_Temp", 89, func(p *setCurvesPayload) *int { return p.Zone1.Cool.Outside.High }},
	{"Z2_Cool_Curve_Target_High_Temp", 90, func(p *setCurvesPayload) *int { return p.Zone2.Cool.Target.High }},
	{"Z2_Cool_Curve_Target_Low_Temp", 91, func(p *setCurvesPayload) *int { return p.Zone2.Cool.Target.Low }},
	{"Z2_Cool_Curve_Outside_Low_Temp", 92, func(p *setCurvesPayload) *int { return p.Zone2.Cool.Outside.Low }},
	{"Z2_Cool_Curve_Outside_High_Temp", 93, func(p *setCurvesPayload) *int { return p.Zone2.Cool.Outside.High }},
}

func buildSetCurvesCommand(payload []byte) ([]byte, error) {
	var p setCurvesPayload
	dec := json.NewDecoder(bytes.NewReader(payload))
	// a misspelt key would otherwise be ignored and nothing would change
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("invalid SetCurves JSON: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("invalid SetCurves JSON: unexpected data after the object")
	}

	command := make([]byte, PANASONICQUERYSIZE)
	copy(command, []byte{0xf1, 0x6c, 0x01, 0x10})
	set := 0
	for _, c := range curveBytes {
		v := c.value(&p)
		if v == nil {
			continue
		}
		// -128 would encode as 0x00, i.e. "keep"
		if *v < -127 || *v > 127 {
			return nil, fmt.Errorf("%s: %d is out of range -127..127", c.topic, *v)
		}
		command[c.byte] = byte(*v + 128)
		set++
	}
	if set == 0 {
		return nil, errors.New("SetCurves: no curve values given")
	}
	return command, nil
}

func HandleSetCurves(mclient mqtt.Client, msg mqtt.Message) {
	command, err := buildSetCurvesCommand(msg.Payload())
	if err != nil {
		logger.Error("%v", err)
		return
	}
	logger.Info("set curves to %s", msg.Payload())
	logger.DebugHex("Set curves command", command)
	CommandsToSend[xid.New()] = command
}
