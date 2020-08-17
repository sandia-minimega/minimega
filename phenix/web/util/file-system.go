package util

import (
	"net/http"

	"github.com/elazarl/go-bindata-assetfs"
)

type BinaryFileSystem struct {
	assets *assetfs.AssetFS
}

func NewBinaryFileSystem(assets *assetfs.AssetFS) *BinaryFileSystem {
	return &BinaryFileSystem{assets: assets}
}

func (this BinaryFileSystem) ServeFile(w http.ResponseWriter, r *http.Request, name string) {
	f, err := this.assets.Open(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer f.Close()

	d, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.ServeContent(w, r, d.Name(), d.ModTime(), f)
}
