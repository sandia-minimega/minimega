package image

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"phenix/store"
	"phenix/tmpl"
	"phenix/types"
	v1 "phenix/types/version/v1"
	"phenix/util"
	"phenix/util/shell"

	"github.com/activeshadow/structs"
	"github.com/mitchellh/mapstructure"
)

const (
	V_VERBOSE   int = 1
	V_VVERBOSE  int = 2
	V_VVVERBOSE int = 4
)

var (
	ErrMinicccNotFound   = fmt.Errorf("miniccc executable not found")
	ErrProtonukeNotFound = fmt.Errorf("protonuke executable not found")
)

// SetDefaults will set default settings to image values if none are set by the
// user. The default values are:
//   -- Image size at `5G`
//   -- The variant is `minbase`
//   -- The release is `bionic` (Ubuntu 18.04.4 LTS)
//   -- The mirror is `http://us.archive.ubuntu.com/ubuntu/`
//   -- The image format is `raw`
// This will also remove empty strings in packages and overlays; if overlays are
// used, the default `/phenix/images` directory is added to the overlay name.
// Based on the variant value, specific constants will be included during the
// create sub-command. The values are passed from the `constants.go` file. An
// error will be returned if the variant value is not valid (acceptable values
// are `minbase`, `mingui`, `kali`, or `brash`).
func SetDefaults(img *v1.Image) error {
	if img.Size == "" {
		img.Size = "5G"
	}

	if img.Variant == "" {
		img.Variant = "minbase"
	}

	if img.Release == "" {
		img.Release = "bionic"
	}

	if img.Mirror == "" {
		img.Mirror = "http://us.archive.ubuntu.com/ubuntu/"
	}

	if img.Format == "" {
		img.Format = "raw"
	}

	if !strings.Contains(img.DebAppend, "--components=") {
		if img.Release == "kali" || img.Release == "kali-rolling" {
			img.DebAppend += " --components=" + strings.Join(PACKAGES_COMPONENTS_KALI, ",")
		} else {
			img.DebAppend += " --components=" + strings.Join(PACKAGES_COMPONENTS, ",")
		}
	}

	switch img.Variant {
	case "minbase":
		if img.Release == "kali" || img.Release == "kali-rolling" {
			img.Packages = append(img.Packages, PACKAGES_DEFAULT...)
			img.Packages = append(img.Packages, PACKAGES_KALI...)
		} else {
			img.Packages = append(img.Packages, PACKAGES_DEFAULT...)
			img.Packages = append(img.Packages, PACKAGES_BIONIC...)
		}
	case "mingui":
		if img.Release == "kali" || img.Release == "kali-rolling" {
			img.Packages = append(img.Packages, PACKAGES_DEFAULT...)
			img.Packages = append(img.Packages, PACKAGES_KALI...)
			img.Packages = append(img.Packages, PACKAGES_MINGUI...)
			img.Packages = append(img.Packages, PACKAGES_MINGUI_KALI...)
		} else {
			img.Packages = append(img.Packages, PACKAGES_DEFAULT...)
			img.Packages = append(img.Packages, PACKAGES_BIONIC...)
			img.Packages = append(img.Packages, PACKAGES_MINGUI...)
			if img.Release == "xenial" {
				img.Packages = append(img.Packages, "qupzilla")
			} else {
				img.Packages = append(img.Packages, "falkon")
			}
		}
	case "brash":
		if img.Release == "kali" || img.Release == "kali-rolling" {
			img.Packages = append(img.Packages, PACKAGES_DEFAULT...)
			img.Packages = append(img.Packages, PACKAGES_KALI...)
			img.Packages = append(img.Packages, PACKAGES_BRASH...)
		} else {
			img.Packages = append(img.Packages, PACKAGES_DEFAULT...)
			img.Packages = append(img.Packages, PACKAGES_BIONIC...)
			img.Packages = append(img.Packages, PACKAGES_BRASH...)
		}
	default:
		return fmt.Errorf("variant %s is not implemented", img.Variant)
	}

	img.Scripts = make(map[string]string)

	addScriptToImage(img, "POSTBUILD_APT_CLEANUP", POSTBUILD_APT_CLEANUP)

	switch img.Variant {
	case "minbase", "mingui":
		addScriptToImage(img, "POSTBUILD_NO_ROOT_PASSWD", POSTBUILD_NO_ROOT_PASSWD)
		addScriptToImage(img, "POSTBUILD_PHENIX_HOSTNAME", POSTBUILD_PHENIX_HOSTNAME)
	default:
		return fmt.Errorf("variant %s is not implemented", img.Variant)
	}

	if len(img.ScriptPaths) > 0 {
		for _, p := range img.ScriptPaths {
			if err := addScriptToImage(img, p, ""); err != nil {
				return fmt.Errorf("adding script %s to image config: %w", p, err)
			}
		}
	}

	return nil
}

