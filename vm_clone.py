#!/bin/env python

from pysphere import VIServer
from pysphere import VIException
from pysphere import VIApiException
from socket import error
import json
import os
import logging
import time
import sys

class ESXI():
	def __init__(self, host, name, passwd):
		self.host = host 		# the host's ip
		self.name = name 		# the host's name
		self.passwd = passwd 	# the host's password
		self.esxiInst = None	# the esxi's handler 
		self.VMBuf = []  		# the all vms in the vcenter
		self.datastores = {}	# the host's datastores
		self.hosts = {}			# the host's host 
		self.esxiInst = VIServer() # get init the handles of the host
		self.esxiType = ""		# the type(esxi or vcneter) of the  host connected
		self.VGList = [] 		# the vms you get from the vm's buffer
		self.vm = None			# the handler of the vm you got

	# connect to ESXI
	def esxiConnect(self):
		if self.esxiInst == None:
			logging.fatal("ESXI connect failed...")
			return False
		try:
			if not self.esxiInst.is_connected():
				self.esxiInst.connect(self.host, self.name, self.passwd)
				self.esxiType = self.esxiInst.get_api_type() 
			else:
				logging.fatal("esxi already connected")
				return False
		except VIApiException, e:
			logging.fatal(str(e))
			return False
		except error, e:
			logging.fatal(str(e) + " " + "Host Unreachable...")
			return False
		self.VMBuf = self.esxiInst.get_registered_vms()
		self.datastores = self.esxiInst.get_datastores()
		self.hosts = self.esxiInst.get_hosts()
		if len(self.VMBuf) == 0:
			logging.warning("there is no vms on this esxihost...")
		if len(self.datastores) == 0:
			logging.warning("there is no datastore on this esxihost...")
		if len(self.hosts) == 0:
			logging.warning("there is no host on this esxihost...")
		return True

	# disconnect esxi host
	def esxiDisconnect(self):
		if not self.esxiInst.is_connected():
			logging.fatal("not connecting...")
		else:
			self.esxiInst.disconnect()
	
	# get the vm under the condition
	def esxiGetVmGuest(self, strDstVM):
		for vm in self.VMBuf:
			if vm.find(strDstVM) != -1:
				self.VGList.append(vm)

	# esxi host info
	def esxiStatus(self):
		if self.esxiType == "VirtualCenter":
			print "$host:", 
			hostIter = self.esxiInst.get_hosts().itervalues()
			while True:
				try:
					print "\n\t" + hostIter.next(),
				except StopIteration,e:
					break
			print "\n$datacenter:", 
			datacenterIter = self.esxiInst.get_datacenters().itervalues()
			while True:
				try:
					print "\n\t" + datacenterIter.next(),
				except StopIteration,e:
					break
			print "\n$datastore:", 
			storeIter = self.esxiInst.get_datastores().itervalues()
			while True:
				try:
					print "\n\t" + storeIter.next(),
				except StopIteration,e:
					break
			print "\n$version:\n\t" + self.esxiInst.get_server_type() + " " + self.esxiInst.get_api_version() + " " + self.esxiInst.get_api_type() 
			print "$resourcepool:", 
			resourceIter = self.esxiInst.get_resource_pools().itervalues()
			while True:
				try:
					print "\n\t" + resourceIter.next(),
				except StopIteration,e:
					break
			print
		else:
			print "$host:\t\t" + self.esxiInst.get_hosts().itervalues().next()
			print "$datacenter:\t" + self.esxiInst.get_datacenters().itervalues().next()
			print "$datastore:\t" + self.esxiInst.get_datastores().itervalues().next()
			print "$version:\t" + self.esxiInst.get_server_type() + " " + self.esxiInst.get_api_version() + " " + self.esxiInst.get_api_type() 
			print "$resourcepool:\t" + self.esxiInst.get_resource_pools().itervalues().next()

	# testing the host, name, passwd
	def argvTest(self):
		if self.host=="" or self.name=="" or self.passwd=="":
			return False
		else:
			return True

