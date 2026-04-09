package shellscripts

import (
	"bytes"
	"embed"
	"io"
	"strconv"
	"strings"
	"text/template"
)

//go:embed templates/*
var shellTemplates embed.FS

type Args struct {
	Protocol         string
	Host             string
	Port             string
	Name             string
	Arch             string
	OS               string
	WorkingDirectory string
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func pythonQuote(value string) string {
	return strconv.Quote(value)
}

func powerShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func MakeTemplate(attributes Args, extension string) ([]byte, error) {

	file, err := shellTemplates.Open("templates/" + extension)
	if err != nil {
		return nil, err
	}

	t, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	template, err := template.New("shell").Funcs(template.FuncMap{
		"shq": shellQuote,
		"pyq": pythonQuote,
		"psq": powerShellQuote,
	}).Parse(string(t))
	if err != nil {
		return nil, err
	}

	var b bytes.Buffer
	err = template.Execute(&b, attributes)
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil

}