// Create collects image values from user input at command line, creates an
// image configuration, and then persists it to the store. SetDefaults is used
// to set default values if the user did not include any in the image create
// sub-command. This sub-command requires an image `name`. It will return any
// errors encoutered while creating the configuration.
func Create(name string, img *v1.Image) error {
	if name == "" {
		return fmt.Errorf("image name is required to create an image")
	}

	if err := SetDefaults(img); err != nil {
		return fmt.Errorf("setting image defaults: %w", err)
	}

	c := types.Config{
		Version:  "phenix.sandia.gov/v1",
		Kind:     "Image",
		Metadata: types.ConfigMetadata{Name: name},
		Spec:     structs.MapDefaultCase(img, structs.CASESNAKE),
	}

	if err := store.Create(&c); err != nil {
		return fmt.Errorf("storing image config: %w", err)
	}

	return nil
}

// CreateFromConfig will take in an existing image configuration by name and
// modify overlay, packages, and scripts as passed by the user. It will then
// persist a new image configuration to the store. Any errors enountered will be
// passed when creating a new image configuration, retrieving the exisitng image
// configuration file, or storing the new image configuration file in the store.
func CreateFromConfig(name, saveas string, overlays, packages, scripts []string) error {
	c, err := types.NewConfig("image/" + name)
	if err != nil {
		return fmt.Errorf("creating new image config for %s: %w", name, err)
	}

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting config from store: %w", err)
	}

	var img v1.Image

	if err := mapstructure.Decode(c.Spec, &img); err != nil {
		return fmt.Errorf("decoding image spec: %w", err)
	}

	if err := SetDefaults(&img); err != nil {
		return fmt.Errorf("setting image defaults: %w", err)
	}

	c.Metadata.Name = saveas

	if len(overlays) > 0 {
		img.Overlays = append(img.Overlays, overlays...)
	}

	if len(packages) > 0 {
		img.Packages = append(img.Packages, packages...)
	}

	if len(scripts) > 0 {
		for _, s := range scripts {
			if err := addScriptToImage(&img, s, ""); err != nil {
				return fmt.Errorf("adding script %s to image config: %w", s, err)
			}
		}
	}

	c.Spec = structs.MapDefaultCase(img, structs.CASESNAKE)

	if err := store.Create(c); err != nil {
		return fmt.Errorf("storing new image config %s in store: %w", saveas, err)
	}

	return nil
}

// Build uses the image configuration `name` passed by users to build an image.
// If verbosity is set, `vmdb` will output progress as it builds the image.
// Otherwise, there will only be output if an error is encountered. The image
// configuration is used with a template to build the `vmdb` configuration file
// and then pass it to the shelled out `vmdb` command. This expects the `vmdb`
// application is in the `$PATH`. Any errors encountered will be returned during
// the process of getting an existing image configuration, decoding it,
// generating the `vmdb` verbosconfiguration file, or executing the `vmdb` command.
func Build(ctx context.Context, name string, verbosity int, cache bool, dryrun bool, output string) error {
	c, _ := types.NewConfig("image/" + name)

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting image config %s from store: %w", name, err)
	}

	var img v1.Image

	if err := mapstructure.Decode(c.Spec, &img); err != nil {
		return fmt.Errorf("decoding image spec: %w", err)
	}

	if verbosity >= V_VVVERBOSE {
		img.VerboseLogs = true
	}

	img.Cache = cache

	// The Kali package repos use `kali-rolling` as the release name.
	if img.Release == "kali" {
		img.Release = "kali-rolling"
	}

	if img.IncludeMiniccc != "" {
		if err := addMinicccToImage(&img, name); err != nil {
			if errors.Is(err, ErrMinicccNotFound) {
				util.AddWarnings(ctx, err)
			} else {
				return fmt.Errorf("adding miniccc to image: %w", err)
			}
		}
	}

	if img.IncludeProtonuke != "" {
		if err := addProtonukeToImage(&img, name); err != nil {
			if errors.Is(err, ErrProtonukeNotFound) {
				util.AddWarnings(ctx, err)
			} else {
				return fmt.Errorf("adding protonuke to image: %w", err)
			}
		}
	}

	filename := output + "/" + name + ".vmdb"

	if err := tmpl.CreateFileFromTemplate("vmdb.tmpl", img, filename); err != nil {
		return fmt.Errorf("generate vmdb config from template: %w", err)
	}

	if !dryrun && !shell.CommandExists("vmdb2") {
		return fmt.Errorf("vmdb2 app does not exist in your path")
	}

	args := []string{
		filename,
		"--output", output + "/" + name,
		"--rootfs-tarball", output + "/" + name + ".tar",
	}

	if verbosity >= V_VERBOSE {
		args = append(args, "-v")
	}

	if verbosity >= V_VVERBOSE {
		args = append(args, "--log", "stderr")
	}

	if dryrun {
		fmt.Printf("DRY RUN: vmdb2 %s\n", strings.Join(args, " "))
	} else {
		cmd := exec.Command("vmdb2", args...)

		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("starting vmdb2 command: %w", err)
		}

		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				fmt.Println(scanner.Text())
			}
		}()

		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				fmt.Println(scanner.Text())
			}
		}()

		if err := cmd.Wait(); err != nil {
			return fmt.Errorf("building image with vmdb2: %w", err)
		}
	}

	return nil
}

