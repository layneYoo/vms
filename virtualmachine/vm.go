package virtualmachine

import (
	"github.com/vmware/govmomi/vim25/types"
)

var DefaultDNSSuffixes = []string{
	"vsphere.local",
}

var DefaultDNSServers = []string{
	"8.8.8.8",
	"8.8.4.4",
}

type networkInterface struct {
	deviceName       string
	label            string
	ipv4Address      string
	ipv4PrefixLength int
	ipv6Address      string
	ipv6PrefixLength int
	adapterType      string // default vmxnet3 ; TODO: Make "adapter_type" argument
}

type hardDisk struct {
	size     int64
	iops     int64
	initType string
}

// define the vm
type virtualMachine struct {
	name                       string
	folder                     string
	datacenter                 string
	cluster                    string
	resourcePool               string
	datastore                  string
	vcpu                       int
	memoryMb                   int64
	template                   string
	networkInterfaces          []networkInterface
	hardDisks                  []hardDisk
	gateway                    string
	domain                     string
	timeZone                   string
	dnsSuffixes                []string
	dnsServers                 []string
	customConfigurations       map[string](types.AnyType)
	customizationSpecification map[string](types.AnyType)

	host string
}

// vcenter configure
type Config struct {
	User          string
	Password      string
	VCenterServer string
}

//var chs []chan string = make([]chan string, 2)
