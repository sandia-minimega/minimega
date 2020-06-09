package image

import (
	"bufio"
	"fmt"
	"io"
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

	"github.com/activeshadow/structs"
	"github.com/mitchellh/mapstructure"
)

const (
	SCRIPT_START_COMMENT = "## %s START ##"
	SCRIPT_END_COMMENT   = "## %s END ##"
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

	for _, l := range strings.Split(POSTBUILD_APT_CLEANUP, "\n") {
		img.Scripts = append(img.Scripts, l)
	}

	switch img.Variant {
	case "brash":
		for _, l := range strings.Split(POSTBUILD_BRASH, "\n") {
			img.Scripts = append(img.Scripts, l)
		}

		fallthrough
	case "minbase", "mingui":
		for _, l := range strings.Split(POSTBUILD_NO_ROOT_PASSWD, "\n") {
			img.Scripts = append(img.Scripts, l)
		}

		for _, l := range strings.Split(POSTBUILD_PHENIX_HOSTNAME, "\n") {
			img.Scripts = append(img.Scripts, l)
		}
	default:
		return fmt.Errorf("variant %s is not implemented", img.Variant)
	}

	if len(img.ScriptPaths) > 0 {
		if err := addScriptsToImage(img, img.ScriptPaths); err != nil {
			return fmt.Errorf("adding scripts to image config: %w", err)
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

	c.Metadata.Name = saveas

	if len(overlays) > 0 {
		img.Overlays = append(img.Overlays, overlays...)
	}

	if len(packages) > 0 {
		img.Packages = append(img.Packages, packages...)
	}

	if len(scripts) > 0 {
		if err := addScriptsToImage(&img, scripts); err != nil {
			return fmt.Errorf("adding scripts to image config: %w", err)
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
// generating the `vmdb` configuration file, or executing the `vmdb` command.
func Build(name, verbosity string, cache bool) error {
	c, _ := types.NewConfig("image/" + name)

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting image config %s from store: %w", name, err)
	}

	var img v1.Image

	if err := mapstructure.Decode(c.Spec, &img); err != nil {
		return fmt.Errorf("decoding image spec: %w", err)
	}

	if verbosity != "" {
		img.Verbosity = verbosity
	}

	img.Cache = cache

	// The Kali package repos use `kali-rolling` as the release name.
	if img.Release == "kali" {
		img.Release = "kali-rolling"
	}

	dir := "/phenix/images"
	filename := dir + "/" + name + ".vmdb"

	if err := tmpl.CreateFileFromTemplate("vmdb.tmpl", img, filename); err != nil {
		return fmt.Errorf("generate vmdb config from template: %w", err)
	}

	if !util.ShellCommandExists("vmdb2") {
		return fmt.Errorf("vmdb2 app does not exist in your path")
	}

	args := []string{
		filename,
		"--output", dir + "/" + name,
		"--rootfs-tarball", dir + "/" + name + ".tar",
	}

	switch verbosity {
	case "v":
		args = append(args, "-v")
	case "vv":
		fallthrough
	case "vvv":
		args = append(args, "-v", "--log", "stderr")
	}

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
		if err := addScriptsToImage(&img, scripts); err != nil {
			return fmt.Errorf("adding scripts to image config: %w", err)
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
		if err := removeScriptsFromImage(&img, scripts); err != nil {
			return fmt.Errorf("removing scripts from image config: %w", err)
		}
	}

	c.Spec = structs.MapDefaultCase(img, structs.CASESNAKE)

	if err := store.Update(c); err != nil {
		return fmt.Errorf("updating image config in store: %w", err)
	}

	return nil
}

func addScriptsToImage(img *v1.Image, scriptPaths []string) error {
	var scripts []string

	for _, p := range scriptPaths {
		u, err := url.Parse(p)
		if err != nil {
			return fmt.Errorf("parsing script path: %w", err)
		}

		// Default to file scheme if no scheme provided.
		if u.Scheme == "" {
			u.Scheme = "file"
		}

		var body io.ReadCloser

		switch u.Scheme {
		case "http", "https":
			resp, err := http.Get(p)
			if err != nil {
				return fmt.Errorf("getting script via HTTP(s): %w", err)
			}

			body = resp.Body
		case "file":
			body, err = os.Open(u.Host + u.Path)
			if err != nil {
				return fmt.Errorf("opening script file: %w", err)
			}
		default:
			return fmt.Errorf("scheme %s not supported for scripts", u.Scheme)
		}

		defer body.Close()

		scripts = append(scripts, fmt.Sprintf(SCRIPT_START_COMMENT, p))

		scanner := bufio.NewScanner(body)
		scanner.Split(bufio.ScanLines)

		for scanner.Scan() {
			scripts = append(scripts, scanner.Text())
		}

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("processing script %s: %w", p, err)
		}

		scripts = append(scripts, fmt.Sprintf(SCRIPT_END_COMMENT, p))
	}

	img.Scripts = append(img.Scripts, scripts...)

	return nil
}

func removeScriptsFromImage(img *v1.Image, scriptPaths []string) error {
	for _, p := range scriptPaths {
		var (
			matcher = fmt.Sprintf(SCRIPT_START_COMMENT, p)
			start   = -1
			end     = -1
		)

		for i, l := range img.Scripts {
			if l == matcher {
				if start < 0 {
					start = i
					matcher = fmt.Sprintf(SCRIPT_END_COMMENT, p)
				} else {
					end = i
					break
				}
			}
		}

		if start >= 0 && end > 0 {
			img.Scripts = append(img.Scripts[:start], img.Scripts[end+1:]...)
		}
	}

	return nil
}
