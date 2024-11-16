package xdp

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/1outres/wrangell/pkg/wrangellpkt"
	"github.com/1outres/wrangelld/internal/pkg/client"
	"github.com/1outres/wrangelld/xdp"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/labstack/gommon/log"
	"net"
	"os"
	"strconv"
)

type (
	SynHandler interface {
		Handle(packet *SynPacket) error
	}

	SynPacket struct {
		SrcIp   uint32
		DstIp   uint32
		SrcPort uint16
		DstPort uint16
		Seq     uint32
	}

	Manager interface {
		Start(iface string) error
		SetTargetInfo(ip uint32, port uint16) error
		RemoveTargetInfo(ip uint32) error
		IsReady() bool
		Close()
	}

	manager struct {
		udpClient      client.Client
		ready          bool
		targetsMap     *ebpf.Map
		deferFunctions []func()
	}
)

const (
	MetadataSize = 16
)

func (m *manager) Start(ifname string) error {
	log.Debug("Removing memlock...")
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatal("Removing memlock:", err)
	}

	log.Debug("Loading eBPF objects...")
	objs, err := xdp.GetObjects()
	if err != nil {
		log.Fatal("Loading eBPF objects:", err)
	}

	m.deferFunctions = append(m.deferFunctions, func() {
		log.Debug("Closing eBPF objects...")
		err := objs.Close()
		if err != nil {
			log.Fatal("Closing eBPF objects:", err)
		}
	})

	log.Debugf("Getting interface %s...", ifname)
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		log.Fatalf("Getting interface %s: %s", ifname, err)
	}

	log.Debugf("Attaching XDP...")
	l, err := link.AttachXDP(link.XDPOptions{
		Program:   objs.Wrangell,
		Interface: iface.Index,
		Flags:     link.XDPGenericMode,
	})
	if err != nil {
		log.Fatal("Attaching XDP:", err)
	}

	m.deferFunctions = append(m.deferFunctions, func() {
		log.Debugf("Closing XDP link...")
		err := l.Close()
		if err != nil {
			log.Fatal("Closing XDP link:", err)
		}
	})

	m.ready = true
	m.targetsMap = objs.Targets

	log.Debugf("Creating perf event reader...")
	perfEvent, err := perf.NewReader(objs.Perfmap, 4096)
	if err != nil {
		log.Fatal("Creating perf event reader:", err)
	}

	go func() {
		var event SynPacket
		for {
			log.Debugf("Reading perf event...")
			evnt, err := perfEvent.Read()
			if err != nil {
				if errors.Is(errors.Unwrap(err), perf.ErrClosed) {
					break
				}
				log.Fatal("Reading perf event:", err)
				os.Exit(1)
			}
			reader := bytes.NewReader(evnt.RawSample)
			if err := binary.Read(reader, binary.LittleEndian, &event); err != nil {
				log.Fatal("Reading perf event:", err)
				continue
			}

			packet := ""

			packet += fmt.Sprintf("TCP: %v:%d -> %v:%d %d\n",
				intToIpv4(event.SrcIp), ntohs(event.SrcPort),
				intToIpv4(event.DstIp), ntohs(event.DstPort),
				ntohl(event.Seq),
			)
			if len(evnt.RawSample)-MetadataSize > 0 {
				packet += fmt.Sprintln(hex.Dump(evnt.RawSample[MetadataSize:]))
			}

			log.Debug(packet)

			err = m.udpClient.Send(wrangellpkt.Packet{
				Msg: wrangellpkt.MessageRequest,
				ReqPacket: &wrangellpkt.ReqPacket{
					Ip:   ntohl(event.DstIp),
					Port: ntohs(event.DstPort),
				},
			})
			if err != nil {
				log.Fatal("Handling event:", err)
			}
		}
	}()

	return nil
}

func (m *manager) SetTargetInfo(ip uint32, port uint16) error {
	if m.targetsMap == nil {
		return errors.New("not Available")
	}

	err := m.targetsMap.Put(ip, port)
	if err != nil {
		return err
	}

	return nil
}

func (m *manager) RemoveTargetInfo(ip uint32) error {
	if m.targetsMap == nil {
		return errors.New("not Available")
	}

	err := m.targetsMap.Delete(ip)
	if err != nil {
		if err.Error() == ebpf.ErrKeyNotExist.Error() {
			return nil
		}
		return err
	}

	return nil
}

func (m *manager) IsReady() bool {
	log.Debugf("IsReady: %s", strconv.FormatBool(m.ready))
	return m.ready
}

func (m *manager) Close() {
	for _, f := range m.deferFunctions {
		f()
	}
}

func intToIpv4(ip uint32) net.IP {
	res := make([]byte, 4)
	binary.LittleEndian.PutUint32(res, ip)
	return res
}

func ntohs(value uint16) uint16 {
	return ((value & 0xff) << 8) | (value >> 8)
}

func ntohl(value uint32) uint32 {
	return ((value & 0xff) << 24) | ((value & 0xff00) << 8) | ((value & 0xff0000) >> 8) | (value >> 24)
}

func generateKey(ip uint32, port uint16) uint64 {
	return uint64(ip<<16) | uint64(port) // IPアドレスとポートを64ビット整数に組み合わせ
}

func NewManager(udpClient client.Client) Manager {
	return &manager{
		udpClient:      udpClient,
		deferFunctions: make([]func(), 0),
	}
}
