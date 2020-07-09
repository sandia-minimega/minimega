package app

import (
	"errors"
	"fmt"

	"phenix/types"
	"phenix/util/shell"
)

func init() {
	apps["ntp"] = new(NTP)
	apps["serial"] = new(Serial)
	apps["startup"] = new(Startup)
	apps["user-shell"] = new(UserApp)
	apps["vyatta"] = new(Vyatta)

	defaultApps = []string{"ntp", "serial", "startup", "vyatta"}
}

// Action represents the different experiment lifecycle hooks.
type Action string

const (
	ACTIONCONFIG    Action = "configure"
	ACTIONPRESTART  Action = "pre-start"
	ACTIONPOSTSTART Action = "post-start"
	ACTIONCLEANUP   Action = "cleanup"
)

var (
	apps        = make(map[string]App)
	defaultApps []string
)

func List() []string {
	var names []string

	for name := range apps {
		// Don't include app that wraps external user apps.
		if name == "user-shell" {
			continue
		}

		var exclude bool

		// Don't include default apps in the list since they always get applied.
		for _, d := range defaultApps {
			if name == d {
				exclude = true
				break
			}
		}

		if !exclude {
			names = append(names, name)
		}
	}

	for _, name := range shell.FindCommandsWithPrefix("phenix-app-") {
		names = append(names, name)
	}

	return names
}

// GetApp returns the initialized phenix app with the given name. If an app with
// the given name is not known internally, it returns the generic `user-shell`
// app that handles shelling out to external custom user apps.
func GetApp(name string) App {
	app, ok := apps[name]
	if !ok {
		app = apps["user-shell"]
		app.Init(Name(name))
	}

	return app
}

// DefaultApps returns a slice of all the initialized default phenix apps.
func DefaultApps() []App {
	a := make([]App, len(defaultApps))

	for i, app := range defaultApps {
		a[i] = GetApp(app)
	}

	return a
}

// App is the interface that identifies all the required functionality for a
// phenix app. Each experiment lifecycle hook function is passed a pointer to
// the experiment the app is being applied to, and the lifecycle hook function
// should modify the experiment as necessary. Not all lifecycle hook functions
// have to be implemented. If one (or more) isn't needed for an app, it should
// simply return nil.
type App interface {
	// Init is used to initialize a phenix app with options generic to all apps.
	Init(...Option) error

	// Name returns the name of the phenix app.
	Name() string

	// Configure is called for an app at the `configure` experiment lifecycle
	// phase.
	Configure(*types.Experiment) error

	// Start is called for an app at the `pre-start` experiment lifecycle phase.
	PreStart(*types.Experiment) error

	// PostStart is called for an app at the `post-start` experiment lifecycle
	// phase.
	PostStart(*types.Experiment) error

	// Cleanup is called for an app at the `cleanup` experiment lifecycle
	// phase.
	Cleanup(*types.Experiment) error
}

// ApplyApps applies all the default phenix apps and any configured user apps to
// the given experiment for the given lifecycle phase. It returns any errors
// encountered while applying the apps.
func ApplyApps(action Action, exp *types.Experiment) error {
	var err error

	for _, a := range DefaultApps() {
		switch action {
		case ACTIONCONFIG:
			fmt.Printf("configuring experiment with default '%s' app\n", a.Name())

			err = a.Configure(exp)
		case ACTIONPRESTART:
			fmt.Printf("pre-starting experiment with default '%s' app\n", a.Name())

			err = a.PreStart(exp)
		case ACTIONPOSTSTART:
			fmt.Printf("post-starting experiment with default '%s' app\n", a.Name())

			err = a.PostStart(exp)
		case ACTIONCLEANUP:
			fmt.Printf("cleaning up experiment with default '%s' app\n", a.Name())

			err = a.Cleanup(exp)
		}

		if err != nil {
			return fmt.Errorf("applying default app %s for action %s: %w", a.Name(), action, err)
		}
	}

	if exp.Spec.Scenario != nil && exp.Spec.Scenario.Apps != nil {
		for _, e := range exp.Spec.Scenario.Apps.Experiment {
			a := GetApp(e.Name)

			switch action {
			case ACTIONCONFIG:
				fmt.Printf("configuring experiment with '%s' user app\n", a.Name())

				err = a.Configure(exp)
			case ACTIONPRESTART:
				fmt.Printf("pre-starting experiment with '%s' user app\n", a.Name())

				err = a.PreStart(exp)
			case ACTIONPOSTSTART:
				fmt.Printf("post-starting experiment with '%s' user app\n", a.Name())

				err = a.PostStart(exp)
			case ACTIONCLEANUP:
				fmt.Printf("cleaning up experiment with '%s' user app\n", a.Name())

				err = a.Cleanup(exp)
			}

			if err != nil {
				if errors.Is(err, ErrUserAppNotFound) {
					fmt.Printf("experiment app %s not found\n", a.Name())
					continue
				}

				return fmt.Errorf("applying experiment app %s for action %s: %w", a.Name(), action, err)
			}
		}

		for _, h := range exp.Spec.Scenario.Apps.Host {
			a := GetApp(h.Name)

			switch action {
			case ACTIONCONFIG:
				fmt.Printf("configuring experiment with '%s' user app\n", a.Name())

				err = a.Configure(exp)
			case ACTIONPRESTART:
				fmt.Printf("pre-starting experiment with '%s' user app\n", a.Name())

				err = a.PreStart(exp)
			case ACTIONPOSTSTART:
				fmt.Printf("post-starting experiment with '%s' user app\n", a.Name())

				err = a.PreStart(exp)
			case ACTIONCLEANUP:
				fmt.Printf("cleaning up experiment with '%s' user app\n", a.Name())

				err = a.Cleanup(exp)
			}

			if err != nil {
				if errors.Is(err, ErrUserAppNotFound) {
					fmt.Printf("host app %s not found\n", a.Name())
					continue
				}

				return fmt.Errorf("applying host app %s for action %s: %w", a.Name(), action, err)
			}
		}
	}

	return nil
}
