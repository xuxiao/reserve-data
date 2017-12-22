package binance

import (
	"os"
)

type Interface interface {
	PublicEndpoint() string
	AuthenticatedEndpoint() string
}

type RealInterface struct{}

func (self *RealInterface) PublicEndpoint() string {
	return "https://www.binance.com"
}

func (self *RealInterface) AuthenticatedEndpoint() string {
	return "https://www.binance.com"
}

func NewRealInterface() *RealInterface {
	return &RealInterface{}
}

type SimulatedInterface struct{}

func (self *SimulatedInterface) baseurl() string {
	baseurl := "127.0.0.1"
	if len(os.Args) > 1 {
		baseurl = os.Args[1]
	}
	return baseurl + ":5100"
}

func (self *SimulatedInterface) PublicEndpoint() string {
	return self.baseurl()
}

func (self *SimulatedInterface) AuthenticatedEndpoint() string {
	return self.baseurl()
}

func NewSimulatedInterface() *SimulatedInterface {
	return &SimulatedInterface{}
}

type KovanInterface struct{}

func (self *KovanInterface) baseurl() string {
	baseurl := "127.0.0.1"
	if len(os.Args) > 1 {
		baseurl = os.Args[1]
	}
	return baseurl + ":5100"
}

func (self *KovanInterface) PublicEndpoint() string {
	return "https://www.binance.com"
}

func (self *KovanInterface) AuthenticatedEndpoint() string {
	return self.baseurl()
}

func NewKovanInterface() *KovanInterface {
	return &KovanInterface{}
}

type DevInterface struct{}

func (self *DevInterface) PublicEndpoint() string {
	return "https://www.binance.com"
	// return "http://192.168.25.16:5100"
}

func (self *DevInterface) AuthenticatedEndpoint() string {
	return "https://www.binance.com"
	// return "http://192.168.25.16:5100"
}

func NewDevInterface() *DevInterface {
	return &DevInterface{}
}

type SocketInterface struct{}

func (self *SocketInterface) PuclicEndpoint() string {
	return "wss://stream.binance.com:9443/ws/"
}

func (self *SocketInterface) AuthenticatedEndpoint() string {
	return "wss://stream.binance.com:9443/ws/"
}

func NewSocketInterface() *SocketInterface {
	return &SocketInterface{}
}
