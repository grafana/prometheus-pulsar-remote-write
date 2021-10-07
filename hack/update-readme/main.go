package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
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

	edits := myers.ComputeEdits(span.URIFromPath("before"), string(data), string(newData))

	// nothing to do file matches
	if len(edits) == 0 {
		return nil
	}

	// if we don't want to update, show diffs
	if !update {
		fmt.Println(gotextdiff.ToUnified("before", "after", string(data), edits))
		return fmt.Errorf("The file %s needs updates, please run with '--update'", readmePath)
	}

	if err := ioutil.WriteFile(readmePath, newData, 0644); err != nil {
		return fmt.Errorf("error writing readme file: %w", err)
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
