#!/usr/bin/env python3
"""Build the router sandwich with the minimega Python bindings.

Chapter 21 of the minimega miniclass. Run it as root on a host where minimega
is running and the chapter 2 images (miniccc.kernel, miniccc.initrd,
minirouter.kernel, minirouter.initrd) are in the files directory. It builds
the same experiment as scripts/03-router-sandwich.mm in the "sandwich"
namespace and prints one line per VM.

The earlier chapter scripts leave VMs in the "sandwich" namespace, and the
bindings cannot clear a namespace. Clear it before and after each run with:

    sudo minimega -e clear namespace sandwich
"""

import sys
import time

import minimega

NAMESPACE = "sandwich"
ROUTER_WAIT = 10  # seconds for minirouter to boot and apply its configuration
READY_TIMEOUT = 300  # seconds to wait for every VM to connect and get an address


def build(mm):
    """Configure and launch the two clients and the router."""
    mm.clear_vm_config()

    # The clients: miniccc image, 3 GB each, one on each network
    mm.vm_config_kernel("miniccc.kernel")
    mm.vm_config_initrd("miniccc.initrd")
    mm.vm_config_memory(3072)
    mm.vm_config_networks("net_left")
    mm.vm_launch_kvm("vm-left")
    mm.vm_config_networks("net_right")
    mm.vm_launch_kvm("vm-right")

    # The router: minirouter image, one interface on each network
    mm.vm_config_kernel("minirouter.kernel")
    mm.vm_config_initrd("minirouter.initrd")
    mm.vm_config_networks("net_left net_right")
    mm.vm_launch_kvm("router")

    # Router configuration is staged, then applied by commit
    mm.router_interface("router", 0, "10.0.0.1/24")
    mm.router_dhcp_range("router", "10.0.0.0", "10.0.0.2", "10.0.0.254")
    mm.router_dhcp_router("router", "10.0.0.0", "10.0.0.1")
    mm.router_interface("router", 1, "10.0.1.1/24")
    mm.router_dhcp_range("router", "10.0.1.0", "10.0.1.2", "10.0.1.254")
    mm.router_dhcp_router("router", "10.0.1.0", "10.0.1.1")
    mm.router_commit("router")

    # Start the router first so DHCP is up before the clients ask for a lease
    mm.vm_start("router")
    time.sleep(ROUTER_WAIT)
    mm.vm_start("all")
    wait_ready(mm)


def wait_ready(mm):
    """Wait until every VM has a connected cc agent and an IP address."""
    deadline = time.time() + READY_TIMEOUT
    while time.time() < deadline:
        vms = [vm for r in mm.vm_info() if r["Tabular"] for vm in minimega.as_dict(r)]
        if vms and all(vm["cc_active"] == "true" and "10." in vm["ip"] for vm in vms):
            return
        time.sleep(3)
    raise TimeoutError(f"VMs not ready after {READY_TIMEOUT} s")


def report(mm):
    """Print name, state, VLANs and IPs for every VM in the namespace."""
    for resp in mm.vm_info():
        if not resp["Tabular"]:
            continue
        for vm in minimega.as_dict(resp):
            print(f'{vm["name"]:10} {vm["state"]:9} {vm["vlan"]:34} {vm["ip"]}')


def main():
    try:
        mm = minimega.connect(namespace=NAMESPACE)
    except OSError as err:
        sys.exit(f"cannot connect to /tmp/minimega/minimega: {err}")

    try:
        build(mm)
    except (minimega.Error, TimeoutError) as err:
        sys.exit(f"minimega: {err}")

    report(mm)


if __name__ == "__main__":
    main()
