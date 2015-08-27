package main

import (
	"github.com/elastic/libbeat/common"
	"github.com/elastic/libbeat/logp"
	"time"
	"github.com/elastic/dfbeat/beat"
	//"fmt"

)

// You can overwrite these, e.g.: go build -ldflags "-X main.Version 1.0.0-beta3"
var Version = "1.0.0-beta2"
var Name = "dfbeat"

// Must make sure all the
type DfBeat struct {
	period      time.Duration
	filesystems []FileSystemStats
	events      chan common.MapStr
}

/*type InputConfig struct {
	Period *int64
}*/

func main() {

	dfbeat := &DfBeat{}

	b := &beat.Beat{
		Version: Version,
		Name:    Name,
	}

	b.Init()

	//
	// This is the space to add your own flagset commands if necessary
	// Use beat.CmdLine to add flags
	//
	b.CommandLineSetup()

	var input beat.InputConfig

	b.ConfigSetup(dfbeat, input)
	b.Run(dfbeat)
}

func (d *DfBeat) Init(b *beat.Beat) error {

	input := b.Config.Input

	if *input.Period > 0 {
		b.Period = time.Duration(*input.Period) * time.Second
	} else {
		b.Period = 1 * time.Second
	}

	logp.Debug("dfbeat", "Period %v\n", d.period)

	return nil
}

func (d *DfBeat) Run(b *beat.Beat) error {
	return exportDiskStats(b)
}


func exportDiskStats(b *beat.Beat) error {

	diskStats, err := GetFilesystemStatList()

	if err != nil {
		logp.Warn("Getting diskstats details: %v", err)
		return err
	}

	// Iterate over stats and send an event per volume
	for _, stat := range diskStats {
		event := common.MapStr{
			"timestamp": common.Time(time.Now()),
			"type":      "disk",
			"disk":      stat,
		}

		b.SendEvent(&event)
	}

	return nil
}
