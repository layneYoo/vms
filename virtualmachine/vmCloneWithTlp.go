package virtualmachine

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"xlei/vmMulti/g"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/guest"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	//"github.com/vmware/govmomi/vim25"
	"github.com/Masterminds/glide/msg"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
)

// addHardDisk adds a new Hard Disk to the VirtualMachine.
func addHardDisk(vm *object.VirtualMachine, size, iops int64, diskType string, datastore *object.Datastore) error {
	devices, err := vm.Device(context.TODO())
	if err != nil {
		return err
	}
	//log.Printf("[DEBUG] vm devices: %#v\n", devices)

	controller, err := devices.FindDiskController("scsi")
	if err != nil {
		return err
	}
	//log.Printf("[DEBUG] disk controller: %#v\n", controller)

	disk := devices.CreateDisk(controller, datastore.Reference(), "")
	existing := devices.SelectByBackingInfo(disk.Backing)
	//log.Printf("[DEBUG] disk: %#v\n", disk)

	if len(existing) == 0 {
		disk.CapacityInKB = int64(size * 1024 * 1024)
		if iops != 0 {
			disk.StorageIOAllocation = &types.StorageIOAllocationInfo{
				Limit: iops,
			}
		}
		backing := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo)

		if diskType == "eager_zeroed" {
			// eager zeroed thick virtual disk
			backing.ThinProvisioned = types.NewBool(false)
			backing.EagerlyScrub = types.NewBool(true)
		} else if diskType == "thin" {
			// thin provisioned virtual disk
			backing.ThinProvisioned = types.NewBool(true)
		}

		//log.Printf("[DEBUG] addHardDisk: %#v\n", disk)
		//log.Printf("[DEBUG] addHardDisk: %#v\n", disk.CapacityInKB)

		return vm.AddDevice(context.TODO(), disk)
	} else {
		//log.Printf("[DEBUG] addHardDisk: Disk already present.\n")

		return nil
	}
}

// buildNetworkDevice builds VirtualDeviceConfigSpec for Network Device.
func buildNetworkDevice(f *find.Finder, label, adapterType string) (*types.VirtualDeviceConfigSpec, error) {
	//network, err := f.Network(context.TODO(), "*"+label)
	network, err := f.DefaultNetwork(context.TODO())
	if err != nil {
		return nil, err
	}

	backing, err := network.EthernetCardBackingInfo(context.TODO())
	if err != nil {
		return nil, err
	}

	// add virtualdeviceconnectinfo
	//for _, customnetwork := range network {
	//	fmt.Println(customnetwork.GetVirtualDevice())
	//}

	// make default adapterType
	if adapterType == "" {
		adapterType = "vmxnet3"
	}

	if adapterType == "vmxnet3" {
		return &types.VirtualDeviceConfigSpec{
			Operation: types.VirtualDeviceConfigSpecOperationAdd,
			Device: &types.VirtualVmxnet3{
				VirtualVmxnet: types.VirtualVmxnet{
					VirtualEthernetCard: types.VirtualEthernetCard{
						VirtualDevice: types.VirtualDevice{
							Key:     -1,
							Backing: backing,
						},
						AddressType: string(types.VirtualEthernetCardMacTypeGenerated),
					},
				},
			},
		}, nil
	} else if adapterType == "e1000" {
		return &types.VirtualDeviceConfigSpec{
			Operation: types.VirtualDeviceConfigSpecOperationAdd,
			Device: &types.VirtualE1000{
				VirtualEthernetCard: types.VirtualEthernetCard{
					VirtualDevice: types.VirtualDevice{
						Key:     -1,
						Backing: backing,
					},
					AddressType: string(types.VirtualEthernetCardMacTypeGenerated),
				},
			},
		}, nil
	} else {
		return nil, fmt.Errorf("Invalid network adapter type.")
	}
}

