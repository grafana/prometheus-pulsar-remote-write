package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
)

func run(droneYMLPath string, repo string, update bool) error {

	stdin, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("error reading from stdin: %w", err)
	}
	stdin = []byte(strings.TrimSpace(string(stdin)))

	existing, err := ioutil.ReadFile(droneYMLPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error reading existing drone.yml: %w", err)
	}

	if pos := bytes.Index(existing, []byte("\n---\nkind: signature")); pos > 0 {
		existing = existing[0:pos]
	}

	edits := myers.ComputeEdits(span.URIFromPath("before"), string(existing), string(stdin))

	// nothing to do file matches
	if len(edits) == 0 {
		return nil
	}

	// if we don't want to update, show diffs
	if !update {
		fmt.Println(gotextdiff.ToUnified("before", "after", string(existing), edits))
		return fmt.Errorf("The file %s needs updates, please run with '--update'", droneYMLPath)
	}

	// sign if repo is set
	if repo != "" {
		tempF, err := ioutil.TempFile("", "drone.yml")
		if err != nil {
			return fmt.Errorf("error creating temp file for signing: %w", err)
		}
		defer os.Remove(tempF.Name())

		if _, err := tempF.Write(stdin); err != nil {
			return fmt.Errorf("error writing to temp file for signing: %w", err)
		}
		if err := tempF.Close(); err != nil {
			return fmt.Errorf("error closing temp file for signing: %w", err)
		}

		sign := exec.Command("drone", "sign", "--save", repo, tempF.Name())
		sign.Stdout = os.Stdout
		sign.Stderr = os.Stderr
		if err := sign.Run(); err != nil {
			return fmt.Errorf("error signing drone config: %w", err)
		}

		stdin, err = ioutil.ReadFile(tempF.Name())
		if err != nil {
			return fmt.Errorf("error reading signed drone config in temp file: %w", err)
		}
	}

	// write drone config
	if err := ioutil.WriteFile(droneYMLPath, stdin, 0644); err != nil {
		return fmt.Errorf("error writing to drone config: %w", err)
	}
	return nil
}

var (
	updateDroneYML bool
	droneYMLPath   string
	repo           string
)

func init() {
	flag.StringVar(&droneYMLPath, "path", ".drone/drone.yml", "Path to drone.yaml")
	flag.BoolVar(&updateDroneYML, "update", false, "Write the changes to the drone.yml file")
	flag.StringVar(&repo, "repo", "", "If given will sign the updated config using drone sign")
}

func main() {
	flag.Parse()

	if err := run(droneYMLPath, repo, updateDroneYML); err != nil {
		log.Fatal(err)
	}
}
