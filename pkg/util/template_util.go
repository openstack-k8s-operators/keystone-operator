package util

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"text/template"
)

// ExecuteTemplateFile creates a template from the file and execute it with the specified data
func ExecuteTemplateFile(filename string, data interface{}) string {

        templates := os.Getenv("KEYSTONE_OPERATOR_TEMPLATES")
	filepath := ""
        if templates == "" {
            // support local testing with 'up local'
	    _, basefile, _, _ := runtime.Caller(1)
	    filepath = path.Join(path.Dir(basefile), "../../templates/" + filename)
        } else {
            // deployed as a container
	    filepath = path.Join(templates + filename)
        }

	b, err := ioutil.ReadFile(filepath)
	if err != nil {
		panic(err)
	}
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
