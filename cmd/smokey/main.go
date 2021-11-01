package main

import (
	"flag"
	"fmt"
	"github.com/antigloss/go/logger"
	"github.com/flavio-fernandes/smokey/internal/manager"
	"github.com/flavio-fernandes/smokey/internal/mqtt_agent"
	"github.com/flavio-fernandes/smokey/internal/web"
	"os"
	"strconv"
)

const (
	DefaultLogDir     = "/tmp/smokey_log"
	DefaultListenPort = 8080
)

func main() {
	defaultLogDir := os.Getenv("LOGDIR")
	if defaultLogDir == "" {
		defaultLogDir = DefaultLogDir
	}
	MqttConfig := mqtt_agent.Config{
		ClientId:    mqtt_agent.DefMqttClientId,
		BrokerUrl:   mqtt_agent.DefBrokerURL,
		User:        mqtt_agent.DefBrokerUser,
		Pass:        mqtt_agent.DefBrokerPass,
		TopicPrefix: mqtt_agent.DefTopicPrefix,
	}
	defaultListenPort := DefaultListenPort
	if i, err := strconv.ParseInt(os.Getenv("LISTENPORT"), 10, 16); err == nil {
		defaultListenPort = int(i)
	}

	debugParamPtr := flag.Bool("debug", false, "enable trace level logs")
	logDirParamPtr := flag.String("logdir", defaultLogDir, "or use env LOGDIR to override")
	clientIdParamPtr := flag.String("client", MqttConfig.ClientId, "mqtt client id")
	brokerUrlParamPtr := flag.String("broker", MqttConfig.BrokerUrl, "mqtt broker url")
	userParamPtr := flag.String("user", MqttConfig.User, "mqtt username")
	passParamPtr := flag.String("pass", MqttConfig.Pass, "mqtt password")
	topicPrefixParamPtr := flag.String("topic", MqttConfig.TopicPrefix, "mqtt topic device prefix")
	listenPortPtr := flag.Int("listenport", defaultListenPort, "or use LISTENPORT to override")
	advertiseStatePtr := flag.Bool("advertise", false, "mqtt publish state of diffuser/light")
	flag.Parse()

	MqttConfig.ClientId = *clientIdParamPtr
	MqttConfig.BrokerUrl = *brokerUrlParamPtr
	MqttConfig.User = *userParamPtr
	MqttConfig.Pass = *passParamPtr
	MqttConfig.TopicPrefix = *topicPrefixParamPtr

	// https://github.com/antigloss/go/blob/f29271b1356642b597925cfa554f4997bddb5cee/logger/logger.go#L83
	loggerConfig := logger.Config{
		LogDir:          *logDirParamPtr,
		LogFileMaxSize:  4,
		LogFileMaxNum:   20,
		LogFileNumToDel: 5,
		LogLevel:        logger.LogLevelInfo,
		LogDest:         logger.LogDestFile,
	}
	if *debugParamPtr {
		loggerConfig.LogLevel = logger.LogLevelTrace
		loggerConfig.LogDest = logger.LogDestBoth
	}
	err := logger.Init(&loggerConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			fmt.Sprint("logger init failed ", *logDirParamPtr, " ", err, "\n"))
		os.Exit(1)
	}

	mqttSubMsgChannel := make(chan mqtt_agent.Msg, 1024)
	mqttPubMsgChannel := mqtt_agent.Start(&MqttConfig, mqttSubMsgChannel)
	mgr := manager.Start(mqttPubMsgChannel, mqttSubMsgChannel, *advertiseStatePtr)
	web.Start(mgr, fmt.Sprintf("%d", *listenPortPtr))

	for {
		select {
		case <-mgr.StopChan:
			logger.Infof("stopping main application")
			return
		}
	}
}
