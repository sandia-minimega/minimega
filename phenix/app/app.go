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

func GetApp(name string) App {
	app, ok := apps[name]
	if !ok {
		app = apps["user-shell"]
		app.Init(Name(name))
	}

	return app
}

func DefaultApps() []App {
	a := make([]App, len(defaultApps))

	for i, app := range defaultApps {
		a[i] = GetApp(app)
	}

	return a
}

type App interface {
	Init(...Option) error
	Name() string

	Configure(*v1.ExperimentSpec) error
	Start(*v1.ExperimentSpec) error
	PostStart(*v1.ExperimentSpec) error
	Cleanup(*v1.ExperimentSpec) error
}

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

	return nil
}
