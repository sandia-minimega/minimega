package types

import (
	v1 "phenix/types/version/v1"
)

type Image struct {
	Metadata ConfigMetadata
	Spec     *v1.Image
}
