package app

import (
	"context"
	"errors"
	"testing"

	"phenix/types"
	"phenix/util/shell"

	gomock "github.com/golang/mock/gomock"
)

func TestUserAppNotFound(t *testing.T) {
	app := GetApp("foobar")

	if app.Name() != "foobar" {
		t.Logf("unexpected user app %s", app.Name())
		t.FailNow()
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := shell.NewMockShell(ctrl)

	shell.DefaultShell = m

	m.EXPECT().CommandExists(gomock.Eq("phenix-app-foobar")).Return(false)

	err := app.Configure(new(types.Experiment))

	if err == nil {
		t.Log("expected error")
		t.FailNow()
	}

	if !errors.Is(err, ErrUserAppNotFound) {
		t.Log("expected UserAppNotFound error")
		t.FailNow()
	}
}

func TestUserAppFound(t *testing.T) {
	app := GetApp("foobar")

	if app.Name() != "foobar" {
		t.Logf("unexpected user app %s", app.Name())
		t.FailNow()
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := shell.NewMockShell(ctrl)
	m.EXPECT().CommandExists(gomock.Eq("phenix-app-foobar")).Return(true)

	opts := []shell.Option{}

	m.EXPECT().ExecCommand(gomock.AssignableToTypeOf(context.Background()), gomock.AssignableToTypeOf(opts)).Return([]byte(`{}`), nil, nil)

	shell.DefaultShell = m

	err := app.Configure(new(types.Experiment))

	if err != nil {
		t.Logf("unexpected error %v", err)
		t.FailNow()
	}
}
