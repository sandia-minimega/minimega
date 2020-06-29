package scheduler

import (
	"context"
	"errors"
	"testing"

	v1 "phenix/types/version/v1"
	"phenix/util/shell"

	gomock "github.com/golang/mock/gomock"
)

func TestUserAppNotFound(t *testing.T) {
	scheduler := new(userScheduler)
	scheduler.Init(Name("foobar"))

	if scheduler.Name() != "foobar" {
		t.Logf("unexpected user scheduler %s", scheduler.Name())
		t.FailNow()
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := shell.NewMockShell(ctrl)

	shell.DefaultShell = m

	m.EXPECT().CommandExists(gomock.Eq("phenix-scheduler-foobar")).Return(false)

	err := scheduler.Schedule(new(v1.ExperimentSpec))

	if err == nil {
		t.Log("expected error")
		t.FailNow()
	}

	if !errors.Is(err, ErrUserSchedulerNotFound) {
		t.Log("expected UserSchedulerNotFound error")
		t.FailNow()
	}
}

func TestUserAppFound(t *testing.T) {
	scheduler := new(userScheduler)
	scheduler.Init(Name("foobar"))

	if scheduler.Name() != "foobar" {
		t.Logf("unexpected user scheduler %s", scheduler.Name())
		t.FailNow()
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := shell.NewMockShell(ctrl)
	m.EXPECT().CommandExists(gomock.Eq("phenix-scheduler-foobar")).Return(true)

	opts := []shell.Option{}

	m.EXPECT().ExecCommand(gomock.AssignableToTypeOf(context.Background()), gomock.AssignableToTypeOf(opts)).Return([]byte(`{}`), nil, nil)

	shell.DefaultShell = m

	err := scheduler.Schedule(new(v1.ExperimentSpec))

	if err != nil {
		t.Logf("unexpected error %v", err)
		t.FailNow()
	}
}
