package client

import (
	"errors"
	"github.com/1outres/wrangell/pkg/wrangellpkt"
	"log"
	"net"
	"os"
	"time"
)

type (
	Client interface {
		Connect() error
		Send(pkt wrangellpkt.Packet) error
		Close()
	}

	client struct {
		handler     NewTargetHandler
		targets     map[[6]byte]uint16
		loopCounter uint8
		conn        net.Conn
		address     string
	}

	NewTargetHandler interface {
		Handle(pkt *wrangellpkt.TargetPacket)
	}
)

func (c *client) Send(pkt wrangellpkt.Packet) error {
	if c.conn == nil {
		return errors.New("connection is not established")
	}
	_, err := c.conn.Write(pkt.ToBuffer())
	if err != nil {
		return err
	}
	return nil
}

func (c *client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *client) Connect() error {
	conn, err := net.Dial("udp", c.address)
	if err != nil {
		return err
	}
	c.conn = conn

	errCh := make(chan error)

	go c.read(errCh)

	err = c.sendHello()
	if err != nil {
		log.Printf("failed to send hello packet: %v", err)
		return err
	}

	ticker := time.NewTicker(30 * time.Second)
	for {
		select {
		case err := <-errCh:
			return err
		case <-ticker.C:
			if c.conn == nil {
				break
			}
			err := c.sendHello()
			if err != nil {
				log.Printf("failed to send hello packet: %v", err)
				return err
			}
			c.loopCounter = 0
		}
	}
}

func (c *client) read(errCh chan error) {
	var buf [16]byte
	defer func() {
		c.conn.Close()
		c.conn = nil
	}()

	for {
		c.conn.SetDeadline(time.Now().Add(90 * time.Second))

		n, err := c.conn.Read(buf[:])
		if err != nil {
			log.Printf("failed to read from server: %v", err)
			errCh <- err
			break
		}

		pkt := wrangellpkt.PacketFromBuffer(buf[:n])
		log.Printf("received from %v, %s", c.conn.RemoteAddr(), pkt)

		if pkt.Msg == wrangellpkt.MessageHello {
			count := c.count()

			if pkt.HelloPacket.Count != count {
				if c.loopCounter > 30 {
					log.Printf("loop counter is over 30: %v", c.conn.RemoteAddr())
					errCh <- errors.New("loop counter is over 30")
					break
				}
				c.loopCounter++

				err := c.sendHello()
				if err != nil {
					log.Printf("failed to send hello packet: %v", err)
					errCh <- err
					break
				}
			}

			continue
		} else if pkt.Msg == wrangellpkt.MessageTarget {
			c.targets[[6]byte{byte(pkt.TargetPacket.Ip), byte(pkt.TargetPacket.Ip >> 8), byte(pkt.TargetPacket.Ip >> 16), byte(pkt.TargetPacket.Ip >> 24), byte(pkt.TargetPacket.Port), byte(pkt.TargetPacket.Port >> 8)}] = pkt.TargetPacket.Replicas
			if c.handler != nil {
				c.handler.Handle(pkt.TargetPacket)
			}
		} else {
			log.Printf("invalid message received: %v", c.conn.RemoteAddr())
			continue
		}
	}
}

func (c *client) count() uint16 {
	var count uint16 = 0
	for _, replicas := range c.targets {
		if replicas == 0 {
			count++
		}
	}
	return count
}

func (c *client) sendHello() error {
	helloPacket := wrangellpkt.Packet{
		Msg: wrangellpkt.MessageHello,
		HelloPacket: &wrangellpkt.HelloPacket{
			Count: c.count(),
		},
	}

	_, err := c.conn.Write(helloPacket.ToBuffer())
	if err != nil {
		return err
	}
	return nil
}

func NewClient(handler NewTargetHandler, address string) Client {
	return &client{
		handler: handler,
		targets: map[[6]byte]uint16{},
		address: address,
	}
}

func main() {
	conn, err := net.Dial("udp", "127.0.0.1:8080")
	if err != nil {
		log.Fatalln(err)
		os.Exit(1)
	}
	defer conn.Close()

	n, err := conn.Write([]byte("Ping"))
	if err != nil {
		log.Fatalln(err)
		os.Exit(1)
	}

	if len([]byte("Ping")) != n {
		log.Printf("data size is %d, but sent data size is %d", len([]byte("Ping")), n)
	}

	recvBuf := make([]byte, 1024)

	n, err = conn.Read(recvBuf)
	if err != nil {
		log.Fatalln(err)
		os.Exit(1)
	}

	log.Printf("Received data: %s", string(recvBuf[:n]))
}
