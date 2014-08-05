'''
Copyright (2014) Sandia Corporation.
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
the U.S. Government retains certain rights in this software.

Devin Cook <devcook@sandia.gov>

Minimega bindings for Python

This API uses a Unix domain socket to communicate with a running instance of
minimega. The protocol is documented here:
https://code.google.com/p/minimega/wiki/UserGuide#Command_Port_and_the_Local_Command_Flag

The commands are taken straight out of the cliCommands map in the file
src/minimega/cli.go.
'''

import socket
import json
import re

from os import path


__version__ = 'minimega.py 9d5cd411baaaef9abefdfdbe39bada5454ee382d 2014-05-12'


class Error(Exception): pass


NET_RE = re.compile(r'((?P<bridge>\w+),)?(?P<id>\d+)(,(?P<mac>([0-9A-Fa-f]:?){6}))?')
FILE_RE = re.compile(r'^(?:(?P<dir><dir> )|\s+)(?P<name>.*?)\s+(?P<size>\d+)$')
DEFAULT_TIMEOUT = 60
MSG_BLOCK_SIZE = 4096


def connect(path):
    '''
    Connect to the minimega instance with UNIX socket at <path> and return
    a new minimega API object.
    '''
    return minimega(path)


class FileStore(dict):
    '''
    This is an internal class that is used to provide access to the minimega
    file API. It should not need to be invoked manually.

    This class behaves like a dictionary of filename -> size mappings. These
    files can be deleted using del, new files can be downloaded using get(),
    and the status of transferring files can be obtained with status().
    '''

    def __init__(self, mm, cwd='/'):
        super(FileStore, self).__init__()
        self._mm = mm
        self._cwd = cwd

        self.list()

    def __setitem__(self, key, value):
        # TODO(devin): this could actually be useful, store a file from python?
        raise NotImplementedError

    def update(self, D):
        raise NotImplementedError

    def __delitem__(self, key):
        if key not in self:
            raise KeyError
        self._mm._send('file', 'delete', path.join(self._cwd, key))
        super(FileStore, self).__delitem__(key)

    def status(self):
        return self._mm._send('file', 'status')

    def get(self, key):
        self._mm._send('file', 'get', path.join(self._cwd, key))

    def list(self):
        '''
        This function updates the internal file listing from minimega and
        returns a list of files.
        '''
        self.clear()
        setitem = super(FileStore, self).__setitem__

        for file in self._mm._send('file', 'list', self._cwd).splitlines():
            m = FILE_RE.match(file)
            if not m:
                raise Error('Failure parsing file listing in ' + self._cwd)
            name = m.group('name')
            if m.group('dir'):
                #recurse
                setitem(name, FileStore(self._mm, path.join(self._cwd, name)))
            else:
                setitem(name, int(m.group('size')))

        return self.keys()