// buildVMRelocateSpec builds VirtualMachineRelocateSpec to set a place for a new VirtualMachine.
func buildVMRelocateSpec(rp *object.ResourcePool, ds *object.Datastore, host *object.HostSystem, vm *object.VirtualMachine, initType string) (types.VirtualMachineRelocateSpec, error) {
	var key int

	devices, err := vm.Device(context.TODO())
	if err != nil {
		return types.VirtualMachineRelocateSpec{}, err
	}
	for _, d := range devices {
		if devices.Type(d) == "disk" {
			key = d.GetVirtualDevice().Key
		}
	}

	isThin := initType == "thin"
	rpr := rp.Reference()
	dsr := ds.Reference()
	hst := host.Reference()
	// TODO : add host
	return types.VirtualMachineRelocateSpec{
		Datastore: &dsr,
		Pool:      &rpr,
		Disk: []types.VirtualMachineRelocateSpecDiskLocator{
			types.VirtualMachineRelocateSpecDiskLocator{
				Datastore: dsr,
				DiskBackingInfo: &types.VirtualDiskFlatVer2BackingInfo{
					DiskMode:        "persistent",
					ThinProvisioned: types.NewBool(isThin),
					EagerlyScrub:    types.NewBool(!isThin),
				},
				DiskId: key,
			},
		},
		Host: &hst,
	}, nil
}

// getDatastoreObject gets datastore object.
func getDatastoreObject(client *govmomi.Client, f *object.DatacenterFolders, name string) (types.ManagedObjectReference, error) {
	s := object.NewSearchIndex(client.Client)
	ref, err := s.FindChild(context.TODO(), f.DatastoreFolder, name)
	if err != nil {
		return types.ManagedObjectReference{}, err
	}
	if ref == nil {
		return types.ManagedObjectReference{}, fmt.Errorf("Datastore '%s' not found.", name)
	}
	//log.Printf("[DEBUG] getDatastoreObject: reference: %#v", ref)
	return ref.Reference(), nil
}

// getVmGuestInfo get guest information.
func getVmGuestInfo(client *govmomi.Client, vm types.ManagedObjectReference) (types.GuestInfo, error) {

	var mvm mo.VirtualMachine

	collector := property.DefaultCollector(client.Client)
	if err := collector.RetrieveOne(context.TODO(), vm, []string{"guest"}, &mvm); err != nil {
		return types.GuestInfo{}, err
	}

	return *mvm.Guest, nil
}

// buildStoragePlacementSpecCreate builds StoragePlacementSpec for create action.
func buildStoragePlacementSpecCreate(f *object.DatacenterFolders, rp *object.ResourcePool, storagePod object.StoragePod, configSpec types.VirtualMachineConfigSpec) types.StoragePlacementSpec {
	vmfr := f.VmFolder.Reference()
	rpr := rp.Reference()
	spr := storagePod.Reference()

	sps := types.StoragePlacementSpec{
		Type:       "create",
		ConfigSpec: &configSpec,
		PodSelectionSpec: types.StorageDrsPodSelectionSpec{
			StoragePod: &spr,
		},
		Folder:       &vmfr,
		ResourcePool: &rpr,
	}
	//log.Printf("[DEBUG] findDatastore: StoragePlacementSpec: %#v\n", sps)
	return sps
}

// buildStoragePlacementSpecClone builds StoragePlacementSpec for clone action.
func buildStoragePlacementSpecClone(c *govmomi.Client, f *object.DatacenterFolders, vm *object.VirtualMachine, rp *object.ResourcePool, storagePod object.StoragePod) types.StoragePlacementSpec {
	vmr := vm.Reference()
	vmfr := f.VmFolder.Reference()
	rpr := rp.Reference()
	spr := storagePod.Reference()

	var o mo.VirtualMachine
	err := vm.Properties(context.TODO(), vmr, []string{"datastore"}, &o)
	if err != nil {
		return types.StoragePlacementSpec{}
	}
	ds := object.NewDatastore(c.Client, o.Datastore[0])
	//log.Printf("[DEBUG] findDatastore: datastore: %#v\n", ds)

	devices, err := vm.Device(context.TODO())
	if err != nil {
		return types.StoragePlacementSpec{}
	}

	var key int
	for _, d := range devices.SelectByType((*types.VirtualDisk)(nil)) {
		key = d.GetVirtualDevice().Key
		//log.Printf("[DEBUG] findDatastore: virtual devices: %#v\n", d.GetVirtualDevice())
	}

	sps := types.StoragePlacementSpec{
		Type: "clone",
		Vm:   &vmr,
		PodSelectionSpec: types.StorageDrsPodSelectionSpec{
			StoragePod: &spr,
		},
		CloneSpec: &types.VirtualMachineCloneSpec{
			Location: types.VirtualMachineRelocateSpec{
				Disk: []types.VirtualMachineRelocateSpecDiskLocator{
					types.VirtualMachineRelocateSpecDiskLocator{
						Datastore:       ds.Reference(),
						DiskBackingInfo: &types.VirtualDiskFlatVer2BackingInfo{},
						DiskId:          key,
					},
				},
				Pool: &rpr,
			},
			PowerOn:  false,
			Template: false,
		},
		CloneName: "dummy",
		Folder:    &vmfr,
	}
	return sps
}

