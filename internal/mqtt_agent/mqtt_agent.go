package mqtt_agent

import (
	"fmt"
	"github.com/antigloss/go/logger"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"time"
)

type Msg struct {
	Topic   string
	Payload string
}

type Config struct {
	ClientId    string
	BrokerUrl   string
	User        string
	Pass        string
	TopicPrefix string
}

const (
	DefMqttClientId = "smokey_mqtt_agent" // no longer than 23 characters
	DefBrokerURL    = "tcp://192.168.10.238:1883"
	DefBrokerUser   = ""
	DefBrokerPass   = ""
	DefTopicPrefix  = "smokey/"
)

const (
	DefTopicSubPower1 = "stat/POWER1"
	DefTopicSubPower2 = "stat/POWER2"
	//DefTopicSubResult   = "stat/RESULT"
	DefTopicSubError = "stat/error"
	//DefTopicSubFanMode  = "stat/fanmode"
	DefTopicSubStatus11 = "stat/STATUS11"
	DefTopicSubState    = "tele/STATE"

	DefTopicPubCheckStatus = "cmnd/Status"
	DefTopicPubCheckWater  = "cmnd/TuyaSend8"
	DefTopicPubDiffuser    = "cmnd/Power1"
	DefTopicPubLight       = "cmnd/Power2"
	DefTopicPubLightMode   = "cmnd/TuyaEnum2"
	DefTopicPubLightDim    = "cmnd/Dimmer0"
	DefTopicPubLightColor  = "cmnd/Color1"
)

func TopicSubPower1() string {
	return gConf.TopicPrefix + DefTopicSubPower1
}

func TopicSubPower2() string {
	return gConf.TopicPrefix + DefTopicSubPower2
}

func TopicSubError() string {
	return gConf.TopicPrefix + DefTopicSubError
}

func TopicSubStatus11() string {
	return gConf.TopicPrefix + DefTopicSubStatus11
}

func TopicSubState() string {
	return gConf.TopicPrefix + DefTopicSubState
}

func MsgPubCheckStatus11() (string, string) {
	return gConf.TopicPrefix + DefTopicPubCheckStatus, "11"
}

func MsgPubCheckWater() (string, string) {
	return gConf.TopicPrefix + DefTopicPubCheckWater, ""
}

func onOff(on bool) string {
	if on {
		return "ON"
	}
	return "OFF"
}

func MsgPubSetDiffuser(on bool) (string, string) {
	return gConf.TopicPrefix + DefTopicPubDiffuser, onOff(on)
}

func MsgPubSetLight(on bool) (string, string) {
	return gConf.TopicPrefix + DefTopicPubLight, onOff(on)
}

func MsgPubSetLightMode(mode int) (string, string) {
	return gConf.TopicPrefix + DefTopicPubLightMode, fmt.Sprintf("%d", mode)
}

func MsgPubSetLightDim(dim int) (string, string) {
	return gConf.TopicPrefix + DefTopicPubLightDim, fmt.Sprintf("%d", dim)
}

func MsgPubSetLightColor(color int) (string, string) {
	return gConf.TopicPrefix + DefTopicPubLightColor, fmt.Sprintf("#%06x", color)
}

