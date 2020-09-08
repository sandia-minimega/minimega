package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"phenix/api/vm"
	"phenix/util"

	"github.com/spf13/cobra"
)

func newVMCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vm",
		Short: "Virtual machine management",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	return cmd
}

func newVMInfoCmd() *cobra.Command {
	desc := `Table of virtual machine(s)
	
  Used to display a table of virtual machine(s) for a specific experiment; 
  virtual machine name is optional, when included will display only that VM.`

	cmd := &cobra.Command{
		Use:   "info <experiment name>/<vm name>",
		Short: "Table of virtual machine(s)",
		Long:  desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("Must provide an experiment name")
			}

			parts := strings.Split(args[0], "/")

			switch len(parts) {
			case 1:
				vms, err := vm.List(parts[0])
				if err != nil {
					err := util.HumanizeError(err, "Unable to get a list of VMs")
					return err.Humanized()
				}

				util.PrintTableOfVMs(os.Stdout, vms...)
			case 2:
				vm, err := vm.Get(parts[0], parts[1])
				if err != nil {
					err := util.HumanizeError(err, "Unable to get information for the "+parts[1]+" VM")
					return err.Humanized()
				}

				util.PrintTableOfVMs(os.Stdout, *vm)
			default:
				return fmt.Errorf("Invalid argument")
			}

			return nil
		},
	}

	return cmd
}

func newVMPauseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pause <experiment name> <vm name>",
		Short: "Pause a running VM for a specific experiment",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("Must provide an experiment and VM name")
			}

			var (
				expName = args[0]
				vmName  = args[1]
			)

			if err := vm.Pause(expName, vmName); err != nil {
				err := util.HumanizeError(err, "Unable to pause the "+vmName+" VM")
				return err.Humanized()
			}

			fmt.Printf("The %s VM in the %s experiment was paused\n", vmName, expName)

			return nil
		},
	}

	return cmd
}

func newVMResumeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resume <experiment name> <vm name>",
		Short: "Resume a paused VM for a specific experiment",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("Must provide an experiment and VM name")
			}

			var (
				expName = args[0]
				vmName  = args[1]
			)

			if err := vm.Resume(expName, vmName); err != nil {
				err := util.HumanizeError(err, "Unable to resume the "+vmName+" VM")
				return err.Humanized()
			}

			fmt.Printf("The %s VM in the %s experiment was resumed\n", vmName, expName)

			return nil
		},
	}

	return cmd
}

func newVMRedeployCmd() *cobra.Command {
	var (
		cpu  int
		mem  int
		part int
	)

	desc := `Redeploy a running experiment VM
	 
  Used to redeploy a running virtual machine for a specific experiment; several 
  values can be modified`

	cmd := &cobra.Command{
		Use:   "redeploy <experiment name> <vm name>",
		Short: "Redeploy a running experiment VM",
		Long:  desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("Must provide an experiment and VM name")
			}

			var (
				expName = args[0]
				vmName  = args[1]
				disk    = MustGetString(cmd.Flags(), "disk")
				inject  = MustGetBool(cmd.Flags(), "replicate-injects")
			)

			if cpu != 0 && (cpu < 1 || cpu > 8) {
				return fmt.Errorf("CPUs can only be 1-8")
			}

			if mem != 0 && (mem < 512 || mem > 16384 || mem%512 != 0) {
				return fmt.Errorf("Memory must be one of 512, 1024, 2048, 3072, 4096, 8192, 12288, 16384")
			}

			opts := []vm.RedeployOption{
				vm.CPU(cpu),
				vm.Memory(mem),
				vm.Disk(disk),
				vm.Inject(inject),
				vm.InjectPartition(part),
			}

			if err := vm.Redeploy(expName, vmName, opts...); err != nil {
				err := util.HumanizeError(err, "Unable to redeploy the "+vmName+" VM")
				return err.Humanized()
			}

			fmt.Printf("The %s VM in the %s experiment was redeployed\n", vmName, expName)

			return nil
		},
	}

	// not sure that this is the correct way to handle ints
	cmd.Flags().IntVarP(&cpu, "cpu", "c", 1, "Number of VM CPUs (1-8 is valid)")
	cmd.Flags().IntVarP(&mem, "mem", "m", 512, "Amount of memory in megabytes (512, 1024, 2048, 3072, 4096, 8192, 12288, 16384 are valid)")
	cmd.Flags().StringP("disk", "d", "", "VM backing disk image")
	cmd.Flags().BoolP("replicate-injects", "r", false, "Recreate disk snapshot and VM injections")
	cmd.Flags().IntVarP(&part, "partition", "p", 1, "Partition of disk to inject files into (only used if disk option is specified)")

	return cmd
}

func newVMKillCmd() *cobra.Command {
	desc := `Kill a running or paused VM
	
  Used to kill or delete a running or paused virtual machine for a specific 
  experiment`

	cmd := &cobra.Command{
		Use:   "kill <experiment name> <vm name>",
		Short: "Kill a running or pause VM",
		Long:  desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("Must provide an experiment and VM name")
			}

			var (
				expName = args[0]
				vmName  = args[1]
			)

			if err := vm.Kill(expName, vmName); err != nil {
				err := util.HumanizeError(err, "Unable to kill the "+vmName+" VM")
				return err.Humanized()
			}

			fmt.Printf("The %s VM in the %s experiment was killed\n", vmName, expName)

			return nil
		},
	}

	return cmd
}