// findDatastore finds Datastore object.
func findDatastore(c *govmomi.Client, sps types.StoragePlacementSpec) (*object.Datastore, error) {
	var datastore *object.Datastore
	//log.Printf("[DEBUG] findDatastore: StoragePlacementSpec: %#v\n", sps)

	srm := object.NewStorageResourceManager(c.Client)
	rds, err := srm.RecommendDatastores(context.TODO(), sps)
	if err != nil {
		return nil, err
	}
	//log.Printf("[DEBUG] findDatastore: recommendDatastores: %#v\n", rds)

	spa := rds.Recommendations[0].Action[0].(*types.StoragePlacementAction)
	datastore = object.NewDatastore(c.Client, spa.Destination)
	//log.Printf("[DEBUG] findDatastore: datastore: %#v", datastore)

	return datastore, nil
}

// deployVirtualMachine deploys a new VirtualMachine.
func (vm *virtualMachine) deployVirtualMachine(c *govmomi.Client) *object.VirtualMachine {
	dc, err := getDatacenter(c, vm.datacenter)
	if err != nil {
		fmt.Println(err.Error())
	}
	g.Check(err != nil, "getDatacenter error", err)
	if !g.Gret {
		g.GoBack()
		return nil
	}
	finder := find.NewFinder(c.Client, true)
	finder = finder.SetDatacenter(dc)

	template, err := finder.VirtualMachine(context.TODO(), vm.template)
	g.Check(err != nil, "get vm template error", err)
	if !g.Gret {
		g.GoBack()
		return nil
	}
	//log.Printf("[DEBUG] template: %#v", template)

	var resourcePool *object.ResourcePool
	if vm.resourcePool == "" {
		if vm.cluster == "" {
			resourcePool, err = finder.DefaultResourcePool(context.TODO())
			g.Check(err != nil, "get resourcePool error", err)
			if !g.Gret {
				g.GoBack()
				return nil
			}
		} else {
			resourcePool, err = finder.ResourcePool(context.TODO(), "*"+vm.cluster+"/Resources")
			g.Check(err != nil, "get resourcePool error", err)
			if !g.Gret {
				g.GoBack()
				return nil
			}
		}
	} else {
		resourcePool, err = finder.ResourcePool(context.TODO(), vm.resourcePool)
		g.Check(err != nil, "get resourcePool error", err)
		if !g.Gret {
			g.GoBack()
			return nil
		}
	}
	//log.Printf("[DEBUG] resource pool: %#v", resourcePool)

	dcFolders, err := dc.Folders(context.TODO())
	g.Check(err != nil, "get dcFolder error", err)
	if !g.Gret {
		g.GoBack()
		return nil
	}

	// get cluster name for getting host based on template
	cluster := strings.Split(resourcePool.InventoryPath, "/")[3]
	hostObj, err := finder.HostSystem(context.TODO(), dcFolders.HostFolder.InventoryPath+"/"+cluster+"/"+vm.host)
	g.Check(err != nil, "get host object error", err)
	if !g.Gret {
		g.GoBack()
		return nil
	}
	//return nil

	//log.Printf("[DEBUG] folder: %#v", vm.folder)
	folder := dcFolders.VmFolder
	if len(vm.folder) > 0 {
		si := object.NewSearchIndex(c.Client)
		folderRef, err := si.FindByInventoryPath(
			context.TODO(), fmt.Sprintf("%v/vm/%v", vm.datacenter, vm.folder))
		if err != nil {
			//return fmt.Errorf("Error reading folder %s: %s", vm.folder, err)
			fmt.Errorf("Error reading folder %s: %s", vm.folder, err)
		} else if folderRef == nil {
			//return fmt.Errorf("Cannot find folder %s", vm.folder)
			fmt.Errorf("Cannot find folder %s", vm.folder)
		} else {
			folder = folderRef.(*object.Folder)
		}
	}

	var datastore *object.Datastore
	if vm.datastore == "" {
		// do not use the default datastore
		//datastore, err = finder.DefaultDatastore(context.TODO())
		g.Check(vm.datastore == "", " No vm datastore declare, please init vm.datastor", nil)
		if !g.Gret {
			g.GoBack()
			return nil
		}
	} else {
		datastore, err = finder.Datastore(context.TODO(), vm.datastore)
		if err != nil {
			// TODO: datastore cluster support in govmomi finder function
			d, err := getDatastoreObject(c, dcFolders, vm.datastore)
			g.Check(err != nil, "498 : get datastore object error", err)
			if !g.Gret {
				g.GoBack()
				return nil
			}

			if d.Type == "StoragePod" {
				sp := object.StoragePod{
					Folder: object.NewFolder(c.Client, d),
				}
				sps := buildStoragePlacementSpecClone(c, dcFolders, template, resourcePool, sp)

				datastore, err = findDatastore(c, sps)
				g.Check(err != nil, "507 : find datastore error", err)
				if !g.Gret {
					g.GoBack()
					return nil
				}
			} else {
				datastore = object.NewDatastore(c.Client, d)
			}
		}
	}

	//log.Printf("[DEBUG] datastore: %#v", datastore)

	relocateSpec, err := buildVMRelocateSpec(resourcePool, datastore, hostObj, template, vm.hardDisks[0].initType)
	g.Check(err != nil, "517 : buildVMRelocateSpec error", err)
	if !g.Gret {
		g.GoBack()
		return nil
	}

	//log.Printf("[DEBUG] relocate spec: %v", relocateSpec)

	// network
	networkDevices := []types.BaseVirtualDeviceConfigSpec{}
	networkConfigs := []types.CustomizationAdapterMapping{}
	for _, network := range vm.networkInterfaces {
		// network device
		nd, err := buildNetworkDevice(finder, network.label, "vmxnet3")
		g.Check(err != nil, "527 : buildNetworkDevice error", err)
		if !g.Gret {
			g.GoBack()
			return nil
		}
		networkDevices = append(networkDevices, nd)

		// TODO: IPv6 support
		var ipSetting types.CustomizationIPSettings
		if network.ipv4Address == "" {
			ipSetting = types.CustomizationIPSettings{
				Ip: &types.CustomizationDhcpIpGenerator{},
			}
		} else {
			if network.ipv4PrefixLength == 0 {
				//return fmt.Errorf("Error: ipv4_prefix_length argument is empty.")
				fmt.Errorf("Error: ipv4_prefix_length argument is empty.")
			}
			m := net.CIDRMask(network.ipv4PrefixLength, 32)
			sm := net.IPv4(m[0], m[1], m[2], m[3])
			subnetMask := sm.String()
			//log.Printf("[DEBUG] gateway: %v", vm.gateway)
			//log.Printf("[DEBUG] ipv4 address: %v", network.ipv4Address)
			//log.Printf("[DEBUG] ipv4 prefix length: %v", network.ipv4PrefixLength)
			//log.Printf("[DEBUG] ipv4 subnet mask: %v", subnetMask)
			ipSetting = types.CustomizationIPSettings{
				Gateway: []string{
					vm.gateway,
				},
				Ip: &types.CustomizationFixedIp{
					IpAddress: network.ipv4Address,
				},
				SubnetMask: subnetMask,
			}
		}

		// network config
		config := types.CustomizationAdapterMapping{
			Adapter: ipSetting,
		}
		networkConfigs = append(networkConfigs, config)
	}
	//log.Printf("[DEBUG] network configs: %v", networkConfigs[0].Adapter)

	// make config spec
	configSpec := types.VirtualMachineConfigSpec{
		NumCPUs:           vm.vcpu,
		NumCoresPerSocket: 1,
		MemoryMB:          vm.memoryMb,
		DeviceChange:      networkDevices,
	}
	//log.Printf("[DEBUG] virtual machine config spec: %v", configSpec)

	//log.Printf("[DEBUG] starting extra custom config spec: %v", vm.customConfigurations)

	// make ExtraConfig
	if len(vm.customConfigurations) > 0 {
		var ov []types.BaseOptionValue
		for k, v := range vm.customConfigurations {
			key := k
			value := v
			o := types.OptionValue{
				Key:   key,
				Value: &value,
			}
			ov = append(ov, &o)
		}
		configSpec.ExtraConfig = ov
		//log.Printf("[DEBUG] virtual machine Extra Config spec: %v", configSpec.ExtraConfig)
	}

	// create CustomizationSpec
	//customSpec := types.CustomizationSpec{
	//	Identity: &types.CustomizationLinuxPrep{
	//		HostName: &types.CustomizationFixedName{
	//			Name: strings.Split(vm.name, ".")[0],
	//		},
	//		Domain:     vm.domain,
	//		TimeZone:   vm.timeZone,
	//		HwClockUTC: types.NewBool(true),
	//	},
	//	GlobalIPSettings: types.CustomizationGlobalIPSettings{
	//		DnsSuffixList: vm.dnsSuffixes,
	//		DnsServerList: vm.dnsServers,
	//	},
	//	//NicSettingMap: []types.CustomizationAdapterMapping{},
	//	NicSettingMap: networkConfigs,
	//}

	// get the guest info
	guestInfo, err := getVmGuestInfo(c, template.Reference())
	g.Check(err != nil, "615 : getVmGuestInfo error", err)
	if !g.Gret {
		g.GoBack()
		return nil
	}

	// check for windows guest
	windowsGuest, err := regexp.MatchString("windows", guestInfo.GuestId)
	//log.Printf("[DEBUG] guestId: %v", guestInfo.GuestId)

	if windowsGuest {
		log.Printf("[DEBUG] isWindows")
	}

	/*
		if vm.customizationSpecification != nil {

			if v, ok := vm.customizationSpecification["name"]; ok {
				specManager := object.NewCustomizationSpecManager(c.Client)
				specItem, err := specManager.GetCustomizationSpec(context.TODO(), v.(string))
				g.Check(err != nil, "630 : specManager.GetCustomizationSpec error", err)
				customSpec = specItem.Spec
			}

		}
	*/

	//log.Printf("[DEBUG] custom spec: %v", customSpec)

	// make vm clone spec
	cloneSpec := types.VirtualMachineCloneSpec{
		Location: relocateSpec,
		Template: false,
		Config:   &configSpec,
		PowerOn:  false,
	}
	//log.Printf("[DEBUG] clone spec: %v", cloneSpec)

	task, err := template.Clone(context.TODO(), folder, vm.name, cloneSpec)
	g.Check(err != nil, "648 : template clone error", err)
	if !g.Gret {
		g.GoBack()
		return nil
	}

	_, err = task.WaitForResult(context.TODO(), nil)
	g.Check(err != nil, "651 : clone task error", err)
	if !g.Gret {
		g.GoBack()
		return nil
	}

	newVM, err := finder.VirtualMachine(context.TODO(), vm.Path())
	g.Check(err != nil, "657 : find virtual machine error", err)
	if !g.Gret {
		g.GoBack()
		return nil
	}
	//log.Printf("[DEBUG] new vm: %v", newVM)

	devices, err := newVM.Device(context.TODO())
	g.Check(err != nil, "658 : new vm device can not be found", err)
	if !g.Gret {
		g.GoBack()
		return nil
	}

	for _, dvc := range devices {
		// Issue 3559/3560: Delete all ethernet devices to add the correct ones later
		if devices.Type(dvc) == "ethernet" {
			err := newVM.RemoveDevice(context.TODO(), dvc)
			g.Check(err != nil, "664 : new vm remove device error", err)
			if !g.Gret {
				g.GoBack()
				return nil
			}
		}
	}
	// Add Network devices
	for _, dvc := range networkDevices {
		err := newVM.AddDevice(
			context.TODO(), dvc.GetVirtualDeviceConfigSpec().Device)
		g.Check(err != nil, "671 : new vm add device error", err)
		if !g.Gret {
			g.GoBack()
			return nil
		}
	}

	// power on the newVM
	newVM.PowerOn(context.TODO())

	return newVM
}

