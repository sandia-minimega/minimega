package file

type ImageKind int
type CopyStatus func(float64)

const (
	_ ImageKind = iota
	VM_IMAGE
	CONTAINER_IMAGE
)

type ImageDetails struct {
	Kind     ImageKind
	Name     string
	FullPath string
	Size     int
}
