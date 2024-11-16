package app

import (
	"os"
	"os/signal"
	"time"

	"github.com/1outres/wrangell/pkg/wrangellpkt"
	"github.com/1outres/wrangelld/internal/pkg/client"
	"github.com/1outres/wrangelld/internal/pkg/xdp"
	"github.com/labstack/gommon/log"
	"github.com/urfave/cli/v2"
)

var version string

type (
	targetHandler struct {
		xdpManager xdp.Manager
	}
)

func (t *targetHandler) setXdpManager(xdpManager xdp.Manager) {
	t.xdpManager = xdpManager
}

func (t *targetHandler) Handle(pkt *wrangellpkt.TargetPacket) {
	if t.xdpManager == nil {
		log.Error("xdp manager is not set")
		return
	}

	var err error
	if pkt.Replicas == 0 {
		err = t.xdpManager.SetTargetInfo(pkt.Ip, pkt.Port)
	} else {
		err = t.xdpManager.RemoveTargetInfo(pkt.Ip)
	}

	if err != nil {
		log.Error(err)
	}
}

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
			Name:  "ifname",
			Value: "eth0",
		},
	}

	app.Before = func(c *cli.Context) error {
		if c.Bool("debug") {
			log.SetLevel(log.DEBUG)
		}
		return nil
	}

	app.Action = func(c *cli.Context) error {

		handler := &targetHandler{}
		udpClient := client.NewClient(handler, "wrangell-udp-service.wrangell-system.svc:3030")
		defer udpClient.Close()

		xdpManager := xdp.NewManager(udpClient)
		handler.setXdpManager(xdpManager)

		err := xdpManager.Start(c.String("ifname"))
		if err != nil {
			return err
		}

		go func() {
			for {
				if xdpManager.IsReady() {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			err := udpClient.Connect()
			if err != nil {
				log.Error(err, "unable to start server")
				os.Exit(1)
			}
		}()

		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt)
		<-interrupt

		xdpManager.Close()

		return nil
	}

	return app
}