// getDatacenter gets datacenter object
func getDatacenter(c *govmomi.Client, dc string) (*object.Datacenter, error) {
	finder := find.NewFinder(c.Client, true)
	if dc != "" {
		d, err := finder.Datacenter(context.TODO(), dc)
		return d, err
	} else {
		d, err := finder.DefaultDatacenter(context.TODO())
		return d, err
	}
}

///////////////////////////////////////////////////////////////////////////////////////////////////
// get the vm's path
func (vm *virtualMachine) Path() string {
	return vmPath(vm.folder, vm.name)
}

// get vm's IPAddress
func (vm *virtualMachine) IPAddr() string {
	return vm.networkInterfaces[0].ipv4Address
}

func vmPath(folder string, name string) string {
	var path string
	if len(folder) > 0 {
		path += folder + "/"
	}
	return path + name
}

// create object of vm
func createVMObj() *virtualMachine {
	var oNet networkInterface
	var ohardDisk hardDisk
	var oVM virtualMachine
	var oNetArr []networkInterface
	var oDiskArr []hardDisk

	oNet.ipv4Address = "10.10.10.10"
	///oNet.ipv4PrefixLength = 24
	//oNet.label = ""
	//oNet.deviceName = "vmnic0"
	//oNet.adapterType = "vmxnet3"

	oNetArr = append(oNetArr, oNet)

	// disk : thin or thick
	ohardDisk.initType = "thick"
	oDiskArr = append(oDiskArr, ohardDisk)

	oVM.networkInterfaces = oNetArr
	oVM.hardDisks = oDiskArr

	oVM.template = "6.7_tlp"
	///oVM.gateway = "10.10.10.10"
	///oVM.vcpu = 4
	///oVM.memoryMb = 4096
	oVM.name = "test"
	//oVM.domain = "vmware"
	///oVM.dnsServers = []string{"10.10.10.10"}

	oVM.host = "10.10.221.15"
	oVM.datastore = "datastore15"

	return &oVM
}

