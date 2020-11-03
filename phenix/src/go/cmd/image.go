package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"phenix/api/config"
	"phenix/api/image"
	v1 "phenix/types/version/v1"
	"phenix/util"
	"phenix/util/printer"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newImageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Virtual disk image management",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	return cmd
}

func newImageListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Table of image configurations",
		RunE: func(cmd *cobra.Command, args []string) error {
			imgs, err := image.List()
			if err != nil {
				err := util.HumanizeError(err, "Unable to print a list of configurations")
				return err.Humanized()
			}

			var optional []string

			if MustGetBool(cmd.Flags(), "format") {
				optional = append(optional, "Format")
			}

			if MustGetBool(cmd.Flags(), "compressed") {
				optional = append(optional, "Compressed")
			}

			if MustGetBool(cmd.Flags(), "mirror") {
				optional = append(optional, "Mirror")
			}

			if len(imgs) == 0 {
				fmt.Println("\nThere are no image configurations available\n")
			} else {
				printer.PrintTableOfImageConfigs(os.Stdout, optional, imgs...)
			}

			return nil
		},
	}

	cmd.Flags().BoolP("format", "f", false, "Include disk image format")
	cmd.Flags().BoolP("compressed", "c", false, "Include disk compression")
	cmd.Flags().BoolP("mirror", "m", false, "Include debootstrap mirror")

	return cmd
}

func newImageCreateCmd() *cobra.Command {
	desc := `Create a disk image configuration

  Used to create a virtual disk image configuration from which to build 
	an image.
	
	When specifying the --size option, the following units can be used:
	
	M - Megabytes
	G - Gigabytes
	
	When specifying the --include-miniccc or --include-protonuke options,
	the directory to install the miniccc and/or protonuke executable into
	should be provided. For example:
	
	--include-miniccc=/usr/local/bin
	
	When building the image, the build subcommand will look for the miniccc
	and/or protonuke executable in /usr/local/share/minimega/bin on the host
	building the image.`

	example := `
  phenix image create <image name>
  phenix image create --size 2G --variant mingui --release xenial --format qcow2 --compress --overlays foobar --packages foo --scripts bar <image name>`

	cmd := &cobra.Command{
		Use:     "create <image name>",
		Short:   "Create a disk image configuration",
		Long:    desc,
		Example: example,
		RunE: func(cmd *cobra.Command, args []string) error {
			var img v1.Image

			if len(args) == 0 {
				return fmt.Errorf("Must provide an image name")
			} else if len(args) > 1 {
				// This might happen if, for example, multiple overlays are provided to
				// the overlays flag space-delimited instead of comma-delimited.
				return fmt.Errorf("Must provide an image name as the only argument (check that you are using commas where required for flags)")
			}

			name := args[0]
			img.Size = MustGetString(cmd.Flags(), "size")
			img.Variant = MustGetString(cmd.Flags(), "variant")
			img.Release = MustGetString(cmd.Flags(), "release")
			img.Mirror = MustGetString(cmd.Flags(), "mirror")
			img.Format = v1.Format(MustGetString(cmd.Flags(), "format"))
			img.Compress = MustGetBool(cmd.Flags(), "compress")
			img.Ramdisk = MustGetBool(cmd.Flags(), "ramdisk")
			img.DebAppend = MustGetString(cmd.Flags(), "debootstrap-append")
			img.IncludeMiniccc = MustGetString(cmd.Flags(), "include-miniccc")
			img.IncludeProtonuke = MustGetString(cmd.Flags(), "include-protonuke")

			if overlays := MustGetString(cmd.Flags(), "overlays"); overlays != "" {
				img.Overlays = strings.Split(overlays, ",")
			}

			if packages := MustGetString(cmd.Flags(), "packages"); packages != "" {
				img.Packages = strings.Split(packages, ",")
			}

			if scripts := MustGetString(cmd.Flags(), "scripts"); scripts != "" {
				img.ScriptPaths = strings.Split(scripts, ",")
			}

			units := img.Size[len(img.Size)-1:]
			if units != "M" && units != "G" {
				return fmt.Errorf("Must provide a valid unit for disk size option (e.g., '500M' or '10G')")
			}

			if err := image.Create(name, &img); err != nil {
				err := util.HumanizeError(err, "Unable to create the "+name+" image")
				return err.Humanized()
			}

			fmt.Printf("The configuration for the %s image was created\n", name)

			return nil
		},
	}

	cmd.Flags().StringP("size", "s", "5G", "Image size to use")
	cmd.Flags().StringP("variant", "v", "minbase", "Image variant to use")
	cmd.Flags().StringP("release", "r", "bionic", "OS release codename")
	cmd.Flags().StringP("mirror", "m", "http://us.archive.ubuntu.com/ubuntu/", "Debootstrap mirror (must match release)")
	cmd.Flags().StringP("format", "f", "raw", "Format of disk image")
	cmd.Flags().BoolP("compress", "c", false, "Compress image after creation (does not apply to raw image)")
	cmd.Flags().BoolP("ramdisk", "R", false, "Create a kernel/initrd pair in addition to a disk image")
	cmd.Flags().StringP("overlays", "O", "", "List of overlay names (include full path; separated by comma)")
	cmd.Flags().StringP("packages", "P", "", "List of packages to include in addition to those provided by variant (separated by comma)")
	cmd.Flags().StringP("scripts", "T", "", "List of scripts to include in addition to the defaults (include full path; separated by comma)")
	cmd.Flags().StringP("debootstrap-append", "d", "", `Additional arguments to debootstrap "(default: --components=main,restricted,universe,multiverse)"`)
	cmd.Flags().String("include-miniccc", "", `Include the miniccc executable in the directory provided`)
	cmd.Flags().String("include-protonuke", "", `Include the protonuke executable in the directory provided`)

	return cmd
}