// List collects image configurations from the store. It returns a slice of all
// configurations. It will return any errors encountered while getting the list
// of image configurations.
func List() ([]types.Image, error) {
	configs, err := store.List("Image")
	if err != nil {
		return nil, fmt.Errorf("getting list of image configs from store: %w", err)
	}

	var images []types.Image

	for _, c := range configs {
		spec := new(v1.Image)

		if err := mapstructure.Decode(c.Spec, spec); err != nil {
			return nil, fmt.Errorf("decoding image spec: %w", err)
		}

		img := types.Image{Metadata: c.Metadata, Spec: spec}

		images = append(images, img)
	}

	return images, nil
}

// Update retrieves the named image configuration file from the store and will
// update scripts. First, it will verify the script is present on disk. If so,
// it will remove the existing script from the configuration file and update the
// file with updated. It will return any errors encountered during the process
// of creating a new image configuration, decoding it, or updating it in the
// store.
func Update(name string) error {
	c, err := types.NewConfig("image/" + name)
	if err != nil {
		return fmt.Errorf("creating new image config for %s: %w", name, err)
	}

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting config from store: %w", err)
	}

	var img v1.Image

	if err := mapstructure.Decode(c.Spec, &img); err != nil {
		return fmt.Errorf("decoding image spec: %w", err)
	}

	scripts := img.Scripts

	if len(scripts) > 0 {
		for k := range scripts {
			if _, err := os.Stat(k); err == nil {
				delete(img.Scripts, k)

				if err := addScriptToImage(&img, k, ""); err != nil {
					return fmt.Errorf("adding script %s to image config: %w", k, err)
				}
			}
		}
	}

	c.Spec = structs.MapDefaultCase(img, structs.CASESNAKE)

	if err := store.Update(c); err != nil {
		return fmt.Errorf("updating image config in store: %w", err)
	}

	return nil
}

// Append retrieves the named image configuration file from the store and will
// update it with overlays, packages, and scripts as passed by the user. It will
// return any errors encountered during the process of creating a new image
// configuration, decoding it, or updating it in the store.
func Append(name string, overlays, packages, scripts []string) error {
	c, err := types.NewConfig("image/" + name)
	if err != nil {
		return fmt.Errorf("creating new image config for %s: %w", name, err)
	}

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting config from store: %w", err)
	}

	var img v1.Image

	if err := mapstructure.Decode(c.Spec, &img); err != nil {
		return fmt.Errorf("decoding image spec: %w", err)
	}

	if len(overlays) > 0 {
		img.Overlays = append(img.Overlays, overlays...)
	}

	if len(packages) > 0 {
		img.Packages = append(img.Packages, packages...)
	}

	if len(scripts) > 0 {
		for _, s := range scripts {
			if err := addScriptToImage(&img, s, ""); err != nil {
				return fmt.Errorf("adding script %s to image config: %w", s, err)
			}
		}
	}

	c.Spec = structs.MapDefaultCase(img, structs.CASESNAKE)

	if err := store.Update(c); err != nil {
		return fmt.Errorf("updating image config in store: %w", err)
	}

	return nil
}