// create object of vm
func createVMObjs() []virtualMachine {
	var oVM []virtualMachine
	baRet, err := ioutil.ReadFile("vmlistPath")
	g.Check(err != nil, "Read file error", err)
	if !g.Gret {
		g.GoBack()
		return nil
	}
	vmInfos := strings.Split(string(baRet), "\n")
	for _, info := range vmInfos {
		var oNet networkInterface
		var oNetArr []networkInterface
		var oDiskArr []hardDisk
		var oDisk hardDisk
		items := strings.Split(info, " ")
		g.Check(len(items) != 5, "config file error", nil)
		if !g.Gret {
			g.GoBack()
			return nil
		}
		var vm virtualMachine
		oNet.ipv4Address = items[0]
		oNetArr = append(oNetArr, oNet)
		oDisk.initType = ""
		oDiskArr = append(oDiskArr, oDisk)
		vm.networkInterfaces = oNetArr
		vm.hardDisks = oDiskArr
		vm.name = items[1]
		vm.host = items[2]
		vm.datastore = items[3]
		vm.template = items[4]
		oVM = append(oVM, vm)
	}

	return oVM
}

// clone the vm
func deployVMs(vm *virtualMachine, client *govmomi.Client) *object.VirtualMachine {
	vmClient := vm.deployVirtualMachine(client)
	g.Check(vmClient == nil, "Deploy the vm error", nil)
	if g.Gret == false {
		g.GoBack()
		return nil
	}
	return vmClient
}

