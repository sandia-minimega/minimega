// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

// This package provides a singleton bridge object that wraps openvswitch. It
// allows the programmatic creation and deletion of bridges and taps, packet
// captures, applying qos constraints, and adding tunnels and trunks.
//
// It also tracks information about taps such as recent bandwidth stats and
// snoops on traffic to the identify IP addresses associated with them.
package bridge
