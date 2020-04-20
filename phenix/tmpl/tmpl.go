package tmpl

import (
	"fmt"
	"io"
	"text/template"
)

func GenerateFromTemplate(name string, data interface{}, w io.Writer) error {
	tmpl := template.Must(template.New(name).Parse(string(MustAsset(name))))

	if err := tmpl.Execute(w, data); err != nil {
		return fmt.Errorf("executing %s template: %w", name, err)
	}

	return nil
}