// login vm and process something
func (vm *virtualMachine) vmProcess(client *govmomi.Client, vmpath, vmipaddr string) {
	var auth types.NamePasswordAuthentication
	auth.Username = "root"
	auth.Password = "8bio8cwa"

	finder := find.NewFinder(client.Client, true)
	vmInst, err := finder.VirtualMachine(context.TODO(), vmpath)
	g.Check(err != nil, "find vm guest instance error", err)

	o := guest.NewOperationsManager(vmInst.Client(), vmInst.Reference())
	pro, err := o.ProcessManager(context.TODO())
	g.Check(err != nil, "processor manager error", err)

	// check vm power
	msg.Info("Wait for power on")
	powerstate, err := vmInst.PowerState(context.TODO())
	g.Check(err != nil, "get vm guest power status failed", err)
	count := 0
	for {
		if powerstate != "poweredOn" {
			time.Sleep(time.Second)
			powerstate, err = vmInst.PowerState(context.TODO())
			g.Check(err != nil, "get vm guest power status failed", err)
			count += 1
			fmt.Print(". ")
			if count == 10 {
				g.Check(true, "try 10 times, exit...", nil)
				if !g.Gret {
					g.GoBack()
					fmt.Println()
					return
				}
			}
		} else {
			break
		}
	}
	fmt.Println()

	// check vmware tools
	msg.Info("wait, the vmware tools not running...")
	ret, err := vmInst.IsToolsRunning(context.TODO())
	g.Check(err != nil, "get vmwaretool status error", err)
	count = 0
	for {
		if ret != true {
			time.Sleep(time.Second)
			ret, err = vmInst.IsToolsRunning(context.TODO())
			g.Check(err != nil, "get vmwaretool status error", err)
			count += 1
			fmt.Print(". ")
			if count == 30 {
				g.Check(true, "try 30 times, exit...", nil)
				if !g.Gret {
					g.GoBack()
					fmt.Println()
					return
				}
			}
		} else {
			break
		}
	}
	fmt.Println()

	spec := types.GuestProgramSpec{
		ProgramPath:      "/bin/sed",
		Arguments:        "-i 's/10.10.10.10/" + vmipaddr + "/g' /etc/sysconfig/network-scripts/ifcfg-eth0",
		WorkingDirectory: "/",
		EnvVariables:     []string{},
	}
	_, err = pro.StartProgram(context.TODO(), &auth, &spec)
	g.Check(err != nil, "Vm guest start program error", err)
	if !g.Gret {
		g.GoBack()
		return
	}
	err = vmInst.RebootGuest(context.TODO())
	g.Check(err != nil, "Vm guest reboot error", err)
	if !g.Gret {
		g.GoBack()
		return
	}
	_, err = vmInst.WaitForIP(context.TODO())
	g.Check(err != nil, "Wait for vm IP error", err)
	if !g.Gret {
		g.GoBack()
		return
	}
}

