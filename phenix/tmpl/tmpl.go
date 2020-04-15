package tmpl

import (
	"fmt"
	"io"
	"path"
	"runtime"
	"text/template"
)

func init() {
	_, filename, _, _ := runtime.Caller(0)

	glob := path.Dir(filename) + "/templates/*.tmpl"

	templates = template.Must(template.ParseGlob(glob))
}

var templates *template.Template

func GenerateFromTemplate(name string, data interface{}, w io.Writer) error {
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		return fmt.Errorf("executing %s template: %w", name, err)
	}

	return nil
}
