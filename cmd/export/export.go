/*
 * Copyright (c) 2021, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package export

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"

	yaml "gopkg.in/yaml.v2"
)

var log = logrus.New()

func GetLogger() *logrus.Logger {
	return log
}

const (
	JSONFormat         = "json"
	YAMLFormat         = "yaml"
	DefaultConfigLabel = "current"
)

type Flags struct {
	OutputFormat string
	ConfigLabel  string
	Placements   bool
}

type Context struct {
	*cli.Context
	Flags *Flags
}

func BuildCommand() *cli.Command {
	// Create a flags struct to hold our flags
	exportFlags := Flags{}

	// Create the 'export' command
	export := cli.Command{}
	export.Name = "export"
	export.Usage = "Export the MIG configuration from all GPUs in a compatible format"
	export.Action = func(c *cli.Context) error {
		return exportWrapper(c, &exportFlags)
	}

	// Setup the flags for this command
	export.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "output-format",
			Aliases:     []string{"o"},
			Usage:       "Format for the output [json | yaml]",
			Destination: &exportFlags.OutputFormat,
			Value:       YAMLFormat,
			EnvVars:     []string{"MIG_PARTED_OUTPUT_FORMAT"},
		},
		&cli.StringFlag{
			Name:        "config-label",
			Aliases:     []string{"l"},
			Usage:       "Label to apply to the exported config",
			Destination: &exportFlags.ConfigLabel,
			Value:       DefaultConfigLabel,
			EnvVars:     []string{"MIG_PARTED_CONFIG_LABEL"},
		},
		&cli.BoolFlag{
			Name:        "placements",
			Aliases:     []string{"p"},
			Usage:       "Output the actual placements of MIG devices",
			Destination: &exportFlags.Placements,
			Value:       false,
			EnvVars:     []string{"MIG_PARTED_SHOW_PLACEMENTS"},
		},
	}

	return &export
}

func exportWrapper(c *cli.Context, f *Flags) error {
	err := CheckFlags(f)
	if err != nil {
		cli.ShowSubcommandHelp(c)
		return err
	}

	context := Context{
		Context: c,
		Flags:   f,
	}

	if f.Placements {
		return exportPlacements(f)
	}

	spec, err := ExportMigConfigs(&context)
	if err != nil {
		return err
	}

	err = WriteOutput(os.Stdout, spec, f)
	if err != nil {
		return err
	}

	return nil
}

func CheckFlags(f *Flags) error {
	switch f.OutputFormat {
	case JSONFormat:
	case YAMLFormat:
	default:
		return fmt.Errorf("unrecognized 'output-format': %v", f.OutputFormat)
	}
	return nil
}

func WriteOutput(w io.Writer, spec interface{}, f *Flags) error {
	switch f.OutputFormat {
	case YAMLFormat:
		output, err := yaml.Marshal(spec)
		if err != nil {
			return fmt.Errorf("error unmarshaling MIG config to YAML: %v", err)
		}
		w.Write(output)
	case JSONFormat:
		output, err := json.MarshalIndent(spec, "", "  ")
		if err != nil {
			return fmt.Errorf("error unmarshaling MIG config to JSON: %v", err)
		}
		w.Write(output)
	}
	return nil
}
