package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"regexp"
	"strings"
)

func run(readmePath string, update bool) error {
	stdin, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("error reading from stdin: %w", err)
	}
	stdin = []byte(strings.TrimSpace(string(stdin)))

	data, err := ioutil.ReadFile(readmePath)
	if err != nil {
		return fmt.Errorf("error reading readme file: %w", err)
	}

	r := regexp.MustCompile("(?s)## Usage\\s+```([^`]*)```")

	stdin = append([]byte("## Usage\n\n```\n"), stdin...)
	stdin = append(stdin, []byte("\n```")...)

	newData := r.ReplaceAll(data, stdin)

	if update {
		if err := ioutil.WriteFile(readmePath, newData, 0644); err != nil {
			return fmt.Errorf("error writing readme file: %w", err)
		}
		return nil
	}

	if !reflect.DeepEqual(newData, data) {
		return fmt.Errorf("The file %s needs updates, please run with '--update'", readmePath)
	}

	return nil
}

var (
	updateReadme bool
	readmePath   string
)

func init() {
	flag.StringVar(&readmePath, "path", "README.md", "Path to README")
	flag.BoolVar(&updateReadme, "update", false, "Write the changes to the README.md file")
}

func main() {
	flag.Parse()

	if err := run(readmePath, updateReadme); err != nil {
		log.Fatal(err)
	}
}
