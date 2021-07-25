package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"tinygo.org/x/bluetooth"
)

type socatProxy struct {
	listenAddr string
	destAddr   string
}

func NewProxy(listenAddr, destAddr string) *socatProxy {
	if listenAddr == "" {
		listenAddr = "udp6-listen:5684"
	}
	if destAddr == "" {
		destAddr = "udp:coap.golioth.dev:5684"
	}
	return &socatProxy{
		listenAddr: listenAddr,
		destAddr:   destAddr,
	}
}

func (p *socatProxy) Start() error {
	listenParam := fmt.Sprintf("%s,reuseaddr,fork", p.listenAddr)
	cmd := exec.Command("socat", "-v", "-x", listenParam, p.destAddr)
	f, err := os.OpenFile("socat.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open socat log: %v", err)
	}
	cmd.Stdout = f
	cmd.Stderr = f
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to run socat: %w", err)
	}
	return nil
}

type deviceScanner struct {
	adapter        *bluetooth.Adapter
	deviceLastSeen sync.Map
	scanResults    chan bluetooth.ScanResult
}

func NewScanner() *deviceScanner {
	adapter := bluetooth.DefaultAdapter
	err := adapter.Enable()
	if err != nil {
		log.Fatalf("failed to enable bluetooth: %v", err)
	}
	ch := make(chan bluetooth.ScanResult, 1)
	return &deviceScanner{
		adapter:        adapter,
		scanResults:    ch,
		deviceLastSeen: sync.Map{},
	}
}

func (s *deviceScanner) connectLoop() {
	for result := range s.scanResults {
		log.Println("found IPSP device:", result.Address.String(), result.RSSI, result.LocalName())
		time.Sleep(1 * time.Second)

		connectType := 2 // random
		connectParam := fmt.Sprintf("\"connect %s %d\"", result.Address.String(), connectType)
		connectCmdString := fmt.Sprintf("echo %s > /sys/kernel/debug/bluetooth/6lowpan_control", connectParam)

		log.Printf("trying to register device on 6lowpan control: %v\n", result.LocalName())
		for tries := 0; tries < 10; tries++ {
			cmd := exec.Command("bash", "-c", connectCmdString)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err := cmd.Run()
			if err != nil {
				log.Printf("failed to register 6lowpan device: %v\n", err)
				break
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func (s *deviceScanner) onResult(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
	//log.Println("found device:", result.Address.String(), result.RSSI, result.LocalName())
	isIPSPDevice := result.HasServiceUUID(bluetooth.ServiceUUIDInternetProtocolSupport) || strings.Contains(result.LocalName(), "IPSP")
	if isIPSPDevice {
		key := result.Address.String()
		if lastSeen, found := s.deviceLastSeen.Load(key); found {
			if time.Since(lastSeen.(time.Time)) < 30*time.Second {
				return
			}
		}
		s.deviceLastSeen.Store(key, time.Now())
		s.scanResults <- result
	}
}

func (s *deviceScanner) Start() error {
	log.Println("scanning devices...")
	go s.connectLoop()
	err := s.adapter.Scan(s.onResult)
	if err != nil {
		return fmt.Errorf("failed to start ble scan: %v\n", err)
	}

	return nil
}

type netInterfaceMonitor struct {
	knowDevices map[string]bool
}

func NewInterfaceMonitor() *netInterfaceMonitor {
	return &netInterfaceMonitor{
		knowDevices: make(map[string]bool),
	}
}

func (m *netInterfaceMonitor) Start() error {
	ticker := time.NewTicker(5000 * time.Millisecond)
	for {
		ifs, err := net.Interfaces()
		if err != nil {
			log.Fatalf("failed to list interfaces: %v", err)
		}
		for netName := range m.knowDevices {
			found := false
			for _, netInt := range ifs {
				if netInt.Name == netName {
					found = true
				}
			}
			if !found {
				log.Printf("device %s is no longer connected", netName)
				delete(m.knowDevices, netName)
			}
		}
		for _, netInt := range ifs {
			if strings.HasPrefix(netInt.Name, "bt") {
				if _, found := m.knowDevices[netInt.Name]; found {
					continue
				}
				log.Printf("found bluetooth interface: %s\n", netInt.Name)
				m.knowDevices[netInt.Name] = true
				cmd := exec.Command("ip", "address", "add", "2001:db8::2/64", "dev", netInt.Name)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				err := cmd.Run()
				if err == nil {
					log.Printf("bluetooth ipv6 route registered : %v\n", netInt.Name)
				} else {
					log.Printf("failed to register ipv6 route: %v\n", err)
				}
			}
		}
		<-ticker.C
	}
}

func main() {
	bootstrapSixLowPan()

	proxy := NewProxy("udp6-listen:5684", "udp:coap.golioth.dev:5684")
	go func() {
		err := proxy.Start()
		if err != nil {
			log.Fatalf("failed to start proxy: %v", err)
		}
	}()

	scanner := NewScanner()
	go func() {
		err := scanner.Start()
		if err != nil {
			log.Fatalf("failed to start scanner: %v", err)
		}
	}()

	monitor := NewInterfaceMonitor()
	go func() {
		err := monitor.Start()
		if err != nil {
			log.Fatalf("failed to start monitor: %v", err)
		}
	}()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	log.Println("gateway started")
	<-done
	log.Println("gateway received signal to stop")
}

func bootstrapSixLowPan() {
	// is linux
	if runtime.GOOS != "linux" {
		return
	}
	cmds := []string{
		"modprobe bluetooth_6lowpan",
		"echo 1 > /sys/kernel/debug/bluetooth/6lowpan_enable",
		//"hciconfig hci0 reset",
	}

	for _, cmd := range cmds {
		cmd := exec.Command("bash", "-c", cmd)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			log.Fatalf("failed to run command: %v", err)
		}
	}
}
