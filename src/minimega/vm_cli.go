package main

import "minicli"

const (
	vmInfoHelpShort = "print information about VMs"
	vmInfoHelpLong  = `
Print information about VMs. vm_info allows searching for VMs based on any VM
parameter, and output some or all information about the VMs in question.
Additionally, you can display information about all running VMs.

A vm_info command takes three optional arguments, an output mode, a search
term, and an output mask. If the search term is omitted, information about all
VMs will be displayed. If the output mask is omitted, all information about the
VMs will be displayed.

The output mode has two options - quiet and json. Two use either, set the output using the following syntax:

	vm_info output=quiet ...

If the output mode is set to 'quiet', the header and "|" characters in the output formatting will be removed. The output will consist simply of tab delimited lines of VM info based on the search and mask terms.

If the output mode is set to 'json', the output will be a json formatted string containing info on all VMs, or those matched by the search term. The mask will be ignored - all fields will be populated.

The search term uses a single key=value argument. For example, if you want all
information about VM 50:

	vm_info id=50

The output mask uses an ordered list of fields inside [] brackets. For example,
if you want the ID and IPs for all VMs on vlan 100:

	vm_info vlan=100 [id,ip]

Searchable and maskable fields are:

- host	  : the host that the VM is running on
- id	  : the VM ID, as an integer
- name	  : the VM name, if it exists
- memory  : allocated memory, in megabytes
- vcpus   : the number of allocated CPUs
- disk    : disk image
- initrd  : initrd image
- kernel  : kernel image
- cdrom   : cdrom image
- state   : one of (building, running, paused, quit, error)
- tap	  : tap name
- mac	  : mac address
- ip	  : IPv4 address
- ip6	  : IPv6 address
- vlan	  : vlan, as an integer
- bridge  : bridge name
- append  : kernel command line string

Examples:

Display a list of all IPs for all VMs:
	vm_info [ip,ip6]

Display all information about VMs with the disk image foo.qc2:
	vm_info disk=foo.qc2

Display all information about all VMs:
	vm_info`

	vmSaveHelpShort = "save a vm configuration for later use"
	vmSaveHelpLong  = `
Saves the configuration of a running virtual machine or set of virtual
machines so that it/they can be restarted/recovered later, such as after
a system crash.

If no VM name or ID is given, all VMs (including those in the quit and error state) will be saved.

This command does not store the state of the virtual machine itself,
only its launch configuration.`

	vmLaunchHelpShort = "launch virtual machines in a paused state"
	vmLaunchHelpLong  = `
Launch virtual machines in a paused state, using the parameters defined
leading up to the launch command. Any changes to the VM parameters after
launching will have no effect on launched VMs.

If you supply a name instead of a number of VMs, one VM with that name
will be launched.

The optional 'noblock' suffix forces minimega to return control of the
command line immediately instead of waiting on potential errors from
launching the VM(s). The user must check logs or error states from
vm_info.`

	vmKillHelpShort = "kill running virtual machines"
	vmKillHelpLong  = `
Kill a virtual machine by ID or name. Pass -1 to kill all virtual machines.`

	vmStartHelpShort = "start paused virtual machines"
	vmStartHelpLong  = `
Start all or one paused virtual machine. To start all paused virtual machines,
call start without the optional VM ID or name.

Calling vm_start specifically on a quit VM will restart the VM. If the
'quit=true' argument is passed when using vm_start with no specific VM, all VMs
in the quit state will also be restarted.`

	vmStopHelpShort = "stop/pause virtual machines"
	vmStopHelpLong  = `
Stop all or one running virtual machine. To stop all running virtual machines,
call stop without the optional VM ID or name.

Calling stop will put VMs in a paused state. Start stopped VMs with vm_start.`
)

func init() {
	handler := &minicli.Handler{
		Pattern:   "vm info",
		HelpShort: vmInfoHelpShort,
		HelpLong:  vmInfoHelpLong,
		Call:      nil} // TODO

	minicli.Register(handler)
	handler.Pattern = "vm info search <terms>"
	minicli.Register(handler)
	handler.Pattern = "vm info search <terms> mask <masks>"
	minicli.Register(handler)
	handler.Pattern = "vm info mask <masks>"
	minicli.Register(handler)

	minicli.Register(&minicli.Handler{
		Pattern:   "vm kill <vm id or name or *>",
		HelpShort: vmKillHelpShort,
		HelpLong:  vmKillHelpLong})
	minicli.Register(&minicli.Handler{
		Pattern:   "vm start <vm id or name or *>",
		HelpShort: vmStartHelpShort,
		HelpLong:  vmStartHelpLong})
	minicli.Register(&minicli.Handler{
		Pattern:   "vm stop <vm id or name or *>",
		HelpShort: vmStopHelpShort,
		HelpLong:  vmStopHelpLong})
}
