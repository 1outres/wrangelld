package app

import (
	"encoding/binary"
	"github.com/1outres/wrangelld/internal/pkg/xdp"
	"github.com/labstack/gommon/log"
	"github.com/urfave/cli/v2"
	"net"
	"os"
	"os/signal"
)

var version string

type (
	handler struct {
		function func(packet *xdp.SynPacket) error
	}
)

func New() *cli.App {
	app := &cli.App{}
	app.Name = "wrangelld"
	app.Version = version
	app.Description = "Wrangell Daemon"
	app.EnableBashCompletion = true
	app.DisableSliceFlagSeparator = true

	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name: "debug",
		},
		&cli.StringFlag{
			Name:     "ifname",
			Required: true,
		},
	}

	app.Before = func(c *cli.Context) error {
		if c.Bool("debug") {
			log.SetLevel(log.DEBUG)
		}
		return nil
	}

	app.Action = func(c *cli.Context) error {
		handler := newHandler(func(packet *xdp.SynPacket) error {
			return nil
		})

		xdpManager := xdp.NewManager(handler)

		err := xdpManager.Start(c.String("ifname"))
		if err != nil {
			return err
		}

		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt)
		<-interrupt

		xdpManager.Close()

		return nil
	}

	return app
}

func ip2int(ip net.IP) uint32 {
	if len(ip) == 16 {
		return binary.BigEndian.Uint32(ip[12:16])
	}
	return binary.BigEndian.Uint32(ip)
}

func newHandler(function func(packet *xdp.SynPacket) error) xdp.SynHandler {
	return &handler{
		function: function,
	}
}

func (h handler) Handle(packet *xdp.SynPacket) error {
	return nil
}
