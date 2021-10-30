package manager

import (
	"encoding/json"
	"fmt"
	"github.com/antigloss/go/logger"
	"github.com/flavio-fernandes/smokey/internal/mqtt_agent"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	knownErrorValues = map[int64]struct{}{0: {}, 1: {}}
)

const (
	cmdTsDampenInterval   = 6 * time.Second
	DefaultAutoOffSeconds = 3600
)

type OperState struct {
	DiffuserOn string `json:"POWER1"`
	LightOn    string `json:"POWER2"`
	LightColor string `json:"Color"`
	LightDim   int    `json:"Dimmer"`
	UptimeSec  int
	Heap       int
}

type OperState11 struct {
	OperState OperState `json:"StatusSTS"`
}

type OperStateParsed struct {
	DiffuserOn     bool
	LightOn        bool
	LightColor     int
	LightDim       int
	Uptime         string
	Heap           int
	LowWater       bool
	Raw            string
	LastReceiveTs  string
	DiffuserOnSecs int
	LightOnSecs    int
}

type WantedState struct {
	DiffuserOn          bool
	LightOn             bool
	LightColor          int
	LightColorName      string
	LightDim            int
	LightMode           LightMode
	LightModeName       string
	IncreaseDim         bool
	DiffuserAutoOffSecs int
	LightAutoOffSecs    int
	DampenDiffuserTs    time.Time
	DampenLightTs       time.Time
}

type Stats struct {
	PubQueryStatus    uint64
	ParseStateMsgs    uint64
	GetStateHits      uint64
	GetStateWaterHits uint64
}

type command interface {
	run()
}

type aCommand struct {
	f func()
}

func (ac *aCommand) run() {
	ac.f()
}

type sCommand struct {
	sync.Mutex
	f   func() *[]byte
	out *[]byte
}

func (sc *sCommand) run() {
	sc.out = sc.f()
	sc.Unlock()
}

type ManagerState struct {
	WantedState     WantedState
	OperStateParsed OperStateParsed
	Stats           Stats
}

type Manager struct {
	StopChan chan struct{}
	mqttPub  chan<- mqtt_agent.Msg
	mqttSub  <-chan mqtt_agent.Msg
	cmds     chan command
	state    ManagerState
}

func (m *Manager) msgParseStatePower1(raw string) {
	m.state.OperStateParsed.DiffuserOn = strings.ToLower(raw) == "on"
	if !m.state.OperStateParsed.DiffuserOn {
		m.state.OperStateParsed.DiffuserOnSecs = 0
	}
}

func (m *Manager) msgParseStatePower2(raw string) {
	m.state.OperStateParsed.LightOn = strings.ToLower(raw) == "on"
	if !m.state.OperStateParsed.LightOn {
		m.state.OperStateParsed.LightOnSecs = 0
	}
}

func (m *Manager) msgParseStateCommon(o *OperState, raw string) {
	if o.DiffuserOn == "" || o.LightOn == "" || o.LightColor == "" {
		logger.Errorf("Ignoring unexpected operstate: %+v parse: %s", o, raw)
		return
	}
	m.state.OperStateParsed.DiffuserOn = strings.ToLower(o.DiffuserOn) == "on"
	if !m.state.OperStateParsed.DiffuserOn {
		m.state.OperStateParsed.DiffuserOnSecs = 0
	}
	m.state.OperStateParsed.LightOn = strings.ToLower(o.LightOn) == "on"
	if !m.state.OperStateParsed.LightOn {
		m.state.OperStateParsed.LightOnSecs = 0
	}
	if lightColor, err := parseOperLightColor(o.LightColor); err == nil {
		m.state.OperStateParsed.LightColor = lightColor
	}
	m.state.OperStateParsed.LightDim = o.LightDim
	m.state.OperStateParsed.Uptime = secondsToHuman(o.UptimeSec)
	m.state.OperStateParsed.Heap = o.Heap
	m.state.OperStateParsed.Raw = raw
	m.state.OperStateParsed.LastReceiveTs = ts()

	m.state.Stats.ParseStateMsgs += 1
}

