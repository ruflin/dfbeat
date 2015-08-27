package beat

import (
	"flag"
	"fmt"
	"github.com/elastic/libbeat/cfgfile"
	"github.com/elastic/libbeat/common"
	"github.com/elastic/libbeat/logp"
	"github.com/elastic/libbeat/outputs"
	"github.com/elastic/libbeat/publisher"
	"github.com/elastic/libbeat/service"
	"os"
	"runtime"
	"time"
)

type Beater interface {
	Init(*Beat) error
	Run(*Beat) error
	//Stop(b *Beat)
}

type InputConfig struct {
	Period *int64
}

type Beat struct {
	Name      string
	Version   string
	isAlive   bool
	Period    time.Duration
	config    ConfigSettings
	publisher publisher.PublisherType
	events    chan common.MapStr
	CmdLine   *flag.FlagSet
	Config ConfigSettings
}

type ConfigSettings struct {
	Input   InputConfig
	Output  map[string]outputs.MothershipConfig
	Logging logp.Logging
	Shipper publisher.ShipperConfig
}

func (beat *Beat) Init() error {
	// Check if version and name var ar set
	beat.CmdLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	return nil
}

func (beat *Beat) SendEvent(m *common.MapStr) {
	beat.events <- *m
}

func (beat *Beat) CommandLineSetup() {

	cfgfile.CmdLineFlags(beat.CmdLine, beat.Name)
	logp.CmdLineFlags(beat.CmdLine)
	service.CmdLineFlags(beat.CmdLine)

	//publishDisabled := CmdLine.Bool("N", false, "Disable actual publishing for testing")
	printVersion := beat.CmdLine.Bool("version", false, "Print version and exit")

	beat.CmdLine.Parse(os.Args[1:])

	if *printVersion {
		fmt.Printf("%s version %s (%s)\n", beat.Name, beat.Version, runtime.GOARCH)
		return
	}
}

func (beat *Beat) ConfigSetup(beater Beater, inputConfig interface{}) {

	config := &ConfigSettings{
		//Input: inputConfig,
	}

	err := cfgfile.Read(config)

	beat.Config = *config

	if err != nil {
		logp.Debug("Log read error", "Error %v\n", err)
	}

	logp.Init(beat.Name, &beat.Config.Logging)

	logp.Debug("main", "Initializing output plugins")

	if err := publisher.Publisher.Init(beat.Config.Output, beat.Config.Shipper); err != nil {
		logp.Critical(err.Error())
		os.Exit(1)
	}

	beat.events = publisher.Publisher.Queue

	logp.Debug(beat.Name, "Init %s", beat.Name)

	if err := beater.Init(beat); err != nil {

		logp.Critical(err.Error())
		os.Exit(1)
	}
}

// internal libbeat function that calls beater Run method
func (beat *Beat) Run(beater Beater) {
	service.BeforeRun()

	service.HandleSignals(beat.stop)

	beat.isAlive = true

	for beat.isAlive {
		time.Sleep(beat.Period)
		err := beater.Run(beat)

		if err != nil {
			logp.Critical("Fetching failed: %v", err)
			os.Exit(1)
		}

	}

	logp.Debug("main", "Cleanup")
	service.Cleanup()
}

func (beat *Beat) stop() {
	beat.isAlive = false

	// TODO: should this call a Stop function on the beater object?
}