# interacting hadnle
def InteractVmHanle(esxi):
	strVM = raw_input("the vm :")
	if strVM == "":
		logging.warning("no info for vm...")
		return
	esxi.esxiGetVmGuest(strVM)
	nLen = len(esxi.VGList)
	theVM = []
	if nLen == 0:
		logging.warning("no vm found...")
		return
	elif nLen > 1:
		print '''
the vms you get:'''
		count = 1
		for vm in esxi.VGList:
			print "\t$" + str(count) + " " + vm.split(" ")[1]
			count += 1

		dstVm = raw_input("Enter a num or 'all':")
		if dstVm == 'all':
			theVM = esxi.VGList
		elif int(dstVm)>0 and int(dstVm)<=nLen:
			theVM.append(esxi.VGList[int(dstVm) -1 ])
		else:
			pass
	else:
		theVM = esxi.VGList
		print theVM
	while True:
		nRule = VmMenu(esxi.esxiType)
		cmd = raw_input("Enter a choice:")
		try:
			if cmd == "$":
				break
			elif int(cmd)>nRule or int(cmd)<=0:
				print "-------------------"
				logging.warning("error num...")
				print "-------------------"
			else:
				num = int(cmd)
				for vmItem in theVM:
					vm = esxi.esxiInst.get_vm_by_path(vmItem)
					print vmItem.split(" ")[1]
					if num == 1:
						VmHandle(esxi, vm, "start", [])
					elif num == 2:
						VmHandle(esxi, vm, "stop", [])
					elif num == 3:
						VmHandle(esxi, vm, "status", [])
					elif num == 4:
						VmHandle(esxi, vm, "reboot", [])
					elif num == 5:
						VmHandle(esxi, vm, "runcmd", [])
					elif num == 6:
						VmHandle(esxi, vm, "clone", [])
					elif num == 7:
						VmHandle(esxi, vm, "migrate", [])
					else:
						pass
		except ValueError, e:
			logging.warning(str(e))
			continue

# handle vms from config
def AutoVmHandle(esxi, action, version, name, dstvm, ips, host, datastore):
	# get the vm stored in VGList
	vmPath = ""
	esxi.esxiGetVmGuest(dstvm)
	for item in esxi.VGList:
		vmPath = item
	vmClone = esxi.esxiInst.get_vm_by_path(vmPath)
	if vmClone == None:
		logging.warning("clone vm get error ... ")
		return
	VmHandle(esxi, vmClone, action, [name, ips, host, datastore])