func FirstN(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func connectionWorker(connectionQueue <-chan bool) {
	//create a ClientOptions struct setting the broker address, clientid, turn
	// opts := MQTT.NewClientOptions().AddBroker("tcp://iot.eclipse.org:1883")
	opts := MQTT.NewClientOptions().AddBroker(gConf.BrokerUrl).SetClientID(gConf.ClientId)
	if gConf.User != "" {
		opts.SetUsername(gConf.User)
	}
	if gConf.Pass != "" {
		opts.SetPassword(gConf.Pass)
	}
	opts.SetAutoReconnect(false) // reconnects will be handled by the worker
	opts.SetConnectionLostHandler(myMqttConnLost)
	opts.SetOnConnectHandler(myMqttConnected)
	opts.SetDefaultPublishHandler(myMqttCallback)
	opts.SetKeepAlive(61 * time.Second)
	opts.SetMaxReconnectInterval(5 * time.Minute)

	gClient = MQTT.NewClient(opts)
	// Important: the retry mechanism, is based on this defer; which
	// will basically spawn a new worker as this function is finished
	defer func() {
		gClient.Disconnect(500) // 500 Millisecond quiesce
		time.Sleep(15000 * time.Millisecond)
		go connectionWorker(connectionQueue) // long lives the worker!
	}()

	logger.Info("connecting to mqtt", gConf.BrokerUrl)
	token := gClient.Connect()
	if !token.WaitTimeout(30*time.Second) || token.Error() != nil {
		logger.Warn("connectionWorker was unable to connect:", token.Error())
		return
	}

	// wait for connected callback and subscribe to topics that we care about
	var isConnected bool
	for {
		select {
		case isConnected = <-connectionQueue:
			if !isConnected {
				continue
			}
		case <-time.After(10 * time.Second):
			logger.Warn("connectionWorker connect callback timeout")
			return
		}
		break
	}
	logger.Trace("connectionWorker connected and got connect callback")

	for _, topic := range gMqttTopics {
		token := gClient.Subscribe(topic, 0, nil)
		if !token.WaitTimeout(20*time.Second) || token.Error() != nil {
			logger.Warnf("connectionWorker was unable to subscribe to %s: %s",
				topic, token.Error())
			return
		}
		logger.Trace("connectionWorker subscribed to", topic)
	}

	for isConnected {
		select {
		case isConnected = <-connectionQueue:
			logger.Info("connectionWorker got connection callback", isConnected)
		case <-time.After(180 * time.Second):
			logger.Trace("connectionWorker happy loop")
		}
	}

	// if we made it here, defer will reconnect...
}

func mqttMessageWorker(mqttSubMsgChannel chan<- Msg, mqttPubMsgChannel <-chan Msg) {
	var mqttMsg MQTT.Message
	var msg Msg

	for {
		select {
		case mqttMsg = <-gMessageQueue:
			msg = Msg{mqttMsg.Topic(), string(mqttMsg.Payload())}
			logger.Tracef("mqttMessageWorker received %s %q...", msg.Topic, FirstN(msg.Payload, 10))
			mqttSubMsgChannel <- msg
		case msg = <-mqttPubMsgChannel:
			token := gClient.Publish(msg.Topic, 0, false, msg.Payload)
			if token.WaitTimeout(10 * time.Second) {
				logger.Tracef("mqttMessageWorker sent %+v", msg)
				time.Sleep(500 * time.Millisecond)
			} else {
				logger.Errorf("mqttMessageWorker timed out sending %+v", msg)
			}
		}
	}
}

func Start(config *Config, mqttSubMsgChannel chan<- Msg) chan<- Msg {
	mqttPubMsgChannel := make(chan Msg, 512)
	gConf = *config

	// build subscribe topics, using config's prefix
	subFuncs := []func() string{
		TopicSubPower1,
		TopicSubPower2,
		TopicSubError,
		TopicSubStatus11,
		TopicSubState,
	}
	for _, subFunc := range subFuncs {
		gMqttTopics = append(gMqttTopics, subFunc())
	}

	go connectionWorker(gConnectionQueue)
	go mqttMessageWorker(mqttSubMsgChannel, mqttPubMsgChannel)

	return mqttPubMsgChannel
}

//define a function for the default message handler
var myMqttCallback MQTT.MessageHandler = func(client MQTT.Client, msg MQTT.Message) {
	//logger.Tracef("mqtt callback: %+v", msg)
	gMessageQueue <- msg
}

var myMqttConnLost MQTT.ConnectionLostHandler = func(client MQTT.Client, err error) {
	logger.Warnf("mqtt lost connection: %s", err)
	gConnectionQueue <- false
}

var myMqttConnected MQTT.OnConnectHandler = func(client MQTT.Client) {
	logger.Infof("mqtt got connected callback. Connect: %t", client.IsConnected())
	gConnectionQueue <- true
}

var gConf Config
var gClient MQTT.Client
var gMessageQueue = make(chan MQTT.Message, 1024)
var gConnectionQueue = make(chan bool)
var gMqttTopics = []string{
	// Note: Additional sub topics will be appended here upon Start
}
