// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

const stringTemplate = `{
	HelpShort: "configures {{ .ConfigName }}",
	HelpLong: ` + "`{{ .Doc }}`," + `
	Patterns: []string{
		"vm config {{ .ConfigName }} [value]",
	},
	Call: wrapSimpleCLI(func (ns *Namespace, c *minicli.Command, r *minicli.Response) error {
		if len(c.StringArgs) == 0 {
			r.Response = ns.vmConfig.{{ .Field }}
			return nil
		}

		{{ if .Path }}
		v := c.StringArgs["value"]

		// Ensure that relative paths are always relative to /files/
		if !filepath.IsAbs(v) {
			v = filepath.Join(*f_iomBase, v)
		}

		if _, err := os.Stat(v); os.IsNotExist(err) {
			log.Warn("file does not exist: %v", v)
		}

		ns.vmConfig.{{ .Field }} = v
		{{ else }}
		ns.vmConfig.{{ .Field }} = c.StringArgs["value"]
		{{ end }}

		return nil
	}),
},
`

const sliceTemplate = `{
	HelpShort: "configures {{ .ConfigName }}",
	HelpLong: ` + "`{{ .Doc }}`," + `
	Patterns: []string{
		"vm config {{ .ConfigName }} [value]...",
	},
	Call: wrapSimpleCLI(func (ns *Namespace, c *minicli.Command, r *minicli.Response) error {
		if len(c.ListArgs) == 0 {
			if len(ns.vmConfig.{{ .Field }}) == 0 {
				return nil
			}

			r.Response = fmt.Sprintf("%v", ns.vmConfig.{{ .Field }})
			return nil
		}

		{{ if .Path }}
		vals := c.ListArgs["value"]

		for i, v := range vals {
			// Ensure that relative paths are always relative to /files/
			if !filepath.IsAbs(v) {
				// TODO: mmmga
				v = filepath.Join(*f_iomBase, v)
				vals[i] = v
			}

			if _, err := os.Stat(v); os.IsNotExist(err) {
				log.Warn("file does not exist: %v", v)
			}
		}

		ns.vmConfig.{{ .Field }} = vals
		{{ else }}
		ns.vmConfig.{{ .Field }} = c.ListArgs["value"]
		{{ end }}

		return nil
	}),
},
`

// numTemplate handles int64 and uint64
const numTemplate = `{
	HelpShort: "configures {{ .ConfigName }}",
	HelpLong: ` + "`{{ .Doc }}`," + `
	Patterns: []string{
		"vm config {{ .ConfigName }} [value]",
	},
	Call: wrapSimpleCLI(func (ns *Namespace, c *minicli.Command, r *minicli.Response) error {
		if len(c.StringArgs) == 0 {
			{{- if .Signed }}
			r.Response = strconv.FormatInt(ns.vmConfig.{{ .Field }}, 10)
			{{- else }}
			r.Response = strconv.FormatUint(ns.vmConfig.{{ .Field }}, 10)
			{{- end }}
			return nil
		}

		{{ if .Signed -}}
		i, err := strconv.ParseInt(c.StringArgs["value"], 10, 64)
		{{- else }}
		i, err := strconv.ParseUint(c.StringArgs["value"], 10, 64)
		{{- end }}
		if err != nil {
			return err
		}

		ns.vmConfig.{{ .Field }} = i

		return nil
	}),
},
`

const boolTemplate = `{
	HelpShort: "configures {{ .ConfigName }}",
	HelpLong: ` + "`{{ .Doc }}`," + `
	Patterns: []string{
		"vm config {{ .ConfigName }} [true,false]",
	},
	Call: wrapSimpleCLI(func (ns *Namespace, c *minicli.Command, r *minicli.Response) error {
		if len(c.BoolArgs) == 0 {
			r.Response = strconv.FormatBool(ns.vmConfig.{{ .Field }})
			return nil
		}

		ns.vmConfig.{{ .Field }} = c.BoolArgs["true"]

		return nil
	}),
},
`

const clearTemplate = `{
	HelpShort: "reset one or more configurations to default value",
	Patterns: []string{
		"clear vm config",
		{{- range . }}
		"clear vm config <{{ .ConfigName }},>",
		{{- end }}
	},
	Call: wrapSimpleCLI(func (ns *Namespace, c *minicli.Command, r *minicli.Response) error {
		// at most one key will be set in BoolArgs but we don't know what it
		// will be so we have to loop through the args and set whatever key we
		// see.
		mask := Wildcard
		for k := range c.BoolArgs {
			mask = k
		}

		ns.vmConfig.Clear(mask)

		return nil
	}),
},
`

const funcsTemplate = `
{{ range $type, $fields := . }}
func (v *{{ $type }}) Info(field string) (string, error) {
		{{- range $fields }}
		if field == "{{ .ConfigName }}" {
			{{- if eq .Type "string" }}
			return v.{{ .Field }}, nil
			{{- else if eq .Type "uint64" }}
			return strconv.FormatUint(v.{{ .Field }}, 10), nil
			{{- else if eq .Type "bool" }}
			return strconv.FormatBool(v.{{ .Field }}), nil
			{{- else }}
			return fmt.Sprintf("%v", v.{{ .Field }}), nil
			{{- end }}
		}
		{{- end }}

		return "", fmt.Errorf("invalid info field: %v", field)
}

func (v *{{ $type }} ) Clear(mask string) {
		{{- range $fields }}
		if mask == Wildcard || mask == "{{ .ConfigName }}" {
			v.{{ .Field }} = {{ .Default }}
		}
		{{- end }}
}
{{ end }}
`
