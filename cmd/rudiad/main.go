package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/kardianos/osext"
	"github.com/kardianos/service"
	"github.com/ralreegorganon/rudia"
	log "github.com/sirupsen/logrus"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

func init() {
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{DisableColors: true})

	ef, err := osext.ExecutableFolder()
	if err != nil {
		log.Fatal(err)
	}

	logPath := filepath.Join(ef, "logs", "rudiad.log")

	log.SetOutput(&lumberjack.Logger{
		Filename: logPath,
	})
}

type program struct {
	exit            chan struct{}
	cleanupComplete chan bool
}

func main() {
	svcFlag := flag.String("service", "", "Control the system service.")
	flag.Parse()

	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}

	svcConfig := &service.Config{
		Name:             "rudiad",
		DisplayName:      "rudiad",
		Description:      "rudiad",
		WorkingDirectory: dir,
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	if len(*svcFlag) != 0 {
		err := service.Control(s, *svcFlag)
		if err != nil {
			log.Printf("Valid actions: %q\n", service.ControlAction)
			log.Fatal(err)
		}
		return
	}

	err = s.Run()
	if err != nil {
		log.Fatal(err)
	}
}

func (p *program) Start(s service.Service) error {
	go p.run()
	p.exit = make(chan struct{})
	p.cleanupComplete = make(chan bool)
	return nil
}

func (p *program) run() error {
	ef, err := osext.ExecutableFolder()
	if err != nil {
		log.Fatal(err)
	}

	configPath := filepath.Join(ef, "config.json")

	f, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Fatal(err)
	}

	config := Config{}
	if err = json.Unmarshal(f, &config); err != nil {
		log.Fatal(err)
	}

	repeaters := make([]*rudia.Repeater, 0)

	for _, c := range config.Relays {
		ro := &rudia.RepeaterOptions{
			UpstreamProxyIdleTimeout:    time.Duration(c.UpstreamProxyIdleTimeout) * time.Second,
			UpstreamListenerIdleTimeout: time.Duration(c.UpstreamListenerIdleTimeout) * time.Second,
			RetryInterval:               time.Duration(c.RetryInterval) * time.Second,
		}

		r := rudia.NewRepeater(ro)

		for _, u := range c.Proxy {
			r.Proxy(u)
		}

		for _, c := range c.Push {
			r.Push(c)
		}

		go r.ListenAndAcceptClients(":" + strconv.FormatInt(c.ClientListenerPort, 10))
		go r.ListenAndAcceptUpstreams(":" + strconv.FormatInt(c.UpstreamListenerPort, 10))

		repeaters = append(repeaters, r)
	}

	for {
		select {
		case <-p.exit:
			for _, r := range repeaters {
				r.Shutdown()
			}
			p.cleanupComplete <- true
			return nil
		default:
		}
	}
}

func (p *program) Stop(s service.Service) error {
	close(p.exit)
	<-p.cleanupComplete
	return nil
}

type Config struct {
	Relays []Relay `json:"relays"`
}

type Relay struct {
	ClientListenerPort          int64    `json:"clientListenerPort"`
	UpstreamListenerPort        int64    `json:"upstreamListenerPort"`
	UpstreamListenerIdleTimeout int64    `json:"upstreamListenerIdleTimeout"`
	UpstreamProxyIdleTimeout    int64    `json:"upstreamProxyIdleTimeout"`
	RetryInterval               int64    `json:"retryInterval"`
	Proxy                       []string `json:"proxy"`
	Push                        []string `json:"push"`
}
