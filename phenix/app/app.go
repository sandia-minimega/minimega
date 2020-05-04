package app

import (
	"errors"
	"fmt"

	v1 "phenix/types/version/v1"
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
	ACTIONSTART     Action = "start"
	ACTIONPOSTSTART Action = "postStart"
	ACTIONCLEANUP   Action = "cleanup"
)

var (
	apps        = make(map[string]App)
	defaultApps []string
)

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
	Configure(*v1.ExperimentSpec) error

	// Start is called for an app at the `start` experiment lifecycle phase.
	Start(*v1.ExperimentSpec) error

	// PostStart is called for an app at the `postStart` experiment lifecycle
	// phase.
	PostStart(*v1.ExperimentSpec) error

	// Cleanup is called for an app at the `cleanup` experiment lifecycle
	// phase.
	Cleanup(*v1.ExperimentSpec) error
}

// ApplyApps applies all the default phenix apps and any configured user apps to
// the given experiment for the given lifecycle phase. It returns any errors
// encountered while applying the apps.
func ApplyApps(action Action, spec *v1.ExperimentSpec) error {
	var err error

	for _, a := range DefaultApps() {
		switch action {
		case ACTIONCONFIG:
			fmt.Printf("configuring experiment with default '%s' app\n", a.Name())

			err = a.Configure(spec)
		case ACTIONSTART:
			fmt.Printf("starting experiment with default '%s' app\n", a.Name())

			err = a.Start(spec)
		case ACTIONPOSTSTART:
		case ACTIONCLEANUP:
		}

		if err != nil {
			return fmt.Errorf("applying default app %s for action %s: %w", a.Name(), action, err)
		}
	}

	if spec.Scenario != nil && spec.Scenario.Apps != nil {
		for _, e := range spec.Scenario.Apps.Experiment {
			a := GetApp(e.Name)

			switch action {
			case ACTIONCONFIG:
				fmt.Printf("configuring experiment with '%s' user app\n", a.Name())

				err = a.Configure(spec)
			case ACTIONSTART:
				fmt.Printf("starting experiment with '%s' user app\n", a.Name())

				err = a.Start(spec)
			case ACTIONPOSTSTART:
			case ACTIONCLEANUP:
			}

			if err != nil {
				if errors.Is(err, ErrUserAppNotFound) {
					fmt.Printf("experiment app %s not found\n", a.Name())
					continue
				}

				return fmt.Errorf("applying experiment app %s for action %s: %w", a.Name(), action, err)
			}
		}

		for _, h := range spec.Scenario.Apps.Host {
			a := GetApp(h.Name)

			switch action {
			case ACTIONCONFIG:
				fmt.Printf("configuring experiment with '%s' user app\n", a.Name())

				err = a.Configure(spec)
			case ACTIONSTART:
				fmt.Printf("starting experiment with '%s' user app\n", a.Name())

				err = a.Start(spec)
			case ACTIONPOSTSTART:
			case ACTIONCLEANUP:
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