func newVMSetCmd() *cobra.Command {
	desc := `Set configuration value for a VM
	
  Used to set a configuration value for a virtual machine in a stopped 
  experiment. This command is not yet implemented. For now, you can edit the 
  experiment directly with 'phenix config edit'`

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set configuration value for a VM",
		Long:  desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	return cmd
}

func newVMNetCmd() *cobra.Command {
	desc := `Modify network connectivity for a VM

  Used to modify the network connectivity for a virtual machine in a running
  experiment; see command help for connect or disconnect for additional
  arguments.`

	cmd := &cobra.Command{
		Use:   "net",
		Short: "Modify network connectivity for a VM",
		Long:  desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	connect := &cobra.Command{
		Use:   "connect <experiment name> <vm name> <iface index> <vlan id>",
		Short: "Connect a VM interface to a VLAN",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 4 {
				return fmt.Errorf("Must provide all arguments")
			}

			var (
				expName = args[0]
				vmName  = args[1]
				vlan    = args[3]
			)

			iface, err := strconv.Atoi(args[2])
			if err != nil {
				return fmt.Errorf("The network interface index must be an integer")
			}

			if err := vm.Connect(expName, vmName, iface, vlan); err != nil {
				err := util.HumanizeError(err, "Unable to modify the connectivity for the "+vmName+" VM")
				return err.Humanized()
			}

			fmt.Printf("The network for the %s VM in the %s experiment was modified\n", vmName, expName)

			return nil
		},
	}

	disconnect := &cobra.Command{
		Use:   "disconnect <experiment name> <vm name> <iface index>",
		Short: "Disconnect a VM interface",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 3 {
				return fmt.Errorf("Must provide all arguments")
			}

			var (
				expName = args[0]
				vmName  = args[1]
			)

			iface, err := strconv.Atoi(args[2])
			if err != nil {
				return fmt.Errorf("The network interface index must be an integer")
			}

			if err := vm.Disonnect(expName, vmName, iface); err != nil {
				err := util.HumanizeError(err, "Unable to disconnect the interface on the "+vmName+" VM")
				return err.Humanized()
			}

			fmt.Printf("The %d interface on the %s VM in the %s experiment was paused\n", iface, vmName, expName)

			return nil
		},
	}

	cmd.AddCommand(connect)
	cmd.AddCommand(disconnect)

	return cmd
}

func newVMCaptureCmd() *cobra.Command {
	desc := `Modify network packet captures for a VM
	
  Used to modify the network packet captures for a virtual machine in a running 
  experiment; see command help for start and stop for additional arguments.`

	cmd := &cobra.Command{
		Use:   "capture",
		Short: "Modify network packet captures for a VM",
		Long:  desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	start := &cobra.Command{
		Use:   "start <experiment name> <vm name> <iface index> </path/to/out file>",
		Short: "Start a packet capture",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 4 {
				return fmt.Errorf("Must provide all arguments")
			}

			var (
				expName = args[0]
				vmName  = args[1]
				out     = args[3]
			)

			iface, err := strconv.Atoi(args[2])
			if err != nil {
				return fmt.Errorf("The network interface index must be an integer")
			}

			if err := vm.StartCapture(expName, vmName, iface, out); err != nil {
				err := util.HumanizeError(err, "Unable to start a capture on the interface on the "+vmName+" VM")
				return err.Humanized()
			}

			fmt.Printf("A packet capture was started for the %d interface on the %s VM in the %s experiment\n", iface, vmName, expName)

			return nil
		},
	}

	stop := &cobra.Command{
		Use:   "stop <experiment name> <vm name>",
		Short: "Stop all packet captures",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("Must provide all arguments")
			}

			var (
				expName = args[0]
				vmName  = args[1]
			)

			if err := vm.StopCaptures(expName, vmName); err != nil {
				err := util.HumanizeError(err, "Unable to stop the packet capture(s) on the "+vmName+" VM")
				return err.Humanized()
			}

			fmt.Printf("The packet capture(s) for the %s VM in the %s experiment was stopped\n", vmName, expName)

			return nil
		},
	}

	cmd.AddCommand(start)
	cmd.AddCommand(stop)

	return cmd
}

func init() {
	vmCmd := newVMCmd()

	vmCmd.AddCommand(newVMInfoCmd())
	vmCmd.AddCommand(newVMPauseCmd())
	vmCmd.AddCommand(newVMResumeCmd())
	vmCmd.AddCommand(newVMRedeployCmd())
	vmCmd.AddCommand(newVMKillCmd())
	vmCmd.AddCommand(newVMSetCmd())
	vmCmd.AddCommand(newVMNetCmd())
	vmCmd.AddCommand(newVMCaptureCmd())

	rootCmd.AddCommand(vmCmd)
}