class minimega(object):
    '''
    This class communicates with a running instance of minimega using a Unix
    domain socket. The protocol is specified here:
    https://code.google.com/p/minimega/wiki/UserGuide#Command_Port_and_the_Local_Command_Flag

    Each minimega command can be called from this object, and the response will
    be returned unless an Exception is thrown.

    Attributes:

    files - minimega "file" API
            This object behaves like a dictionary of filename -> size mappings.
            Note that getting a file via get() does not make the file
            immediately accessible. After it has completed downloading, you need
            to call list() to refresh the file list.

            To list the files available:
                >>> mm.files
                {'dir1': {'file3': 1024}, 'file1': 42, 'file2': 1337}
                >>> mm.files.list()
                ['dir1', 'file1', 'file2']

            To delete a file or recursively delete a directory:
                >>> del mm.files['file1']
                >>> del mm.files['dir1']

            To download a new file from the mesh:
                >>> mm.files.get('newfile')

            To get the status of transferring files:
                >>> mm.files.status()
    '''

    def __init__(self, path, timeout=None):
        '''Connects to the minimega instance with Unix socket at <path>.'''
        self._debug = False
        self._path = path
        self._timeout = timeout
        self._socket = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self._socket.settimeout(timeout if timeout != None else DEFAULT_TIMEOUT)
        self._socket.connect(path)
        self.files = FileStore(self)

    def _reconnect(self):
        try:
            self._socket.close()
        except:
            pass

        self.__init__(self._path, self._timeout)

    def _send(self, cmd, *args):
        msg = json.dumps({'Command': cmd, 'Args': args},
                         separators=(',', ':'))
        if len(msg) != self._socket.send(msg.encode('utf-8')):
            raise Error('failed to write message to minimega')

        msg = ''
        more = self._socket.recv(MSG_BLOCK_SIZE).decode('utf-8')
        response = None
        while response is None and more:
            msg += more
            try:
                response = json.loads(msg)
            except ValueError as e:
                if self._debug:
                    print(e)
                more = self._socket.recv(MSG_BLOCK_SIZE).decode('utf-8')

        if not msg:
            raise Error('Expected response, socket closed')

        if response['Error']:
            raise Error(response['Error'])

        return response['Response']

    def rate(self, ms=None):
        '''
        set the launch/kill rate in milliseconds

        Set the launch and kill rate in milliseconds. Some calls to external
        tools can take some time to respond, causing errors if you try to
        launch or kill VMs too quickly. The default value is 100 milliseconds.

        If called with no argument, it returns the current rate (e.g. "100ms").
        '''
        if ms is None:
            return self._send('rate')

        if not isinstance(ms, int):
            raise TypeError('rate must be specified in milliseconds')

        return self._send('rate', str(ms))

    def log_level(self, level=None):
        '''
        set the log level

        Set the log level to one of [debug, info, warn, error, fatal]. Log
        levels inherit lower levels, so setting the level to error will also
        log fatal, and setting the mode to debug will log everything.

        If called with no argument, it returns the current log level.
        '''
        options = ('debug', 'info', 'warn', 'error', 'fatal')

        if level is None:
            return self._send('log_level')

        if not isinstance(level, str):
            raise TypeError('level must be specified as one of ' + str(options))

        level = level.lower()
        if level not in options:
            raise ValueError('level must be specified as one of ' +
                             str(options))

        return self._send('log_level', level)

    def log_stderr(self, enabled=None):
        '''
        enable/disable logging to stderr

        Enable or disable logging to stderr. Valid options are [true, false].

        If called with no argument, it returns whether or not logging to stderr
        is enabled.
        '''
        if enabled is None:
            return bool(self._send('log_stderr'))

        if not isinstance(enabled, bool):
            raise TypeError('enabled must be a bool')

        return self._send('log_stderr', str(enabled).lower())

    def log_file(self, filename=None):
        '''
        enable logging to a file

        Log to a file. To disable file logging, call "clear log_file".

        If called with no argument, it returns whether or not logging to a file
        is enabled.
        '''
        if filename is None:
            return bool(self._send('log_file'))

        if not isinstance(filename, str):
            raise TypeError('filename must be a string')

        return self._send('log_file', filename)

    def check(self):
        '''
        check for the presence of all external executables minimega uses

        Minimega maintains a list of external packages that it depends on, such
        as qemu. Calling check will attempt to find each of these executables
        in the avaiable path, and returns an error on the first one not found.
        '''
        return self._send('check')

    def nuke(self):
        '''
        attempt to clean up after a crash

        After a crash, the VM state on the machine can be difficult to recover
        from. Nuke attempts to kill all instances of QEMU, remove all taps and
        bridges, and removes the temporary minimega state on the harddisk.
        '''
        return self._send('nuke')

    def write(self, filename):
        '''
        write the command history to a file

        Write the command history to file. This is useful for handcrafting
        configs on the minimega command line and then saving them for later
        use. Args that failed, as well as some commands that do not impact the
        VM state, such as 'help', do not get recorded.
        '''
        if not isinstance(filename, str):
            raise TypeError('filename must be a string')

        return self._send('write', filename)

    def vm_save(self, name, *idsOrNames):
        '''
        save a vm configuration for later use

        Saves the configuration of a running virtual machine or set of virtual
        machines so that it/they can be restarted/recovered later, such as
        after a system crash.

        If no VM name or ID is given, all VMs (including those in the quit and
        error state) will be saved.

        This command does not store the state of the virtual machine itself,
        only its launch configuration.
        '''
        if not isinstance(name, str):
            raise TypeError('name must be a string')

        for id in idsOrNames:
            if not (isinstance(id, int) or isinstance(id, str)):
                raise TypeError('id must be an integer or string')

        return self._send('vm_save', name, *map(str, idsOrNames))

    def read(self, filename):
        '''
        read and execute a command file

        Read a command file and execute it. This has the same behavior as if
        you typed the file in manually.
        '''
        if not isinstance(filename, str):
            raise TypeError('filename must be a string')

        return self._send('read', filename)

    def vm_info(self, output=None, search=None, mask=None):
        '''print information about VMs


        Print information about VMs. vm_info allows searching for VMs based on
        any VM parameter, and output some or all information about the VMs in
        question. Additionally, you can display information about all running
        VMs.

        A vm_info command takes three optional arguments, an output mode, a
        search term, and an output mask. If the search term is omitted,
        information about all VMs will be displayed. If the output mask is
        omitted, all information about the VMs will be displayed.

        The output mode has two options - quiet and json. Two use either, set
        the output using the following syntax:
            vm_info output=quiet ...

        If the output mode is set to 'quiet', the header and "|" characters in
        the output formatting will be removed. The output will consist simply
        of tab delimited lines of VM info based on the search and mask terms.

        If the output mode is set to 'json', the output will be a json
        formatted string containing info on all VMs, or those matched by the
        search term. The mask will be ignored - all fields will be populated.

        The search term uses a single key=value argument. For example, if you
        want all information about VM 50:
            vm_info id=50

        The output mask uses an ordered list of fields inside [] brackets. For
        example, if you want the ID and IPs for all VMs on vlan 100:
            vm_info vlan=100 [id,ip]

        Searchable and maskable fields are:
            host    : the host that the VM is running on
            id      : the VM ID, as an integer
            name    : the VM name, if it exists
            memory  : allocated memory, in megabytes
            vcpus   : the number of allocated CPUs
            disk    : disk image
            initrd  : initrd image
            kernel  : kernel image
            cdrom   : cdrom image
            state   : one of (building, running, paused, quit, error)
            tap     : tap name
            mac     : mac address
            ip      : IPv4 address
            ip6     : IPv6 address
            vlan    : vlan, as an integer
            bridge  : bridge name
            append  : kernel command line string

        Examples:
        Display a list of all IPs for all VMs:
            vm_info [ip,ip6]

        Display all information about VMs with the disk image foo.qc2:
            vm_info disk=foo.qc2

        Display all information about all VMs:
            vm_info
        '''
        args = []

        if output != None:
            if not isinstance(output, str):
                raise TypeError('output argument must be a string')
            if not output.startswith('output='):
                raise ValueError(
                    'output must be specified as "output=[json|quiet]"'
                )
            args.append(output)

        if search != None:
            if not isinstance(search, str):
                raise TypeError('search must be a string')
            if len(output.split('=')) != 2:
                raise ValueError('search must be a single key=value pair')
            args.append(search)

        if mask != None:
            if not (mask is None or isinstance(mask, str)):
                raise TypeError('mask must be a string')
            args.append(mask)

        return self._send('vm_info', *args)

    def quit(self, delay=None):
        '''
        Quit. An optional integer argument X allows deferring the quit call for
        X seconds. This is useful for telling a mesh of minimega nodes to quit.
        '''
        if not delay:
            return self._send('quit')

        if not isinstance(delay, int):
            raise TypeError('delay must be an integer')

        return self._send('quit', str(delay))

    def exit(self, delay=None):
        '''An alias to 'quit'.'''
        return self.quit(delay)

    def vm_launch(self, numOrName):
        '''
        launch virtual machines in a paused state

        Launch <numOrName> virtual machines in a paused state, using the
        parameters defined leading up to the launch command. Any changes to the
        VM parameters after launching will have no effect on launched VMs.

        If you supply a name instead of a number of VMs, one VM with that name
        will be launched.
        '''
        if not (isinstance(numOrName, str) or isinstance(numOrName, int)):
            raise TypeError('numOrName must be a string or an int')
        
        return self._send('vm_launch', str(numOrName))

    def vm_kill(self, idOrName):
        '''
        kill running virtual machines",

        Kill a virtual machine by ID or name. Pass -1 to kill all virtual
        machines.
        '''
        if not (isinstance(idOrName, str) or isinstance(idOrName, int)):
            raise TypeError('idOrName must be a string or an int')

        return self._send('vm_kill', str(idOrName))

    def vm_start(self, idOrName=None):
        '''
        start paused virtual machines

        Start all or one paused virtual machine. To start all paused virtual
        machines, call start without the optional VM ID or name.

        Calling vm_start on a quit VM will restart the VM.
        '''
        if idOrName is None:
            return self._send('vm_start')

        if not (isinstance(idOrName, str) or isinstance(idOrName, int)):
            raise TypeError('idOrName must be a string or an int')

        return self._send('vm_start', str(idOrName))

    def vm_stop(self, idOrName=None):
        '''
        stop/pause virtual machines

        Stop all or one running virtual machine. To stop all running virtual
        machines, call stop without the optional VM ID or name.

        Calling stop will put VMs in a paused state. Start stopped VMs with
        vm_start
        '''
        if idOrName is None:
            return self._send('vm_stop')

        if not (isinstance(idOrName, str) or isinstance(idOrName, int)):
            raise TypeError('idOrName must be a string or an int')

        return self._send('vm_stop', str(idOrName))

    def vm_qemu(self, qemu=None):
        '''
        set the qemu process to invoke

        Set the qemu process to invoke. Relative paths are ok.

        Call vm_qemu with no arguments to get the current qemu process.
        '''
        if qemu is None:
            return self._send('vm_qemu')

        if not isinstance(qemu, str):
            raise TypeError('qemu must be a string')

        return self._send('vm_qemu', qemu)

    def vm_memory(self, mem=None):
        '''
        set the amount of physical memory for a VM

        Set the amount of physical memory to allocate in megabytes.

        Call vm_memory with no arguments to get the current memory.
        '''
        if mem is None:
            return int(self._send('vm_memory'))

        if not isinstance(mem, int):
            raise TypeError('mem must be an integer value in MB')

        return self._send('vm_memory', str(mem))

    def vm_vcpus(self, num=None):
        '''
        set the number of virtual CPUs for a VM

        Set the number of virtual CPUs to allocate a VM.

        Call vm_vcpus with no arguments to get the current number of CPUs.
        '''
        if num is None:
            return int(self._send('vm_vcpus'))

        if not isinstance(num, int):
            raise TypeError('num must be an int')

        return self._send('vm_vcpus', str(num))

    def vm_disk(self, disk=None):
        '''
        set a disk image to attach to a VM

        Attach a disk to a VM. Any disk image supported by QEMU is a valid
        parameter. Disk images launched in snapshot mode may safely be used for
        multiple VMs.

        Call vm_disk with no arguments to get the current disk.
        '''
        if disk is None:
            return self._send('vm_disk')

        if not isinstance(disk, str):
            raise TypeError('disk must be a string')

        return self._send('vm_disk', disk)

    def vm_cdrom(self, cdrom=None):
        '''
        set a cdrom image to attach to a VM

        Attach a cdrom to a VM. When using a cdrom, it will automatically be
        set to be the boot device.

        Call vm_cdrom with no arguments to get the current cdrom image.
        '''
        if cdrom is None:
            return self._send('vm_cdrom')

        if not isinstance(cdrom, str):
            raise TypeError('cdrom must be a string')

        return self._send('vm_cdrom', cdrom)

    def vm_kernel(self, kernel=None):
        '''
        set a kernel image to attach to a VM

        Attach a kernel image to a VM. If set, QEMU will boot from this image
        instead of any disk image.

        Call vm_kernel with no arguments to get the current kernel.
        '''
        if kernel is None:
            return self._send('vm_kernel')

        if not isinstance(kernel, str):
            raise TypeError('kernel must be a string')

        return self._send('vm_kernel', kernel)

    def vm_initrd(self, initrd=None):
        '''
        set a initrd image to attach to a VM

        Attach an initrd image to a VM. Passed along with the kernel image at
        boot time.

        Call vm_initrd with no arguments to get the current initrd.
        '''
        if initrd is None:
            return self._send('vm_initrd')

        if not isinstance(initrd, str):
            raise TypeError('initrd must be a string')

        return self._send('vm_initrd', initrd)

    def vm_qemu_append(self, args=None):
        '''
        add additional arguments for the QEMU command

        Add additional arguments to be passed to the QEMU instance. For
        example, mm.vm_qemu_append(["-serial", "tcp:localhost:4001"]).

        Call vm_qemu_append with no arguments to get the current extra qemu
        arguments.
        '''
        if not args:
            return self._send('vm_qemu_append').split()

        if not isinstance(args, list):
            raise TypeError('args must be a list of arguments')

        for arg in args:
            if not isinstance(arg, str):
                raise TypeError('all args must be strings')

        return self._send('vm_qemu_append', *args)

    def vm_append(self, config=None):
        '''
        set an append string to pass to a kernel set with vm_kernel

        Add an append string to a kernel set with vm_kernel. Setting vm_append
        without using vm_kernel will result in an error.

        For example, to set a static IP for a linux VM:
            mm.vm_append("ip=10.0.0.5 gateway=10.0.0.1 netmask=255.255.255.0 dns=10.10.10.10")

        Call vm_append with no arguments to get the current extra vm arguments.
        '''
        if config is None:
            return self._send('vm_append')

        if not isinstance(config, str):
            raise TypeError('config must be a string')

        return self._send('vm_append', config)

    def vm_net(self, *networks):
        '''
        specify the networks the VM is a member of

        Specify the network(s) that the VM is a member of by id. A
        corresponding VLAN will be created for each network. Optionally, you
        may specify the bridge the interface will be connected on. If the
        bridge name is omitted, minimega will use the default 'mega_bridge'.
        You can also optionally specify the mac address of the interface to
        connect to that network. If not specifed, the mac address will be
        randomly generated. Additionally, you can also specify a driver for
        qemu to use. By default, e1000 is used.

        Examples:

        To connect a VM to VLANs 1 and 5:
            mm.vm_net('1', '5')
        To connect a VM to VLANs 100, 101, and 102 with specific mac addresses:
            mm.vm_net('100,00:00:00:00:00:00', '101,00:00:00:00:01:00', '102,00:00:00:00:02:00')
        To connect a VM to VLAN 1 on bridge0 and VLAN 2 on bridge1:
            mm.vm_net('bridge0,1', 'bridge1,2')
        To connect a VM to VLAN 100 on bridge0 with a specific mac:
            mm.vm_net('bridge0,100,00:11:22:33:44:55')
        To specify a specific driver, such as i82559c:
            mm.vm_net('100,i82559c')

        Calling vm_net with no parameters will list the current networks for
        this VM.
        '''
        if not networks:
            return self._send('vm_net')

        for net in networks:
            if not isinstance(net, str):
                raise TypeError()
            if not NET_RE.match(net):
                raise ValueError('incorrect network specification: ' + net)

        return self._send('vm_net', *networks)

    def web(self, port=8080, novnc=None):
        '''
        start the minimega web interface

        Launch a webserver that allows you to browse the connected minimega
        hosts and VMs, and connect to any VM in the pool.

        This command requires access to an installation of novnc. By default
        minimega looks in 'pwd'/misc/novnc. To set a different path, invoke:

            web novnc <path to novnc>

        To start the webserver on a specific port, issue the web command with
        the port:
            web 7000

        8080 is the default port.

        Calling mm.web(port=None, novnc=None) will return the current path to
        novnc.
        '''
        if novnc != None:
            if not isinstance(novnc, str):
                raise TypeError('novnc must be a string')
            #setting novnc doesn't return anything
            self._send('web', 'novnc', novnc)

        if port != None and not isinstance(port, int):
            raise TypeError('port must be an int')

        if port is None:
            return self._send('web', 'novnc')

        return self._send('web', str(port))

    def history(self):
        '''shows the command history'''
        return self._send('history').splitlines()

    def clear(self, var):
        '''
        restore a variable to its default state

        Restores a variable to its default state or clears it. For example,
        mm.clear('net') will clear the list of associated networks.
        '''
        if not isinstance(var, str):
            raise TypeError('var must be a string')

        return self._send('clear', var)

    def host_tap(self, cmd=None, *args):
        '''
        control host taps for communicating between hosts and VMs

        Control host taps on a named vlan for communicating between a host and
        any VMs on that vlan.

        Calling host_tap with no cmd argument will list all created host_taps.

        To create a host_tap on a particular vlan, invoke host_tap with the
        create command:

            host_tap create <vlan> <ip/dhcp>

        For example, to create a host tap with ip and netmask 10.0.0.1/24 on
        VLAN 5:

            host_tap create 5 10.0.0.1/24

        Optionally, you can specify the bridge to create the host tap on:

            host_tap create <bridge> <vlan> <ip/dhcp>

        Additionally, you can bring the tap up with DHCP by using "dhcp"
        instead of a ip/netmask:

            host_tap create 5 dhcp

        To delete a host tap, use the delete command and tap name from the
        host_tap list:

            host_tap delete <id>
	
        To delete all host taps, use mm.clear('host_tap') or id = -1:

            host_tap delete -1
        '''
        options = ('create', 'delete')

        if cmd is None:
            return self._send('host_tap')

        if not isinstance(cmd, str):
            raise TypeError('cmd must be a string')

        if cmd not in options:
            raise ValueError('cmd must be one of: ' + str(options))

        return self._send('host_tap', cmd, *map(str, args))

    def mesh_degree(self, degree=None):
        '''view or set the current degree for this mesh node'''
        if degree is None:
            return int(self._send('mesh_degree'))

        if not isinstance(degree, int):
            raise TypeError('degree must be an int')

        return self._send('mesh_degree', str(degree))

    def mesh_dial(self, addr):
        '''Attempt to connect to another listening node.'''
        if not isinstance(addr, str):
            raise TypeError('addr must be a string')

        return self._send('mesh_dial', addr)

    def mesh_dot(self, filename):
        '''output a graphviz formatted dot file'''
        if not isinstance(filename, str):
            raise TypeError('filename must be a string')

        return self._send('mesh_dot', filename)

    def mesh_status(self):
        '''display a short status report of the mesh'''
        return self._send('mesh_status')

    def mesh_list(self):
        '''display the mesh adjacency list'''
        return self._send('mesh_list')

    def mesh_hangup(self, host):
        '''disconnect from a client'''
        if not isinstance(host, str):
            raise TypeError('host must be a string')

        return self._send('mesh_hangup', host)

    def mesh_msa_timeout(self, timeout=None):
        '''View or set the Meshage State Announcement timeout'''
        if timeout is None:
            return int(self._send('mesh_msa_timeout'))

        if not isinstance(timeout, int):
            raise TypeError('timeout must be an int')

        return self._send('mesh_msa_timeout', str(timeout))

    def mesh_timeout(self, timeout=None):
        '''
        view or set the mesh timeout

        View or set the timeout on sending mesh commands.

        When a mesh command is issued, if a response isn't sent within
        mesh_timeout seconds, the command will be dropped and any future
        response will be discarded. Note that this does not cancel the
        outstanding command - the node receiving the command may still
        complete, but rather this node will stop waiting on a response.
        '''
        if timeout is None:
            return self._send('mesh_timeout')

        if not isinstance(timeout, int):
            raise TypeError('timeout must be an int')

        return self._send('mesh_timeout', str(timeout))

    def mesh_set(self, nodes, cmd, annotate=False):
        '''
        send a command to one or more connected clients

        For example, to get the vm_info from nodes kn1 and kn2:
            mm.mesh_set('kn[1-2]', 'vm_info')

        Optionally, you can annotate the output with the hostname of all
        responders by setting annotate=True.
        '''
        # TODO(devin): there's probably a way to avoid making the user pass cmd
        #              in as a string
        # BUG(devin): this is a hack. cmd is really supposed to be a list
        if not (isinstance(nodes, str) and isinstance(cmd, str)):
            raise TypeError('nodes and cmd must both be strings')

        if annotate:
            return self._send('mesh_set', 'annotate', nodes, cmd)

        return self._send('mesh_set', nodes, cmd)

    def mesh_broadcast(self, cmd, annotate=False):
        '''
        Send a command to all connected clients.
        For example, to get the vm_info from all nodes:
            mm.mesh_broadcast('vm_info')

        Optionally, you can annotate the output with the hostname of all
        responders by setting annotate=True.
        '''
        # TODO(devin): there's probably a way to avoid making the user pass cmd
        #              in as a string
        # BUG(devin): this is a hack. cmd is really supposed to be a list
        if not isinstance(cmd, str):
            raise TypeError('cmd must be a string')

        if annotate:
            return self._send('mesh_broadcast', 'annotate', cmd)

        return self._send('mesh_broadcast', cmd)

    def hostname(self):
        '''return the hostname'''
        return self._send('hostname')

    def dnsmasq(self, cmd=None, *args):
        '''
        start a dhcp/dns server on a specified ip

        Start a dhcp/dns server on a specified IP with a specified range. For
        example, to start a DHCP server on IP 10.0.0.1 serving the range
        10.0.0.2 - 10.0.254.254:

            mm.dnsmasq('start', '10.0.0.1', '10.0.0.2', '10.0.254.254')

        To start only a from a config file:

            mm.dnsmasq('start', '/path/to/config')

        To list running dnsmasq servers, invoke dnsmasq with no arguments. To
        kill a running dnsmasq server, specify its ID from the list of running
        servers. For example, to kill dnsmasq server 2:

            mm.dnsmasq('kill', 2)

        To kill all running dnsmasq servers, pass -1 as the ID:

            mm.dnsmasq('kill', -1)

        dnsmasq will provide DNS service from the host, as well as from
        /etc/hosts. You can specify an additional config file for dnsmasq by
        providing a file as an additional argument.

            mm.dnsmasq('start', '10.0.0.1', '10.0.0.2', '10.0.254.254',
                       '/tmp/dnsmasq-extra.conf')

        NOTE: If specifying an additional config file, you must provide the
        full path to the file.

        Calling mm.dnsmasq() with no arguments will return the list of all
        dnsmasq servers.
        '''
        options = ('start', 'kill')

        if cmd is None:
            return self._send('dnsmasq')

        if not isinstance(cmd, str):
            raise TypeError('cmd must be a string')

        if cmd not in options:
            raise ValueError('cmd must be one of: ' + str(options))

        return self._send('dnsmasq', cmd, *map(str, args))

    def shell(self, cmd, *args):
        '''
        Execute a command under the credentials of the running user. 

        Commands run until they complete or error, so take care not to execute
        a command that does not return.
        '''
        if not isinstance(cmd, str):
            raise TypeError('cmd must be a string')

        return self._send('shell', cmd, *map(str, args))

    def background(self, cmd, *args):
        '''
        Execute a command under the credentials of the running user. 

        Commands run in the background and control returns immediately. Any
        output is logged.
        '''
        if not isinstance(cmd, str):
            raise TypeError('cmd must be a string')

        return self._send('background', cmd, *map(str, args))

    def host_stats(self, quiet=False):
        '''
        report statistics about the host

        Report statistics about the host including hostname, load averages,
        total and free memory, and current bandwidth usage.

        To output host statistics without the header, set quiet=True.
        '''
        if not isinstance(quiet, bool):
            raise TypeError('quiet must be a bool')

        if quiet:
            return self._send('host_stats', 'quiet')

        return self._send('host_stats')

    def vm_snapshot(self, state=None):
        '''
        enable or disable snapshot mode when using disk images

        When enabled, disk images will be loaded in memory when run and changes
        will not be saved. This allows a single disk image to be used for many
        VMs.
        '''
        if state is None:
            return bool(self._send('vm_snapshot'))

        if not isinstance(state, bool):
            raise TypeError('state must be a bool')

        return self._send('vm_snapshot', str(state).lower())

    def optimize(self, ksm=None, hugepages=None, affinity=None,
        affinity_filter=None):
        '''
        enable or disable several virtualization optimizations

        Enable or disable several virtualization optimizations, including
        Kernel Samepage Merging, CPU affinity for VMs, and the use of
        hugepages.

        To enable Kernel Samepage Merging (KSM):
            mm.optimize(ksm=True)

        To enable hugepage support:
            mm.optimize(hugepages='/path/to/hugepages_mount')

        To disable hugepage support:
            mm.optimize(hugepages=False)

        To enable CPU affinity support:
            mm.optimize(affinity=True)

        To set a CPU set filter for the affinity scheduler (for example, to use
        only CPUs 1, 2-20):
            mm.optimize(affinity_filter='[1,2-20]')

        To clear a CPU set filter:
            mm.optimize(affinity_filter=False)
        '''
        # TODO(devin): figure out how to enable viewing of CPU affinity mapping
        # TODO(devin): refactor me, I'm hideous
        numSet = 0
        if ksm != None:
            numSet += 1
        if hugepages != None:
            numSet += 1
        if affinity != None:
            numSet += 1
        if affinity_filter != None:
            numSet += 1

        if numSet != 1:
            raise Error('only one optimization may be set at a time')

        if ksm != None:
            if not isinstance(ksm, bool):
                raise TypeError('ksm must be a bool')
            return self._send('optimize', 'ksm', 'true' if ksm else 'false')

        if hugepages != None:
            if not hugepages:
                return self._send('optimize', 'hugepages', '')
            if not isinstance(hugepages, str):
                raise TypeError('hugepages must be a string or False')
            return self._send('optimize', 'hugepages', hugepages)

        if affinity != None:
            if not isinstance(affinity, bool):
                raise TypeError('affinity must be a bool')
            return self._send('optimize', 'affinity', 'true' if affinity else
                              'false')

        #only thing left is affinity_filter
        if not isinstance(affinity_filter, str):
            raise TypeError('affinity_filter must be a string')
        return self._send('optimize', 'affinity_filter', affinity_filter)

    def version(self):
        '''display the version of minimega'''
        return self._send('version')

    def vm_config(self, cmd=None, filename=None):
        '''
        Display, save, or restore the current VM configuration.

        To display the current configuration, call vm_config with no arguments.

        List the current saved configurations with 'vm_config show'

        To save a configuration:

            mm.vm_config('save', <config name>)

        To restore a configuration:

            mm.vm_config('restore', <config name>)

        Calling mm.clear('vm_config') will clear all VM configuration options,
        but will not remove saved configurations.
        '''
        options = ('save', 'restore')

        if cmd is None:
            return self._send('vm_config')

        if not (isinstance(cmd, str) and isinstance(filename, str)):
            raise TypeError('cmd and filename must both be strings')

        if cmd not in options:
            raise ValueError('cmd must be one of: ' + str(options))

        return self._send('vm_config', cmd, filename)

    def debug(self, panic=False):
        '''
        Display internal debug information. Invoking with the panic=True will
        force minimega to dump a stacktrace upon crash or exit.
        '''
        if panic:
            return self._send('debug', 'panic')
        return self._send('debug')

    def bridge_info(self):
        '''display information about virtual bridges'''
        return self._send('bridge_info')

    def vm_flush(self):
        '''
        discard information about quit or failed VMs

        Discard information about VMs that have either quit or encountered an
        error. This will remove any VMs with a state of "quit" or "error" from
        vm_info. Names of VMs that have been flushed may be reused.
        '''
        return self._send('vm_flush')

    def viz(self, filename):
        '''
        visualize the current experiment as a graph

        Output the current experiment topology as a graphviz readable dot file.
        '''
        if not isinstance(filename, str):
            raise TypeError('filename must be a string')

        return self._send('viz', filename)

    def vyatta(self, cmd=None, *args):
        '''
        Define and write out vyatta router floppy disk images.

        vyatta takes a number of subcommands:

            'dhcp': Add DHCP service to a particular network by specifying the
            network, default gateway, and start and stop addresses. For
            example, to serve dhcp on 10.0.0.0/24, with a default gateway of
            10.0.0.1:

                mm.vyatta('dhcp', 'add', '10.0.0.0/24', '10.0.0.1', '10.0.0.2', '10.0.0.254')

                An optional DNS argument can be used to override the
                nameserver. For example, to do the same as above with a
                nameserver of 8.8.8.8:

                mm.vyatta('dhcp', 'add', '10.0.0.0/24', '10.0.0.1', '10.0.0.2', '10.0.0.254', '8.8.8.8')

            'interfaces': Add IPv4 addresses using CIDR notation. Optionally,
            'dhcp' or 'none' may be specified. The order specified matches the
            order of VLANs used in vm_net. This number of arguments must either
            be 0 or equal to the number of arguments in 'interfaces6' For
            example:

                mm.vyatta('interfaces', '10.0.0.1/24', 'dhcp')

            'interfaces6': Add IPv6 addresses similar to 'interfaces'. The
            number of arguments must either be 0 or equal to the number of
            arguments in 'interfaces'.

            'rad': Enable router advertisements for IPv6. Valid arguments are
            IPv6 prefixes or "none". Order matches that of interfaces6. For
            example:

                mm.vyatta('rad', '2001::/64', '2002::/64')

            'ospf': Route networks using OSPF. For example:

                mm.vyatta('ospf', '10.0.0.0/24', '12.0.0.0/24')

            'ospf3': Route IPv6 interfaces using OSPF3. For example:

                mm.vyatta('ospf3', 'eth0', 'eth1')

            'routes': Set static routes. Routes are specified as
            <network>,<next-hop> ... For example:

                mm.vyatta('routes', '2001::0/64,123::1', '10.0.0.0/24,12.0.0.1')

            'config': Override all other options and use a specified file as
            the config file. For example:
            
                mm.vyatta('config', '/tmp/myconfig.boot')

            'write': Write the current configuration to file. If a filename is
            omitted, a random filename will be used and the file placed in the
            path specified by the -filepath flag. The filename will be
            returned.
        '''
        options = (
            'dhcp', 'interfaces', 'interfaces6', 'rad', 'ospf', 'ospf3',
            'routes', 'config', 'write',
        )

        if cmd is None:
            return self._send('vyatta')

        if cmd not in options:
            raise ValueError('cmd must be one of: ' + str(options))

        return self._send('vyatta', cmd, *map(str, args))

    def vm_hotplug(self, cmd, *args):
        '''
        Add and remove USB drives to a launched VM. 

        To view currently attached media, call vm_hotplug with the 'show'
        argument and a VM ID or name. To add a device, use the 'add' argument
        followed by the VM ID or name, and the name of the file to add. For
        example, to add foo.img to VM 5:

            vm_hotplug add 5 foo.img

        The add command will assign a disk ID, shown in vm_hotplug show. To
        remove media, use the 'remove' argument with the VM ID and the disk ID.
        For example, to remove the drive added above, named 0:

            vm_hotplug remove 5 0

        To remove all hotplug devices, use ID -1.
        '''
        options = ('show', 'add', 'remove')

        if cmd not in options:
            raise ValueError('cmd must be one of: ' + str(options))

        return self._send('vm_hotplug', cmd, *map(str, args))

    def vm_netmod(self, vmid, conn, action):
        '''
        Disconnect or move existing network connections on a running VM. 

        Network connections are indicated by their position in vm_net (same
        order in vm_info) and are zero indexed. For example, to disconnect the
        first network connection from a VM with 4 network connections:

            mm.vm_netmod(<vm name or id>, 0, 'disconnect')

        To disconnect the second connection:

            mm.vm_netmod(<vm name or id>, 1, 'disconnect')

        To move a connection, specify the new VLAN tag:

            mm.vm_netmod(<vm name or id>, 0, 100)
        '''
        if not (isinstance(vmid, int) or isinstance(vmid, str)):
            raise TypeError('vmid must be an int or string')

        if not isinstance(conn, int):
            raise TypeError('conn must be an int')

        if not (action == 'disconnect' or isinstance(action, int)):
            raise TypeError('action must be either "disconnect" or an int')

        return self._send('vm_netmod', str(vmid), str(conn), str(action))

    def vm_inject(self, src_img, dst_img=None, *files):
        '''
        inject files into a qcow image

        Create a backed snapshot of a qcow2 image and injects one or more files
        into the new snapshot.

        Usage:

            mm.vm_inject(<src qcow image>[:<partition>], [<dst qcow image name>], <src file1>:<dst file1>, [<src file2>:<dst file2> ...])

        src qcow image - the name of the qcow to use as the backing image file.

        partition - The optional partition number in which the files should be
        injected. Partition defaults to 1, but if multiple partitions exist and
        partition is not explicitly specified, an error is thrown and files are
        not injected.

        dst qcow image name - The optional name of the snapshot image. This
        should be a name only, if any extra path is specified, an error is
        thrown. This file will be created at 'base'/files. A filename will be
        generated if this optional parameter is omitted.

        src file - The local file that should be injected onto the new qcow2
        snapshot.

        dst file - The path where src file should be injected in the new qcow2
        snapshot.

        If the src file or dst file contains spaces, use double quotes (" ") as
        in the following example:

            mm.vm_inject('src.qc2', 'dst.qc2' '"my file":"Program Files/my file"')

        Alternatively, when given a single argument, this command supplies the
        name of the backing qcow image for a snapshot image.

        Usage:

            mm.vm_inject('snapshot.qc2')
        '''
        if not isinstance(src_img, str):
            raise TypeError('src_img must be a string')

        if dst_img is None:
            return self._send('vm_inject', 'src_img')

        if not isinstance(dst_img, str):
            raise TypeError('dst_img must be a string')

        for f in files:
            if not isinstance(f, str):
                raise TypeError('files must be specified as strings')

        return self._send('vm_inject', src_img, dst_img, *files)

    def define(self, key=None, macro=None):
        '''
        define macros

        Define literal and function like macros.

        Macro keywords are in the form [a-zA-z0-9]+. When defining a macro, all
        text after the key is the macro expansion. For example:

            define key foo bar

        Will replace "key" with "foo bar" in all command line arguments.

        You can also specify function like macros in a similar way to function
        like macros in C. For example:

            define key(x,y) this is my x, this is my y

        Will replace all instances of x and y in the expansion with the
        variable arguments. When used:

            key(foo,bar)

        Will expand to:

            this is mbar foo, this is mbar bar

        To show defined macros, invoke define with no arguments.
        '''
        if key is None:
            return self._send('define')

        if not (isinstance(key, str) and isinstance(macro, str)):
            raise TypeError('key and macro must both be strings')

        return self._send('define', key, *macro.split(' '))

    def undefine(self, macro):
        '''undefine macros by name'''
        if not isinstance(macro, str):
            raise TypeError('macro must be a string')

        return self._send('undefine', macro)

    def echo(self, cmd):
        '''
        Returns the command after macro expansion and comment removal.

        cmd must be passed as a list of tokens.
        '''
        if not isinstance(cmd, list):
            raise TypeError('cmd must be a list of tokens')

        return self._send('echo', *map(str, cmd))

    def vm_qmp(self, vm_id, cmd):
        '''
        Issue a JSON-encoded QMP command. This is a convenience function for accessing
        the QMP socket of a VM via minimega. 

        Arguments:
        vm_id -- the integer ID of the VM to run the QMP command
        cmd -- a dictionary that represents the QMP command to be run

        Returns:
        The JSON-encoded result of running the QMP command

        Example:
            mm.vm_qmp(0, { "execute": "query-status" })

        Returns:
            {"return":{"running":false,"singlestep":false,"status":"prelaunch"}}
        '''
        if not isinstance(vm_id, int) and not isinstance(vm_id, str):
            raise TypeError('vm_id must be an integer or a string')

        if not isinstance(cmd, dict):
            raise TypeError('cmd must be a dictionary')

        return self._send('vm_qmp', str(vm_id), json.dumps(cmd))

    def capture(self, cmd=None, *args):
        '''
        capture experiment data

        Capture experiment data including netflow. Netflow capture obtains netflow data
        from any local openvswitch switch, and can write to file, another socket, or
        both. Netflow data can be written out in raw or ascii format, and file output
        can be compressed on the fly. Multiple netflow writers can be configured.
        Usage: capture [netflow <bridge> [file <filename> <raw,ascii> [gzip], socket <tcp,udp> <hostname:port> <raw,ascii>]]
        Usage: capture clear netflow <id,-1>
        Usage: capture netflow timeout <new timeout in seconds>

        Arguments:
        cmd -- the capture command to be run
        args -- arguments for the capture command

        Examples:
        To capture netflow data on all associated bridges to file in ascii
        mode and with gzip compression:

            mm.capture('netflow', 'file', 'foo.netflow', 'ascii', 'gzip')

        To clear captures on all bridges:

            mm.capture('clear', 'netflow', -1)

        You can change the active flow timeout with:

            mm.capture('netflow', 'timeout', <new timeout in seconds>)
        '''
        options = ('clear','netflow')

        if cmd is None:
            return self._send('capture')

        if cmd not in options:
            raise ValueError('cmd must be one of: ' + str(options))

        return self._send('capture', cmd, *map(str,args))

    def vm_uuid(self, uuid=None):
        '''
        Set the UUID for a VM. When called with no arguments, this will return the
        UUID for the current VM configuration.

        Arguments:
        uuid -- the UUID of the VM
        '''
        if uuid is None:
            return self._send('vm_uuid')

        if not isinstance(uuid, int) and not isinstance(uuid, str):
            raise TypeError('uuid must be an integer or string')

        return self._send('vm_uuid',str(uuid))

    def cc(self, cmd=None, *args):
        '''
        Command and control layer for minimega

        Arguments:
        cmd -- the cc command to be run
        args -- arguments for the cc command

        Examples:
        To senda file 'foo' and display the contents on a remove VM:
        
            mm.cc('command', 'new', 'command="cat foo"', 'filesend=foo')

        To filter on VMs that are running windows AND have a specific IP,
        OR nodes that have a range of IPs:

            mm.cc('filter', 'add', 'os=windows', 'ip=10.0.0.1')
            mm.cc('filter', 'add', 'ip=12.0.0.0/24')

        To clear commands:

            mm.cc('command', 'clear')

        To clear filters:

            mm.cc('filter', 'clear')
        '''
        options = ('start', 'filter', 'command')

        if cmd is None:
            return self._send('cc')

        if cmd not in options:
            raise ValueError('cmd must be one of: ' + str(options))

        return self._send('cc', cmd, *map(str,args))