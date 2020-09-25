# phenix Types

The `phenix` platform has three main configuration types/schemas that it
deals with: `topology`, `scenario`, and `experiment`.

Each of the above three configuration types has a set of structs that
represent it and enable marshaling/unmarshaling to/from both JSON and YAML.
Those structs are defined in this `types` package.

## Topology

In `phenix`, a topology represents a set of VMs and networks that will the
the System Under Test (SUT) in an experiment. A topology should be
platform/cluster agnostic (ie. not be specific to any cluster node
requirements), as well as application agnostic (ie. nothing specific to a
`phenix` application, such as app configuration metadata, should be present).

At a high level, the topology schema identifies the nodes and networks
(VLANs) that should make up an experiment, along with networking
configuration that allows for the nodes to communicate over the networks as
intended.

## Scenario

In `phenix`, a scenario represents a set of experiment-wide and host-specific
`phenix` application configurations that, when applied to a topology,
instrument the topology with the necessary additional software,
configuration, etc. to carry out a specific scenario as part of an exercise
(ie. represent an industrial control system).

## Experiment

In `phenix`, an experiment represents the combination of a topology and a
scenario, along with cluster-specific settings (ie. ensuring an experiment VM
runs on a specific node for hardware-in-the-loop). Such an experiment is used
to actually orchistrate a cluster (such as minimega) in order to create the
desired SUT.