// Remove will update an existing image configuration by removing the overlays,
// packages, and scripts as passed by the user. It will return any errors
// encountered during the process of creating a new image configuration,
// decoding it, or updating it in the store.
func Remove(name string, overlays, packages, scripts []string) error {
	c, err := types.NewConfig("image/" + name)
	if err != nil {
		return fmt.Errorf("creating new image config for %s: %w", name, err)
	}

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting config from store: %w", err)
	}

	var img v1.Image

	if err := mapstructure.Decode(c.Spec, &img); err != nil {
		return fmt.Errorf("decoding image spec: %w", err)
	}

	if len(overlays) > 0 {
		o := img.Overlays[:0]

		for _, overlay := range img.Overlays {
			var match bool

			for _, n := range overlays {
				if n == overlay {
					match = true
					break
				}
			}

			if !match {
				o = append(o, overlay)
			}
		}

		img.Overlays = o
	}

	if len(packages) > 0 {
		p := img.Packages[:0]

		for _, pkg := range img.Packages {
			var match bool

			for _, n := range packages {
				if n == pkg {
					match = true
					break
				}
			}

			if !match {
				p = append(p, pkg)
			}
		}

		img.Packages = p
	}

	if len(scripts) > 0 {
		for _, s := range scripts {
			delete(img.Scripts, s)
		}
	}

	c.Spec = structs.MapDefaultCase(img, structs.CASESNAKE)

	if err := store.Update(c); err != nil {
		return fmt.Errorf("updating image config in store: %w", err)
	}

	return nil
}

func addScriptToImage(img *v1.Image, name, script string) error {
	if script == "" {
		u, err := url.Parse(name)
		if err != nil {
			return fmt.Errorf("parsing script path: %w", err)
		}

		// Default to file scheme if no scheme provided.
		if u.Scheme == "" {
			u.Scheme = "file"
		}

		var (
			loc  = u.Host + u.Path
			body io.ReadCloser
		)

		switch u.Scheme {
		case "http", "https":
			resp, err := http.Get(name)
			if err != nil {
				return fmt.Errorf("getting script via HTTP(s): %w", err)
			}

			body = resp.Body
		case "file":
			body, err = os.Open(loc)
			if err != nil {
				return fmt.Errorf("opening script file: %w", err)
			}
		default:
			return fmt.Errorf("scheme %s not supported for scripts", u.Scheme)
		}

		defer body.Close()

		contents, err := ioutil.ReadAll(body)
		if err != nil {
			return fmt.Errorf("processing script %s: %w", name, err)
		}

		script = string(contents)
	}

	img.Scripts[name] = script
	img.ScriptOrder = append(img.ScriptOrder, name)

	return nil
}

func addMinicccToImage(img *v1.Image, name string) error {
	pattern := fmt.Sprintf("%s-miniccc-overlay", name)

	dir, err := ioutil.TempDir("", pattern)
	if err != nil {
		return fmt.Errorf("creating temp directory for miniccc overlay: %w", err)
	}

	binPath := fmt.Sprintf("%s/usr/local/bin", dir)
	if err := os.MkdirAll(binPath, 0755); err != nil {
		return fmt.Errorf("creating directory structure for miniccc overlay: %w", err)
	}

	src, err := os.Open("/usr/local/share/minimega/bin/miniccc")
	if err != nil {
		return ErrMinicccNotFound
	}

	defer src.Close()

	dst, err := os.OpenFile(binPath+"/miniccc", os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		return fmt.Errorf("opening miniccc destination file: %w", err)
	}

	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copying miniccc file to overlay: %w", err)
	}

	img.Overlays = append(img.Overlays, dir)

	return nil
}

func addProtonukeToImage(img *v1.Image, name string) error {
	pattern := fmt.Sprintf("%s-protonuke-overlay", name)

	dir, err := ioutil.TempDir("", pattern)
	if err != nil {
		return fmt.Errorf("creating temp directory for protonuke overlay: %w", err)
	}

	binPath := fmt.Sprintf("%s/usr/local/bin", dir)
	if err := os.MkdirAll(binPath, 0755); err != nil {
		return fmt.Errorf("creating directory structure for protonuke overlay: %w", err)
	}

	src, err := os.Open("/usr/local/share/minimega/bin/protonuke")
	if err != nil {
		return ErrProtonukeNotFound
	}

	defer src.Close()

	dst, err := os.OpenFile(binPath+"/protonuke", os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		return fmt.Errorf("opening protonuke destination file: %w", err)
	}

	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copying protonuke file to overlay: %w", err)
	}

	img.Overlays = append(img.Overlays, dir)

	return nil
}