func (m *Manager) msgParseState(payload string) {
	logger.Tracef("msgParseState: %s", mqtt_agent.FirstN(payload, 15))
	//{"Time":"2021-10-17T17:26:43","Uptime":"16T22:35:15","UptimeSec":1463715,"Heap":27,"SleepMode":"Dynamic","Sleep":50,"LoadAvg":38,"MqttCount":5,"POWER1":"ON","POWER2":"OFF","Dimmer":100,"Color":"FF2A00","HSBColor":"10,100,100","Channel":[100,16,0],"Scheme":0,"Fade":"OFF","Speed":1,"LedTable":"ON","Wifi":{"AP":1,"SSId":"ffiot","BSSId":"F8:BB:BF:95:0F:93","Channel":1,"Mode":"11n","RSSI":74,"Signal":-63,"LinkCount":3,"Downtime":"0T00:00:09"}}

	var o OperState
	err := json.Unmarshal([]byte(payload), &o)
	if err != nil {
		logger.Errorf("Ignoring unexpected operstate: %+v err: %v parse: %s", o, err, payload)
		return
	}
	m.msgParseStateCommon(&o, payload)
}
func (m *Manager) msgParseStatus11(payload string) {
	logger.Tracef("msgParseStatus11: %s", mqtt_agent.FirstN(payload, 15))
	//{"StatusSTS":{"Time":"2021-10-17T17:27:21","Uptime":"16T22:35:53","UptimeSec":1463753,"Heap":27,"SleepMode":"Dynamic","Sleep":50,"LoadAvg":19,"MqttCount":5,"POWER1":"ON","POWER2":"OFF","Dimmer":100,"Color":"FF2A00","HSBColor":"10,100,100","Channel":[100,16,0],"Scheme":0,"Fade":"OFF","Speed":1,"LedTable":"ON","Wifi":{"AP":1,"SSId":"ffiot","BSSId":"F8:BB:BF:95:0F:93","Channel":1,"Mode":"11n","RSSI":74,"Signal":-63,"LinkCount":3,"Downtime":"0T00:00:09"}}}

	// https://www.sohamkamani.com/golang/json/
	var operState11 OperState11
	err := json.Unmarshal([]byte(payload), &operState11)
	if err != nil {
		logger.Errorf("Ignoring unexpected statusSts err: %v parse: %s", err, payload)
		return
	}
	m.msgParseStateCommon(&operState11.OperState, payload)
}
func (m *Manager) msgParseSmokeyError(payload string) {
	payload2 := strings.Split(strings.ToLower(payload), "x")
	value, err := strconv.ParseInt(payload2[len(payload2)-1], 16, 64)
	if err != nil {
		logger.Errorf("Conversion failed for device payload %v: %s", payload, err)
		return
	}
	if _, found := knownErrorValues[value]; !found {
		logger.Warnf("Unexpected error value from device: %d (from %q)", value, payload)
	}
	newLowWater := value&1 == 1
	if newLowWater == m.state.OperStateParsed.LowWater {
		// no change
		return
	}
	m.state.OperStateParsed.LowWater = newLowWater

	if newLowWater {
		logger.Warn("Diffuser is low in water: please refill")
		m.state.WantedState.DiffuserOn = false
	} else {
		logger.Info("Diffuser has water now: nice")
	}
}

