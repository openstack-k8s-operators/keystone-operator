package util

import (
	"bytes"
	"io/ioutil"
	"path"
	"runtime"
	"text/template"
)

// create a template from the file and execute it with the specified data
func ExecuteTemplateFile(filename string, data interface{}) string {
	_, basefile, _, _ := runtime.Caller(1)
	filepath := path.Join(path.Dir(basefile), filename)
	b, err := ioutil.ReadFile(filepath)
	file := string(b)
	var buff bytes.Buffer
	tmpl, err := template.New("tmp").Parse(file)
	if err != nil {
		panic(err)
	}
	err = tmpl.Execute(&buff, data)
	if err != nil {
		panic(err)
	}
	return buff.String()
}
