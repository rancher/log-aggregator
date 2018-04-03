package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/rancher/log-aggregator/driver"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var VERSION = "v0.0.0-dev"
var logFileName = "/var/log/rancher-flexvolume.log"

func setLog(file *os.File) *logrus.Logger {
	log := logrus.New()
	log.Out = file
	log.Level = logrus.DebugLevel
	return log
}

func main() {
	file, err := os.OpenFile(logFileName, os.O_CREATE|os.O_APPEND|os.O_RDWR, os.FileMode(0644))
	if err != nil {
		logrus.Errorf("unable to open file %s: %s", logFileName, err)
	}

	defer file.Close()
	logger := setLog(file)

	app := cli.NewApp()
	app.Name = "log-aggregator"
	app.Version = VERSION
	app.Usage = "local-flexvolme driver to mount log to workload logging path"

	app.Commands = getCommand(logger)
	app.Run(os.Args)
}

func getCommand(logger *logrus.Logger) []cli.Command {
	flexVolumeDriver := driver.FlexVolumeDriver{
		Logger: logger,
	}
	return []cli.Command{
		{
			Name:  "init",
			Usage: "init func",
			Action: func(c *cli.Context) error {
				logger.Info("init function call")
				return printResponse(flexVolumeDriver.Init())
			},
		},
		{
			Name:  "mount",
			Usage: "mount func",
			Action: func(c *cli.Context) error {
				return printResponse(flexVolumeDriver.Mount(c.Args()))
			},
		},
		{
			Name:  "unmount",
			Usage: "unmount func",
			Action: func(c *cli.Context) error {
				return printResponse(flexVolumeDriver.Unmount(c.Args()))
			},
		},
	}
}

func printResponse(resp interface{}) error {
	output, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	fmt.Println(string(output))
	return nil
}
