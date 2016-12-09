package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	_ "strconv"
	"strings"

	"github.com/Shopify/sarama"
	log "github.com/Sirupsen/logrus"
	"github.com/larskluge/babl-server/kafka"
	. "github.com/larskluge/babl-server/utils"
	bn "github.com/larskluge/babl/bablnaming"
)

//Warning log level

//Broker url
const (
	Version     = "0.0.2"
	TopicEvents = "logs.events"
)

var (
	EventsRegex = regexp.MustCompile("die$|start$|oom$")
)

type Event struct {
	Status string `json:"status"`
	ID     string `json:"id"`
	From   string `json:"from"`
	Type   string `json:"Type"`
	Action string `json:"Action"`
	Actor  struct {
		ID         string `json:"ID"`
		Attributes struct {
			ComDockerSwarmNodeID      string `json:"com.docker.swarm.node.id"`
			ComDockerSwarmServiceID   string `json:"com.docker.swarm.service.id"`
			ComDockerSwarmServiceName string `json:"com.docker.swarm.service.name"`
			ComDockerSwarmTask        string `json:"com.docker.swarm.task"`
			ComDockerSwarmTaskID      string `json:"com.docker.swarm.task.id"`
			ComDockerSwarmTaskName    string `json:"com.docker.swarm.task.name"`
			Image                     string `json:"image"`
			Name                      string `json:"name"`
		} `json:"Attributes"`
	} `json:"Actor"`
	Time     int   `json:"time"`
	TimeNano int64 `json:"timeNano"`
}

func main() {
	log.SetOutput(os.Stderr)
	log.SetFormatter(&log.JSONFormatter{})

	app := configureCli()
	app.Run(os.Args)
}

func run(kafkaBrokers string, dbg bool) {

	if dbg {
		log.SetLevel(log.DebugLevel)
	}
	brokers := strings.Split(kafkaBrokers, ",")
	Cluster := SplitFirst(kafkaBrokers, ".")
	ParseEvents(Cluster, brokers)

}

func ParseEvents(Cluster string, brokers []string) {

	client := *kafka.NewClient(brokers, "sentinel", true)
	defer client.Close()

	consumer, err := sarama.NewConsumerFromClient(client)
	Check(err)
	defer consumer.Close()

	offsetNewest, err := client.GetOffset(TopicEvents, 0, sarama.OffsetNewest)
	Check(err)

	cp, err := consumer.ConsumePartition(TopicEvents, 0, offsetNewest)
	Check(err)
	defer cp.Close()
	for msg := range cp.Messages() {
		var m Event
		err := json.Unmarshal(msg.Value, &m)
		Check(err)
		log.WithFields(log.Fields{"broker": brokers, "event": m}).Debug("Docker Event")
		if m.Type == "container" && EventsRegex.MatchString(m.Status) {
			notify(Cluster, m)
		}
		if m.Type == "container" && m.Status == "oom" {
			notifyOom(Cluster, m)
		}
	}
}

func notify(Cluster string, m Event) {
	log.WithFields(log.Fields{"cluster": Cluster, "instance": m.Actor.Attributes.ComDockerSwarmTaskName, "status": m.Status, "id": m.ID, "from": m.From}).Info("Docker Event")
	name := m.Actor.Attributes.ComDockerSwarmTaskName
	if name == "" {
		name = m.From
	}
	str := fmt.Sprintf("[%s] %s --> %s", Cluster, name, m.Status)
	args := []string{"-c", "sandbox.babl.sh:4445", "babl/events", "-e", "EVENT=babl:error"}
	cmd := exec.Command("/bin/babl", args...)
	cmd.Stdin = strings.NewReader(str)
	err := cmd.Run()
	Check(err)
}

//babl -c 192.168.99.100:4445 babl/oom-restart -e MODULE=larskluge/image-resize -e INSTANCE_ID=7b43d4142a24
func notifyOom(Cluster string, m Event) {
	module := bn.ServiceToModule(m.Actor.Attributes.ComDockerSwarmServiceName)
	args := []string{"-c", Cluster + ".babl.sh:4445", "babl/events", "-e", "EVENT=babl:module:oom", "-e", "MODULE=" + module, "-e", "INSTANCE_ID=" + m.ID}
	log.WithFields(log.Fields{"args": args}).Info("oom-restart")
	cmd := exec.Command("/bin/babl", args...)
	err := cmd.Run()
	Check(err)
}