// the start of cloning vms
func worker(vmObj *virtualMachine, client *govmomi.Client, ch chan string) {
	succ := vmObj.IPAddr() + " succeed !"
	failed := vmObj.IPAddr() + " failed !"
	// clone a vm using the template
	oVmClient := deployVMs(vmObj, client)
	g.Check(oVmClient == nil, "go vc client error", nil)
	if g.Gret == false {
		g.GoBack()
		ch <- failed
		return
	}

	// change vm config
	vmObj.vmProcess(client, oVmClient.InventoryPath, vmObj.IPAddr())
	if g.Gret == false {
		g.GoBack()
		ch <- failed
		return
	}
	ch <- succ
}

func CloneVM() {
	//declare the channel
	ch := make(chan string, 2)
	// create vm instants from config file
	vmObjs := createVMObjs()
	g.Check(len(vmObjs) == 0, "create vm insts error", nil)
	if !g.Gret {
		g.GoBack()
		msg.Err("create vms error")
	}
	// init auth for vCenter
	var vmAuth Config
	vmAuth.User = "root"
	vmAuth.Password = "VMware"
	vmAuth.VCenterServer = "10.10.221.45"

	// connect to vCenter
	client, err := vmAuth.Client()
	g.Check(err != nil, "Create vcenter client error", err)
	if g.Gret == false {
		g.GoBack()
		return
	}
	// go tasks
	for index, vm := range vmObjs {
		if &vm != nil {
			go worker(&vmObjs[index], client, ch)
		}
	}
	for i := 0; i < len(vmObjs); i++ {
		msg.Info("TASK: " + strconv.Itoa(i) + " --> Clone vm " + <-ch)
	}
}