func (m *Manager) mainLoop() {
	defer func() { close(m.StopChan) }()
	timeout := time.After(1 * time.Hour)
	secondTick := time.Tick(1 * time.Second)
	checkStatusTickFast := time.Tick(15 * time.Second)
	checkStatusTickSlow := time.Tick(5 * time.Minute)
	var msg mqtt_agent.Msg
	var cmd command
	//mgrloop:
	for {
		select {
		case msg = <-m.mqttSub:
			switch msg.Topic {
			case mqtt_agent.TopicSubPower1():
				m.msgParseStatePower1(msg.Payload)
			case mqtt_agent.TopicSubPower2():
				m.msgParseStatePower2(msg.Payload)
			case mqtt_agent.TopicSubState():
				m.msgParseState(msg.Payload)
			case mqtt_agent.TopicSubStatus11():
				m.msgParseStatus11(msg.Payload)
			case mqtt_agent.TopicSubError():
				m.msgParseSmokeyError(msg.Payload)
			default:
				//logger.Infof("got topic %s payload %s", msg.Topic, msg.Payload)
				logger.Infof("got topic %s payload %q...", msg.Topic, mqtt_agent.FirstN(msg.Payload, 10))
			}
		case <-secondTick:
			m.handleSecondTick()
		case <-checkStatusTickFast:
			if m.state.OperStateParsed.DiffuserOn ||
				m.state.OperStateParsed.LightOn ||
				m.state.OperStateParsed.DiffuserOn != m.state.WantedState.DiffuserOn ||
				m.state.OperStateParsed.LightOn != m.state.WantedState.LightOn {
				m.cmdPubQueryStatus(nil)
			}
			m.bumpIncreaseDim()
		case <-checkStatusTickSlow:
			m.cmdPubQueryStatus(nil)
		case cmd = <-m.cmds:
			cmd.run()
		case <-timeout:
			logger.Trace("manager happy loop")
			//logger.Info("timing out on manager")
			//break mgrloop
		}
	}
}

func (m *Manager) bumpIncreaseDim() {
	if !m.state.WantedState.LightOn ||
		!m.state.OperStateParsed.LightOn ||
		m.state.WantedState.LightMode != Sunshine {
		return
	}

	if m.state.WantedState.LightDim >= 99 {
		// Dim reach limit, change mode to crazy
		logger.Info("Sunshine mode reached max bright. Switching to Crazy mode")
		newAutoOffSecs := m.recalculateLightAutoOff()
		m.cmdLightOn(newAutoOffSecs, Crazy, "")
	} else {
		m.cmdLightDim(m.state.WantedState.LightDim + 1)
	}
}

func (m *Manager) recalculateLightAutoOff() int {
	newAutoOffSecs := m.state.WantedState.LightAutoOffSecs
	if newAutoOffSecs > 0 {
		newAutoOffSecs -= m.state.OperStateParsed.LightOnSecs
		// already expired auto off?
		if newAutoOffSecs <= 0 {
			newAutoOffSecs = 1
		}
	}
	return newAutoOffSecs
}

func (m *Manager) handleSecondTick() {
	// Diffuser
	if m.state.OperStateParsed.DiffuserOn != m.state.WantedState.DiffuserOn &&
		time.Now().After(m.state.WantedState.DampenDiffuserTs) {
		logger.Infof("Diffuser not in wanted state: %v", m.state.WantedState.DiffuserOn)
		m.cmdDiffuser(m.state.WantedState.DiffuserOn)
	} else {
		if m.state.OperStateParsed.DiffuserOn {
			m.state.OperStateParsed.DiffuserOnSecs += 1
			if m.state.WantedState.DiffuserOn &&
				m.state.WantedState.DiffuserAutoOffSecs > 0 &&
				m.state.OperStateParsed.DiffuserOnSecs >= m.state.WantedState.DiffuserAutoOffSecs {
				logger.Info("Diffuser expiring auto off")
				m.cmdDiffuserOff()
			}
		}
	}

	// Light
	if m.state.OperStateParsed.LightOn != m.state.WantedState.LightOn &&
		time.Now().After(m.state.WantedState.DampenLightTs) {
		logger.Infof("Light not in wanted state: %v", m.state.WantedState.LightOn)
		if m.state.WantedState.LightOn {
			newAutoOffSecs := m.recalculateLightAutoOff()
			sameStrColor := LightColor(m.state.WantedState.LightColorName)
			savedDim := m.state.WantedState.LightDim
			m.cmdLightOn(newAutoOffSecs, m.state.WantedState.LightMode, sameStrColor)
			if m.state.WantedState.LightMode == Sunshine {
				m.cmdLightDim(savedDim)
			}
		} else {
			m.cmdLightOff()
		}
	} else {
		if m.state.OperStateParsed.LightOn {
			m.state.OperStateParsed.LightOnSecs += 1
			if m.state.WantedState.LightOn &&
				m.state.WantedState.LightAutoOffSecs > 0 &&
				m.state.OperStateParsed.LightOnSecs >= m.state.WantedState.LightAutoOffSecs {
				logger.Info("Light expiring auto off")
				m.cmdLightOff()
			}
		}
	}
}