func newImageCreateFromCmd() *cobra.Command {
	desc := `Create image configuration from existing one
	
  Used to create a new virtual disk image configuration from an existing one; 
  if options are used they will be added to the exisiting configuration.`

	cmd := &cobra.Command{
		Use:   "create-from <existing configuration> <new configuration>",
		Short: "Create image configuration from existing one",
		Long:  desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("The name of a existing and/or new configuration is required")
			}

			var (
				name     = args[0]
				saveas   = args[1]
				overlays []string
				packages []string
				scripts  []string
			)

			if opt := MustGetString(cmd.Flags(), "overlays"); opt != "" {
				overlays = strings.Split(opt, ",")
			}

			if opt := MustGetString(cmd.Flags(), "packages"); opt != "" {
				packages = strings.Split(opt, ",")
			}

			if opt := MustGetString(cmd.Flags(), "scripts"); opt != "" {
				scripts = strings.Split(opt, ",")
			}

			if err := image.CreateFromConfig(name, saveas, overlays, packages, scripts); err != nil {
				err := util.HumanizeError(err, "Unable to create the configuration file "+saveas)
				return err.Humanized()
			}

			fmt.Printf("The configuration for the %s image was created from %s\n", saveas, name)

			return nil
		},
	}

	cmd.Flags().StringP("overlays", "O", "", "List of overlay names (include full path; separated by comma)")
	cmd.Flags().StringP("packages", "P", "", "List of packages to include in addition to those provided by variant (separated by comma)")
	cmd.Flags().StringP("scripts", "T", "", "List of scripts to include in addition to the defaults (include full path; separated by comma)")

	return cmd
}

func newImageBuildCmd() *cobra.Command {
	desc := `Build a virtual disk image
	
  Used to build a new virtual disk using an exisitng configuration; vmdb2 must 
  be in path.`

	example := `
  phenix image build <configuration name>
  phenix image build --very-very-verbose --output </path/to/dir/>`

	cmd := &cobra.Command{
		Use:     "build <configuration name>",
		Short:   "Build a virtual disk image",
		Long:    desc,
		Example: example,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("The name of a configuration to build the disk image is required")
			}

			var (
				name      = args[0]
				cache     = MustGetBool(cmd.Flags(), "cache")
				dryrun    = MustGetBool(cmd.Flags(), "dry-run")
				output    string
				verbosity int
			)

			if MustGetString(cmd.Flags(), "output") == "" {
				cwd, err := os.Getwd()
				if err != nil {
					err := util.HumanizeError(err, "Unable to get the current working directory")
					return err.Humanized()
				}

				output = cwd
			} else {
				output = MustGetString(cmd.Flags(), "output")
			}

			if MustGetBool(cmd.Flags(), "verbose") {
				verbosity = verbosity | image.V_VERBOSE
			}

			if MustGetBool(cmd.Flags(), "very-verbose") {
				verbosity = verbosity | image.V_VVERBOSE
			}

			if MustGetBool(cmd.Flags(), "very-very-verbose") {
				verbosity = verbosity | image.V_VVVERBOSE
			}

			ctx := context.Background()

			if err := image.Build(ctx, name, verbosity, cache, dryrun, output); err != nil {
				err := util.HumanizeError(err, "Unable to build the "+name+" image")
				return err.Humanized()
			}

			if warns := util.Warnings(ctx); warns != nil {
				printer := color.New(color.FgYellow)

				for _, warn := range warns {
					printer.Printf("[WARNING] %v\n", warn)
				}
			}

			fmt.Printf("The %s image was successfully built\n", name)

			return nil
		},
	}

	// panic: "vv" shorthand is more than one ASCII character
	cmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
	cmd.Flags().BoolP("very-verbose", "w", false, "Enable very verbose output")
	cmd.Flags().BoolP("very-very-verbose", "x", false, "Enable very verbose output plus additional verbose output from debootstrap")
	cmd.Flags().BoolP("cache", "c", false, "Cache rootfs as tar archive")
	cmd.Flags().BoolP("dry-run", "", false, "Do everything but actually call out to vmdb2")
	cmd.Flags().StringP("output", "o", "", "Specify the output directory for the disk image to be saved to")

	return cmd
}

func newImageDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <configuration name>",
		Short: "Delete an existing image configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if name == "" {
				return fmt.Errorf("The name of the configuration to delete is required")
			}

			if err := config.Delete("image/" + name); err != nil {
				err := util.HumanizeError(err, "Unable to delete the "+name+" image")
				return err.Humanized()
			}

			fmt.Printf("The configuration for the %s image was deleted\n", name)

			return nil
		},
	}

	return cmd
}

func newImageAppendCmd() *cobra.Command {
	desc := `Append to an image configuration
	
  Used to add scripts, packages, and/or overlays to an existing virtual disk 
  image configuration.`

	cmd := &cobra.Command{
		Use:   "append <configuration name>",
		Short: "Append to an image configuration",
		Long:  desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("The name of a configuration to append to is required")
			}

			var (
				name     = args[0]
				overlays = strings.Split(MustGetString(cmd.Flags(), "overlays"), ",")
				packages = strings.Split(MustGetString(cmd.Flags(), "packages"), ",")
				scripts  = strings.Split(MustGetString(cmd.Flags(), "scripts"), ",")
			)

			if err := image.Append(name, overlays, packages, scripts); err != nil {
				err := util.HumanizeError(err, "Unable to append to the "+name+" image")
				return err.Humanized()
			}

			fmt.Printf("Scripts, packages, and/or overlays for the %s configuration were appended\n", name)

			return nil
		},
	}

	cmd.Flags().StringP("overlays", "O", "", "List of overlay names (include full path; separated by comma)")
	cmd.Flags().StringP("packages", "P", "", "List of packages to include in addition to those provided by variant (separated by comma)")
	cmd.Flags().StringP("scripts", "T", "", "List of scripts to include in addition to the defaults (include full path; separated by comma)")

	return cmd
}

func newImageRemoveCmd() *cobra.Command {
	desc := `Remove from an image configuration
	
  Used to remove scripts, packages, and/or overlays to an existing virtual disk 
  image configuration`

	cmd := &cobra.Command{
		Use:   "remove <configuration name>",
		Short: "Remove from an image configuration",
		Long:  desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("The name of a configuration to remove from is required")
			}

			var (
				name     = args[0]
				overlays = strings.Split(MustGetString(cmd.Flags(), "overlays"), ",")
				packages = strings.Split(MustGetString(cmd.Flags(), "packages"), ",")
				scripts  = strings.Split(MustGetString(cmd.Flags(), "scripts"), ",")
			)

			if err := image.Remove(name, overlays, packages, scripts); err != nil {
				err := util.HumanizeError(err, "Unable to remove from the "+name+" image")
				return err.Humanized()
			}

			fmt.Printf("Scripts, packages, and/or overlays for the %s configuration were removed\n", name)

			return nil
		},
	}

	cmd.Flags().StringP("overlays", "O", "", "List of overlay names (include full path; separated by comma)")
	cmd.Flags().StringP("packages", "P", "", "List of packages to include in addition to those provided by variant (separated by comma)")
	cmd.Flags().StringP("scripts", "T", "", "List of scripts to include in addition to the defaults (include full path; separated by comma)")

	return cmd
}

func newImageUpdateCmd() *cobra.Command {
	desc := `Update a script on an image configuration
	
  Used to update scripts, packages, and/or overlays to an existing virtual disk 
  image configuration`

	cmd := &cobra.Command{
		Use:   "update <configuration name>",
		Short: "Update a script on an image configuration",
		Long:  desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("The name of a configuration to update is required")
			}

			name := args[0]

			if err := image.Update(name); err != nil {
				err := util.HumanizeError(err, "Unable to update scripts from the "+name+" image")
				return err.Humanized()
			}

			fmt.Printf("The script(s) for the %s configuration were updated\n", name)

			return nil
		},
	}

	return cmd
}

func init() {
	imageCmd := newImageCmd()

	imageCmd.AddCommand(newImageListCmd())
	imageCmd.AddCommand(newImageCreateCmd())
	imageCmd.AddCommand(newImageCreateFromCmd())
	imageCmd.AddCommand(newImageBuildCmd())
	imageCmd.AddCommand(newImageDeleteCmd())
	imageCmd.AddCommand(newImageAppendCmd())
	imageCmd.AddCommand(newImageRemoveCmd())
	imageCmd.AddCommand(newImageUpdateCmd())

	rootCmd.AddCommand(imageCmd)
}
