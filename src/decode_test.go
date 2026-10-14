package main

import (
	"encoding/hex"
	"testing"
)

// capturedPacket is a 0x10 reply captured on 2026-10-14 from a heat pump without the
// extra data block (byte 0xC7 = 2).
const capturedPacket = "71c801105695e261005a000200000000000000001a16325596955555551900000000000000008080979ca87e7b789900000000000000000000008085158a8585d07b781f7e1f1f79798d8dada36c87ada36c879386718f8a949e8f8a949e858f8a035b79c13d7e7c157b7e000000955555217315a907231aa6000000000000000062d20b475442d20b7266a397a38b9fa3a396a394f8323280a3a4b097a4838087809373011c270000f40b41782e014b793d01c06500843b007b00009b00000a01061301010101020000ff"

// wantTopics holds, per topic number, the name, packet byte and decoder function from
// Egyras/HeishaMon decode.h, and the value HeishaMon publishes for capturedPacket.
var wantTopics = []struct {
	name, value string
	byte        int
	function    string
}{
	{"Heatpump_State", "1", 4, "getBit7and8"},                          // TOP0
	{"Pump_Flow", "11.95", 0, "unknown"},                               // TOP1
	{"Force_DHW_State", "0", 4, "getBit1and2"},                         // TOP2
	{"Quiet_Mode_Schedule", "0", 7, "getBit1and2"},                     // TOP3
	{"Operating_Mode_State", "4", 6, "getOpMode"},                      // TOP4
	{"Main_Inlet_Temp", "31.50", 143, "getIntMinus128"},                // TOP5
	{"Main_Outlet_Temp", "35.75", 144, "getIntMinus128"},               // TOP6
	{"Main_Target_Temp", "35", 153, "getIntMinus128"},                  // TOP7
	{"Compressor_Freq", "38", 166, "getIntMinus1"},                     // TOP8
	{"DHW_Target_Temp", "40", 42, "getIntMinus128"},                    // TOP9
	{"DHW_Temp", "35", 141, "getIntMinus128"},                          // TOP10
	{"Operations_Hours", "15235", 0, "unknown"},                        // TOP11
	{"Operations_Counter", "26047", 0, "unknown"},                      // TOP12
	{"Main_Schedule_State", "1", 5, "getBit1and2"},                     // TOP13
	{"Outside_Temp", "11", 142, "getIntMinus128"},                      // TOP14
	{"Heat_Power_Production", "3600", 194, "getPower"},                 // TOP15
	{"Heat_Power_Consumption", "1000", 193, "getPower"},                // TOP16
	{"Powerful_Mode_Time", "0", 7, "getRight3bits"},                    // TOP17
	{"Quiet_Mode_Level", "3", 7, "getBit3and4and5"},                    // TOP18
	{"Holiday_Mode_State", "0", 5, "getBit3and4"},                      // TOP19
	{"ThreeWay_Valve_State", "0", 111, "getBit7and8"},                  // TOP20
	{"Outside_Pipe_Temp", "3", 158, "getIntMinus128"},                  // TOP21
	{"DHW_Heat_Delta", "-7", 99, "getIntMinus128"},                     // TOP22
	{"Heat_Delta", "6", 84, "getIntMinus128"},                          // TOP23
	{"Cool_Delta", "5", 94, "getIntMinus128"},                          // TOP24
	{"DHW_Holiday_Shift_Temp", "-5", 44, "getIntMinus128"},             // TOP25
	{"Defrosting_State", "0", 111, "getBit5and6"},                      // TOP26
	{"Z1_Heat_Request_Temp", "0", 38, "getIntMinus128"},                // TOP27
	{"Z1_Cool_Request_Temp", "0", 39, "getIntMinus128"},                // TOP28
	{"Z1_Heat_Curve_Target_High_Temp", "45", 75, "getIntMinus128"},     // TOP29
	{"Z1_Heat_Curve_Target_Low_Temp", "35", 76, "getIntMinus128"},      // TOP30
	{"Z1_Heat_Curve_Outside_High_Temp", "7", 78, "getIntMinus128"},     // TOP31
	{"Z1_Heat_Curve_Outside_Low_Temp", "-20", 77, "getIntMinus128"},    // TOP32
	{"Room_Thermostat_Temp", "23", 156, "getIntMinus128"},              // TOP33
	{"Z2_Heat_Request_Temp", "23", 40, "getIntMinus128"},               // TOP34
	{"Z2_Cool_Request_Temp", "28", 41, "getIntMinus128"},               // TOP35
	{"Z1_Water_Temp", "35", 145, "getIntMinus128"},                     // TOP36
	{"Z2_Water_Temp", "22", 146, "getIntMinus128"},                     // TOP37
	{"Cool_Power_Production", "0", 196, "getPower"},                    // TOP38
	{"Cool_Power_Consumption", "0", 195, "getPower"},                   // TOP39
	{"DHW_Power_Production", "0", 198, "getPower"},                     // TOP40
	{"DHW_Power_Consumption", "0", 197, "getPower"},                    // TOP41
	{"Z1_Water_Target_Temp", "35", 147, "getIntMinus128"},              // TOP42
	{"Z2_Water_Target_Temp", "20", 148, "getIntMinus128"},              // TOP43
	{"Error", "No error", 0, "unknown"},                                // TOP44
	{"Room_Holiday_Shift_Temp", "-2", 43, "getIntMinus128"},            // TOP45
	{"Buffer_Temp", "120", 149, "getIntMinus128"},                      // TOP46
	{"Solar_Temp", "-78", 150, "getIntMinus128"},                       // TOP47
	{"Pool_Temp", "-78", 151, "getIntMinus128"},                        // TOP48
	{"Main_Hex_Outlet_Temp", "36", 154, "getIntMinus128"},              // TOP49
	{"Discharge_Temp", "48", 155, "getIntMinus128"},                    // TOP50
	{"Inside_Pipe_Temp", "36", 157, "getIntMinus128"},                  // TOP51
	{"Defrost_Temp", "0", 159, "getIntMinus128"},                       // TOP52
	{"Eva_Outlet_Temp", "7", 160, "getIntMinus128"},                    // TOP53
	{"Bypass_Outlet_Temp", "0", 161, "getIntMinus128"},                 // TOP54
	{"Ipm_Temp", "19", 162, "getIntMinus128"},                          // TOP55
	{"Z1_Temp", "35", 139, "getIntMinus128"},                           // TOP56
	{"Z2_Temp", "23", 140, "getIntMinus128"},                           // TOP57
	{"DHW_Heater_State", "1", 9, "getBit5and6"},                        // TOP58
	{"Room_Heater_State", "1", 9, "getBit7and8"},                       // TOP59
	{"Internal_Heater_State", "0", 112, "getBit7and8"},                 // TOP60
	{"External_Heater_State", "0", 112, "getBit5and6"},                 // TOP61
	{"Fan1_Motor_Speed", "450", 173, "getIntMinus1Times10"},            // TOP62
	{"Fan2_Motor_Speed", "0", 174, "getIntMinus1Times10"},              // TOP63
	{"High_Pressure", "22.8", 163, "getIntMinus1Div5"},                 // TOP64
	{"Pump_Speed", "3200", 171, "getIntMinus1Times50"},                 // TOP65
	{"Low_Pressure", "0", 164, "getIntMinus1Times50"},                  // TOP66
	{"Compressor_Current", "5.4", 165, "getIntMinus1Div5"},             // TOP67
	{"Force_Heater_State", "0", 5, "getBit5and6"},                      // TOP68
	{"Sterilization_State", "0", 117, "getBit5and6"},                   // TOP69
	{"Sterilization_Temp", "65", 100, "getIntMinus128"},                // TOP70
	{"Sterilization_Max_Time", "60", 101, "getIntMinus1"},              // TOP71
	{"Z1_Cool_Curve_Target_High_Temp", "15", 86, "getIntMinus128"},     // TOP72
	{"Z1_Cool_Curve_Target_Low_Temp", "10", 87, "getIntMinus128"},      // TOP73
	{"Z1_Cool_Curve_Outside_High_Temp", "30", 89, "getIntMinus128"},    // TOP74
	{"Z1_Cool_Curve_Outside_Low_Temp", "20", 88, "getIntMinus128"},     // TOP75
	{"Heating_Mode", "0", 28, "getBit7and8"},                           // TOP76
	{"Heating_Off_Outdoor_Temp", "19", 83, "getIntMinus128"},           // TOP77
	{"Heater_On_Outdoor_Temp", "-15", 85, "getIntMinus128"},            // TOP78
	{"Heat_To_Cool_Temp", "15", 95, "getIntMinus128"},                  // TOP79
	{"Cool_To_Heat_Temp", "10", 96, "getIntMinus128"},                  // TOP80
	{"Cooling_Mode", "0", 28, "getBit5and6"},                           // TOP81
	{"Z2_Heat_Curve_Target_High_Temp", "45", 79, "getIntMinus128"},     // TOP82
	{"Z2_Heat_Curve_Target_Low_Temp", "35", 80, "getIntMinus128"},      // TOP83
	{"Z2_Heat_Curve_Outside_High_Temp", "7", 82, "getIntMinus128"},     // TOP84
	{"Z2_Heat_Curve_Outside_Low_Temp", "-20", 81, "getIntMinus128"},    // TOP85
	{"Z2_Cool_Curve_Target_High_Temp", "15", 90, "getIntMinus128"},     // TOP86
	{"Z2_Cool_Curve_Target_Low_Temp", "10", 91, "getIntMinus128"},      // TOP87
	{"Z2_Cool_Curve_Outside_High_Temp", "30", 93, "getIntMinus128"},    // TOP88
	{"Z2_Cool_Curve_Outside_Low_Temp", "20", 92, "getIntMinus128"},     // TOP89
	{"Room_Heater_Operations_Hours", "122", 0, "unknown"},              // TOP90
	{"DHW_Heater_Operations_Hours", "154", 0, "unknown"},               // TOP91
	{"Heat_Pump_Model", "62 D2 0B 47 54 42 D2 0B 72 66", 0, "unknown"}, // TOP92
	{"Pump_Duty", "119", 172, "getIntMinus1"},                          // TOP93
	{"Zones_State", "2", 6, "getBit1and2"},                             // TOP94
	{"Max_Pump_Duty", "119", 45, "getIntMinus1"},                       // TOP95
	{"Heater_Delay_Time", "20", 104, "getIntMinus1"},                   // TOP96
	{"Heater_Start_Delta", "-5", 105, "getIntMinus128"},                // TOP97
	{"Heater_Stop_Delta", "-2", 106, "getIntMinus128"},                 // TOP98
	{"Buffer_Installed", "0", 24, "getBit5and6"},                       // TOP99
	{"DHW_Installed", "1", 24, "getBit7and8"},                          // TOP100
	{"Solar_Mode", "0", 24, "getBit3and4"},                             // TOP101
	{"Solar_On_Delta", "10", 61, "getIntMinus128"},                     // TOP102
	{"Solar_Off_Delta", "5", 62, "getIntMinus128"},                     // TOP103
	{"Solar_Frost_Protection", "5", 63, "getIntMinus128"},              // TOP104
	{"Solar_High_Limit", "80", 64, "getIntMinus128"},                   // TOP105
	{"Pump_Flowrate_Mode", "0", 29, "getBit3and4"},                     // TOP106
	{"Liquid_Type", "0", 20, "getBit1"},                                // TOP107
	{"Alt_External_Sensor", "0", 20, "getBit3and4"},                    // TOP108
	{"Anti_Freeze_Mode", "1", 20, "getBit5and6"},                       // TOP109
	{"Optional_PCB", "1", 20, "getBit7and8"},                           // TOP110
	{"Z1_Sensor_Settings", "1", 22, "getSecondByte"},                   // TOP111
	{"Z2_Sensor_Settings", "2", 22, "getFirstByte"},                    // TOP112
	{"Buffer_Tank_Delta", "5", 59, "getIntMinus128"},                   // TOP113
	{"External_Pad_Heater", "0", 25, "getBit3and4"},                    // TOP114
	{"Water_Pressure", "-0.02", 125, "getIntMinus1Div50"},              // TOP115
	{"Second_Inlet_Temp", "-128", 126, "getIntMinus128"},               // TOP116
	{"Economizer_Outlet_Temp", "-128", 127, "getIntMinus128"},          // TOP117
	{"Second_Room_Thermostat_Temp", "-128", 128, "getIntMinus128"},     // TOP118
	{"External_Control", "0", 23, "getBit7and8"},                       // TOP119
	{"External_Heat_Cool_Control", "0", 23, "getBit5and6"},             // TOP120
	{"External_Error_Signal", "0", 23, "getBit3and4"},                  // TOP121
	{"External_Compressor_Control", "0", 23, "getBit1and2"},            // TOP122
	{"Z2_Pump_State", "1", 116, "getBit1and2"},                         // TOP123
	{"Z1_Pump_State", "1", 116, "getBit3and4"},                         // TOP124
	{"TwoWay_Valve_State", "1", 116, "getBit5and6"},                    // TOP125
	{"ThreeWay_Valve_State2", "0", 116, "getBit7and8"},                 // TOP126
	{"Z1_Valve_PID", "30.0", 177, "getValvePID"},                       // TOP127
	{"Z2_Valve_PID", "0.0", 178, "getValvePID"},                        // TOP128
	{"Bivalent_Control", "0", 26, "getBit7and8"},                       // TOP129
	{"Bivalent_Mode", "0", 26, "getBit5and6"},                          // TOP130
	{"Bivalent_Start_Temp", "-5", 65, "getIntMinus128"},                // TOP131
	{"Bivalent_Advanced_Heat", "0", 26, "getBit3and4"},                 // TOP132
	{"Bivalent_Advanced_DHW", "0", 26, "getBit1and2"},                  // TOP133
	{"Bivalent_Advanced_Start_Temp", "-8", 66, "getIntMinus128"},       // TOP134
	{"Bivalent_Advanced_Stop_Temp", "-2", 68, "getIntMinus128"},        // TOP135
	{"Bivalent_Advanced_Start_Delay", "30", 67, "getIntMinus1"},        // TOP136
	{"Bivalent_Advanced_Stop_Delay", "30", 69, "getIntMinus1"},         // TOP137
	{"Bivalent_Advanced_DHW_Delay", "30", 70, "getIntMinus1"},          // TOP138
	{"Heating_Control", "-1", 30, "getBit5and6"},                       // TOP139
	{"Smart_DHW", "1", 24, "getBit1and2"},                              // TOP140
	{"Quiet_Mode_Priority", "-1", 11, "getBit3and4"},                   // TOP141
	{"Expansion_Valve", "74", 175, "getIntMinus1"},                     // TOP142
	{"DHW_Sensor_Selection", "1", 11, "getBit7and8"},                   // TOP143
}

func mustDecodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTopicsMatchHeishaMon(t *testing.T) {
	if len(AllTopics) != len(wantTopics) {
		t.Errorf("got %d topics, want %d", len(AllTopics), len(wantTopics))
	}
	data := mustDecodeHex(t, capturedPacket)
	for k, want := range wantTopics {
		if k >= len(AllTopics) {
			t.Errorf("TOP%d %s missing", k, want.name)
			continue
		}
		if got := AllTopics[k].TopicName; got != want.name {
			t.Errorf("TOP%d name = %q, want %q", k, got, want.name)
		}
		if got := AllTopics[k].TopicBit; got != want.byte {
			t.Errorf("TOP%d %s byte = %d, want %d", k, want.name, got, want.byte)
		}
		if got := AllTopics[k].TopicFunction; got != want.function {
			t.Errorf("TOP%d %s function = %q, want %q", k, want.name, got, want.function)
		}
		if got := decodeTopic(data, k); got != want.value {
			t.Errorf("TOP%d %s = %q, want %q", k, want.name, got, want.value)
		}
	}
}

// HeishaMon decodes in int, so an empty (0) field gives -1; Go byte arithmetic must not wrap to 255.
func TestDecodersOfAZeroByte(t *testing.T) {
	want := map[string]string{
		"getBit1":             "0",
		"getBit1and2":         "-1",
		"getBit3and4":         "-1",
		"getBit5and6":         "-1",
		"getBit7and8":         "-1",
		"getBit3and4and5":     "-1",
		"getRight3bits":       "-1",
		"getIntMinus1":        "-1",
		"getIntMinus128":      "-128",
		"getIntMinus1Div5":    "-0.2",
		"getIntMinus1Div50":   "-0.02",
		"getIntMinus1Times10": "-10",
		"getIntMinus1Times50": "-50",
		"unknown":             "-1",
		"getValvePID":         "-0.5",
		"getPower":            "-200",
		"getFirstByte":        "-1",
		"getSecondByte":       "-1",
		"getOpMode":           "-1",
	}
	for name, w := range want {
		f, ok := topicFunctions[name]
		if !ok {
			t.Errorf("%s missing", name)
			continue
		}
		if got := f(0); got != w {
			t.Errorf("%s(0) = %q, want %q", name, got, w)
		}
	}
}