func (m *Manager) cmdDiffuser(on bool) {
	var msg mqtt_agent.Msg
	msg.Topic, msg.Payload = mqtt_agent.MsgPubSetDiffuser(on)
	m.mqttPub <- msg

	extraInfo := ""
	if on {
		extraInfo = fmt.Sprintf(". Auto off is %d", m.state.WantedState.DiffuserAutoOffSecs)
	}
	logger.Infof("Asking smokey to set diffuser to %s%s", msg.Payload, extraInfo)
	// Also ask for status right after
	m.cmdPubQueryStatus(&msg)

	m.state.OperStateParsed.DiffuserOnSecs = 0
	m.state.WantedState.DampenDiffuserTs = time.Now().Add(cmdTsDampenInterval)
}

func (m *Manager) cmdDiffuserOn(autoOffSecs int) {
	m.state.WantedState.DiffuserAutoOffSecs = autoOffSecs
	m.state.WantedState.DiffuserOn = true
	m.cmdDiffuser(m.state.WantedState.DiffuserOn)
}

func (m *Manager) cmdDiffuserOff() {
	m.state.WantedState.DiffuserOn = false
	m.cmdDiffuser(m.state.WantedState.DiffuserOn)
}

func (m *Manager) currentState() []byte {
	result, err := json.Marshal(m.state)
	if err != nil {
		logger.Errorf("Unable to encode ManagerState: %+v: %v", m.state, err)
		return nil
	}
	m.state.Stats.GetStateHits += 1
	return result
}

func (m *Manager) currentStateWater() []byte {
	result := []byte("high")
	if m.state.OperStateParsed.LowWater {
		result = []byte("low")
	}
	m.state.Stats.GetStateWaterHits += 1
	return result
}

func (m *Manager) cmdLight(on bool, mode LightMode, color LightColor) {
	var msg mqtt_agent.Msg
	if on != m.state.OperStateParsed.LightOn {
		msg.Topic, msg.Payload = mqtt_agent.MsgPubSetLight(on)
		m.mqttPub <- msg
	}
	modeStr, modeInt := mode.XlateVal()
	if on {
		msg.Topic, msg.Payload = mqtt_agent.MsgPubSetLightMode(modeInt)
		m.mqttPub <- msg
		if mode == Solid || mode == Sunshine {
			m.cmdLightColor(color)
		}
	}

	extraInfo := ""
	if on {
		extraInfo = fmt.Sprintf(". Mode %s. Auto off is %d",
			modeStr, m.state.WantedState.LightAutoOffSecs)
	}
	logger.Infof("Asking smokey to set light to %s%s", msg.Payload, extraInfo)
	// Also ask for status right after
	m.cmdPubQueryStatus(&msg)

	m.state.OperStateParsed.LightOnSecs = 0
	m.state.WantedState.LightMode = mode
	m.state.WantedState.LightModeName, _ = mode.XlateVal()
	m.state.WantedState.IncreaseDim = mode == Sunshine
	m.state.WantedState.DampenLightTs = time.Now().Add(cmdTsDampenInterval)
}

func (m *Manager) cmdLightOn(autoOffSecs int, mode LightMode, color LightColor) {
	m.state.WantedState.LightAutoOffSecs = autoOffSecs
	m.state.WantedState.LightOn = true
	m.cmdLight(m.state.WantedState.LightOn, mode, color)
	if mode == Sunshine {
		m.cmdLightDim(1)
	}
}

func (m *Manager) cmdLightColor(color LightColor) {
	colorInt := color.Int()
	m.state.WantedState.LightColor = colorInt
	m.state.WantedState.LightColorName = string(color)
	var msg mqtt_agent.Msg
	msg.Topic, msg.Payload = mqtt_agent.MsgPubSetLightColor(colorInt)
	m.mqttPub <- msg
	logger.Infof("Asking smokey to set light color to %v (%s)", color, msg.Payload)
}

