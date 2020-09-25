package config

import (
	"testing"

	"phenix/store"
	"phenix/types"

	"github.com/golang/mock/gomock"
)

func TestListError(t *testing.T) {
	configs := types.Configs(
		[]types.Config{
			{
				Version: "phenix.sandia.gov/v1",
				Kind:    "Experiment",
				Metadata: types.ConfigMetadata{
					Name: "test-experiment",
				},
			},
		},
	)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := store.NewMockStore(ctrl)
	m.EXPECT().List(gomock.Eq("Topology"), gomock.Eq("Scenario"), gomock.Eq("Experiment"), gomock.Eq("Image")).Return(configs, nil).AnyTimes()

	store.DefaultStore = m

	_, err := List("blech")
	t.Log(err)
	if err == nil {
		t.Log("expected error")
		t.FailNow()
	}
}
