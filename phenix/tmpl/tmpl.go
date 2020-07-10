package tmpl

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"
)

// GenerateFromTemplate executes the template with the given name using the
// given data. The result is written to the given writer. The templates used are
// located in the `phenix/tmpl/templates' directory. Each template will have a
// function with the signature `add(int, int)` available to it via a
// `template.FuncMap`. It returns any errors encountered while executing the
// template.
func GenerateFromTemplate(name string, data interface{}, w io.Writer) error {
	funcs := template.FuncMap{
		"addInt": func(a, b int) int {
			return a + b
		},
		"derefBool": func(b *bool) bool {
			return *b
		},
	}

	tmpl := template.Must(template.New(name).Funcs(funcs).Parse(string(MustAsset(name))))

	if err := tmpl.Execute(w, data); err != nil {
		return fmt.Errorf("executing %s template: %w", name, err)
	}

	return nil
}

// CreateFileFromTemplate executes the template with the given name using the
// given data. The result is written to the given file. Internally it calls
// `GenerateFromTemplate`. It returns any errors encountered while executing the
// template.
func CreateFileFromTemplate(name string, data interface{}, filename string) error {
	dir := filepath.Dir(filename)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating template path: %w", err)
	}

	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("creating template file: %w", err)
	}

	defer f.Close()

	return GenerateFromTemplate(name, data, f)
}
