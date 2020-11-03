package types

import (
	"phenix/store"
	v1 "phenix/types/version/v1"
)

type Image struct {
	Metadata store.ConfigMetadata
	Spec     *v1.Image
}
