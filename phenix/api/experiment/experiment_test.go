package experiment

import (
	"testing"

	"phenix/store"
	"phenix/types"

	"github.com/golang/mock/gomock"
)

func TestList(t *testing.T) {
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
	m.EXPECT().List(gomock.Eq("Experiment")).Return(configs, nil)

	store.DefaultStore = m

	c, err := List()
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	if len(c) != 1 {
		t.Log("expecting 1 config")
		t.FailNow()
	}
}