# vm's hanlde
def VmHandle(esxi, vmInst, cmd, messList):
	if cmd=="" or vmInst==None or esxi==None:
		logging.warning("no command for vms")
		return False
	elif cmd == "start":
		pass
		vmInst.power_on(sync_run=True)
		if vmInst.get_status() == "POWERED ON":
			logging.info("the vm started...")
	elif cmd == "stop":
		pass
		vmInst.power_off(sync_run=True)
		if vmInst.get_status() == "POWERED OFF":
			logging.info("the vm stoped...")
	elif cmd == "reboot":
		pass
		vmInst.reboot_guest()
		if vmInst.get_status() == "POWERED ON":
			logging.info("the vm restarted...")
	elif cmd == "status":
		print "\t-------------------------\n\tvm status \t" + str(vmInst.get_status())
		print "\tvm tool \t" + str(vmInst.get_tools_status())
		print "\tresource pool\t" + str(vmInst.get_resource_pool_name())
		print "\t-------------------------\n"
	elif cmd == "runcmd":
		name = raw_input("Enter guest name:")
		passwd = raw_input("Enter guest password:")
		if name!="" and passwd!="":
			try:
				vmInst.login_in_guest(name, passwd)
				cmds = raw_input("Enter command like this[/usr/bin/wget http://xxx/*], using space to sperate:")
				cmds = cmds.split(" ")
				cc = cmds[0]
				cmds.remove(cmds[0])
				print vmInst.start_process(cc, cmds)
			except VIApiException, e:
				logging.fatal(str(e))
				return
		pass
	elif cmd == "clone":
		if len(messList) != 4:
			logging.warning("clone argvs error...")
			return
		tmpdatastore = ""
		tmphost = ""
		for key in esxi.datastores:
			if esxi.datastores[key] == messList[3]:
				tmpdatastore = key
		for key in esxi.hosts:
			if esxi.hosts[key] == messList[2]:
				tmphost = key
		# clone the vm guest
		print tmphost, tmpdatastore
                print messList[0]
		#vmc = vmInst.clone(messList[0], datastore=tmpdatastore, host=tmphost)
		vmc = vmInst.clone(messList[0], sync_run=True, datastore=tmpdatastore, host=tmphost, power_on=True)
		if vmc == None:
			logging.fatal("clone vm inst None...")
			return
		//if vmc.is_powered_on():
		if false:
			logging.info("vm init ...")
			while True:
				try:
					vmc.login_in_guest("root", "8bio8cwa")
					# print vmc.start_process("/usr/bin/wget", ["http://10.103.11.101/www/wanglei/share/changeIP.sh"])
					# print vmc.start_process("/bin/sh", ["changeIP.sh", messList[1]])
					# print vmc.start_process("/usr/sbin/reboot")
					print "-------------------------"
					print messList[1]
					print vmc.start_process("/bin/echo", ["", ">", "/etc/sysconfig/network-scripts/ifcfg-eth0"])
					print vmc.start_process("/bin/echo", ["DEVICE=eth0", ">", "/etc/sysconfig/network-scripts/ifcfg-eth0"])
					print vmc.start_process("/bin/echo", ["BOOTPROTO=static", ">>", "/etc/sysconfig/network-scripts/ifcfg-eth0"])
					print vmc.start_process("/bin/echo", ["IPADDR="+messList[1], ">>", "/etc/sysconfig/network-scripts/ifcfg-eth0"])
					print vmc.start_process("/bin/echo", ["NETMASK=255.255.255.0", ">>", "/etc/sysconfig/network-scripts/ifcfg-eth0"])
					print vmc.start_process("/bin/echo", ["ONBOOT=yes", ">>", "/etc/sysconfig/network-scripts/ifcfg-eth0"])
					print vmc.start_process("/bin/echo", ["TYPE=Ethernet", ">>", "/etc/sysconfig/network-scripts/ifcfg-eth0"])
					print vmc.start_process("/sbin/reboot")
					logging.info("create vm " + messList[0] + "  sucessfully....")
					print "--------------------------"
					break
				except VIApiException, e:
					time.sleep(1)
                                        print e
					continue
		elif cmd == "rename":
			pass
		else:
			logging.fatal("create vm " + messList[0] + " failed...")
			return
	elif cmd == "migrate":
		pass
	else:
		pass

# vm's menu
def VmMenu(vType):
	esxiMenu = '''
	1. start	vm
	2. stop 	vm
	3. status	vm
	4. reboot 	vm
	5. run a 	command'''
	vcMenu = '''
	6. clone	vm
        :	7. migrate	vm'''
	endMenu = '''
	$. quit
	'''
	if vType == "VirtualCenter":
		print esxiMenu + vcMenu + endMenu
		return 7
	elif vType == "HostAgent":
		print esxiMenu + endMenu
		return 5
	else:
		print "Unknow host..."

def main():
	#esxi = ESXI("vcenter", "user", "passwd")
	esxi = ESXI("10.10.221.44", "root", "pass@word1")
	if esxi.esxiConnect():
		argvLen = len(sys.argv)
		if argvLen == 1:
			InteractVmHanle(esxi)
		else:
			if os.path.exists("vm_config"):
				jsonobj = json.load(file("vm_config"))
				for key in jsonobj:
					if key == "clone":
						cloneDict = jsonobj[key]
						for key in cloneDict:
							key = str(key)
							if key == "version":
								version = str(cloneDict[key])
							elif key == "cloned_vm":
								clonedvm = str(cloneDict[key])
							elif key == "host":
								host = str(cloneDict[key])
							elif key == "datastore":
								datastore = str(cloneDict[key])
							else:
								vmToBeCloneList = cloneDict[key].split(",")
						# clone vm from list
						for vmhost in vmToBeCloneList:
							vmhost = str(vmhost)
                                                        print vmhost
							AutoVmHandle(esxi, "clone", version, "centos6.7(" + vmhost + ")_huiyuan", clonedvm, vmhost, host, datastore)
					if key == "rename":
						AutoVmHandle(esxi, "rename")
			esxi.esxiDisconnect()
	else:
		logging.fatal("esxi connect failed")

if __name__ == '__main__':
	main()