func (m *Manager) cmdLightDim(dim int) {
	m.state.WantedState.LightDim = dim
	var msg mqtt_agent.Msg
	msg.Topic, msg.Payload = mqtt_agent.MsgPubSetLightDim(dim)
	m.mqttPub <- msg
	logger.Infof("Asking smokey to set light dim to %s", msg.Payload)
}

func (m *Manager) cmdLightOff() {
	m.state.WantedState.LightOn = false
	m.cmdLight(m.state.WantedState.LightOn, Solid, LightColorOff)
}

func (m *Manager) cmdPubQueryStatus(msg *mqtt_agent.Msg) {
	if msg == nil {
		msg = &mqtt_agent.Msg{}
	}
	msg.Topic, msg.Payload = mqtt_agent.MsgPubCheckStatus11()
	m.mqttPub <- *msg
	msg.Topic, msg.Payload = mqtt_agent.MsgPubCheckWater()
	m.mqttPub <- *msg

	m.state.Stats.PubQueryStatus += 1
}

func Start(mqttPub chan<- mqtt_agent.Msg, mqttSub <-chan mqtt_agent.Msg) *Manager {
	mgr := Manager{
		StopChan: make(chan struct{}),
		mqttPub:  mqttPub,
		mqttSub:  mqttSub,
		cmds:     make(chan command, 1),
	}
	go mgr.mainLoop()
	return &mgr
}

func (m *Manager) CurrState() []byte {
	cmd := sCommand{
		f: func() *[]byte {
			result := m.currentState()
			return &result
		},
	}
	cmd.Lock()
	m.cmds <- &cmd
	// wait for sCommand to unlock after getting response
	cmd.Lock()
	return *cmd.out
}

func (m *Manager) CurrStateWater() []byte {
	cmd := sCommand{
		f: func() *[]byte {
			result := m.currentStateWater()
			return &result
		},
	}
	cmd.Lock()
	m.cmds <- &cmd
	// wait for sCommand to unlock after getting response
	cmd.Lock()
	return *cmd.out
}

func (m *Manager) CmdDiffuserOn(autoOffSecs int) {
	cmd := aCommand{f: func() { m.cmdDiffuserOn(autoOffSecs) }}
	m.cmds <- &cmd
}

func (m *Manager) CmdDiffuserOff() {
	cmd := aCommand{f: func() { m.cmdDiffuserOff() }}
	m.cmds <- &cmd
}

func (m *Manager) CmdLightOn(autoOffSecs int, mode LightMode, color LightColor) {
	cmd := aCommand{f: func() { m.cmdLightOn(autoOffSecs, mode, color) }}
	m.cmds <- &cmd
}

func (m *Manager) CmdLightColor(color LightColor) {
	cmd := aCommand{f: func() {
		colorInt := color.Int()
		if colorInt == 0 {
			m.cmdLightOff()
		} else if m.state.WantedState.LightMode != Solid ||
			!m.state.OperStateParsed.LightOn {
			autoOffSecs := DefaultAutoOffSeconds
			m.cmdLightOn(autoOffSecs, Solid, color)
		} else {
			m.cmdLightColor(color)
		}
	}}
	m.cmds <- &cmd
}

func (m *Manager) CmdLightDim(dim int) {
	cmd := aCommand{f: func() { m.cmdLightDim(dim) }}
	m.cmds <- &cmd
}

func (m *Manager) CmdLightOff() {
	cmd := aCommand{f: func() { m.cmdLightOff() }}
	m.cmds <- &cmd
}

func (m *Manager) CmdQueryStatus() {
	cmd := sCommand{
		f: func() *[]byte {
			logger.Trace("Asking smokey for status")
			m.cmdPubQueryStatus(nil)
			return nil
		},
	}
	cmd.Lock()
	m.cmds <- &cmd
	// wait for sCommand to unlock after getting response
	cmd.Lock()
}
