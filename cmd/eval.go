// Copyright 2018 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	myUtil "github.com/Honyrik/opa-go-service/util"
	"github.com/open-policy-agent/opa/loader"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/util"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/client-go/util/jsonpath"
)

const stringType = "string"

type repeatedStringFlag struct {
	v     []string
	isSet bool
}

func newrepeatedStringFlag(val []string) repeatedStringFlag {
	return repeatedStringFlag{
		v:     val,
		isSet: false,
	}
}

func (f *repeatedStringFlag) Type() string {
	return stringType
}

func (f *repeatedStringFlag) String() string {
	return strings.Join(f.v, ",")
}

func (f *repeatedStringFlag) Set(s string) error {
	f.v = append(f.v, s)
	f.isSet = true
	return nil
}

func (f *repeatedStringFlag) isFlagSet() bool {
	return f.isSet
}

type evalCommandParams struct {
	dataPaths  repeatedStringFlag
	inputPath  string
	resultPath string
	stdin      bool
	stdinInput bool
}

func validateEvalParams(p *evalCommandParams, cmdArgs []string) error {
	if len(cmdArgs) > 0 && p.stdin {
		return errors.New("specify query argument or --stdin but not both")
	} else if len(cmdArgs) == 0 && !p.stdin {
		return errors.New("specify query argument or --stdin")
	} else if len(cmdArgs) > 1 {
		return errors.New("specify at most one query argument")
	}
	if p.stdin && p.stdinInput {
		return errors.New("specify --stdin or --stdin-input but not both")
	}
	if p.stdinInput && p.inputPath != "" {
		return errors.New("specify --stdin-input or --input but not both")
	}

	return nil
}

type loaderFilter struct {
	Ignore []string
}

func (f loaderFilter) Apply(abspath string, info os.FileInfo, depth int) bool {
	for _, s := range f.Ignore {
		if loader.GlobExcludeName(s, 1)(abspath, info, depth) {
			return true
		}
	}
	return false
}

func init() {

	params := evalCommandParams{}

	evalCommand := &cobra.Command{
		Use:   "eval <query>",
		Short: "Evaluate a Rego query",
		Long: `Evaluate a Rego query and print the result.

Examples
--------

To evaluate a simple query:

    $ opa eval 'x := 1; y := 2; x < y'

To evaluate a query against JSON data:

    $ opa eval --data data.json 'name := data.names[_]'

To evaluate a query against JSON data supplied with a file:// URL:

    $ opa eval --data file:///path/to/file.json 'data'

`,

		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validateEvalParams(&params, args)
		},
		Run: func(cmd *cobra.Command, args []string) {

			_, err := eval(args, params, os.Stdout)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		},
	}
	// Shared flags

	addDataFlag(evalCommand.Flags(), &params.dataPaths)
	addInputFlag(evalCommand.Flags(), &params.inputPath)
	addResultPathFlag(evalCommand.Flags(), &params.resultPath)
	addQueryStdinFlag(evalCommand.Flags(), &params.stdin)
	addInputStdinFlag(evalCommand.Flags(), &params.stdinInput)

	RootCommand.AddCommand(evalCommand)
}

func addDataFlag(fs *pflag.FlagSet, paths *repeatedStringFlag) {
	fs.VarP(paths, "data", "d", "set policy or data file(s). This flag can be repeated.")
}

func addInputFlag(fs *pflag.FlagSet, inputPath *string) {
	fs.StringVarP(inputPath, "input", "i", "", "set input file path")
}

func addResultPathFlag(fs *pflag.FlagSet, resultPath *string) {
	fs.StringVarP(resultPath, "resultPath", "r", "", "set result json path")
}

func addQueryStdinFlag(fs *pflag.FlagSet, stdin *bool) {
	fs.BoolVarP(stdin, "stdin", "", false, "read query from stdin")
}

func addInputStdinFlag(fs *pflag.FlagSet, stdinInput *bool) {
	fs.BoolVarP(stdinInput, "stdin-input", "I", false, "read input document from stdin")
}

func readInputBytes(params evalCommandParams) ([]byte, error) {
	if params.stdinInput {
		return io.ReadAll(os.Stdin)
	} else if params.inputPath != "" {
		return os.ReadFile(params.inputPath)
	}
	return nil, nil
}

func eval(args []string, params evalCommandParams, w io.Writer) (bool, error) {

	var query string

	ctx := context.Background()

	if params.stdin {
		bs, err := io.ReadAll(os.Stdin)
		if err != nil {
			return false, err
		}
		query = string(bs)
	} else {
		query = args[0]
	}

	regoArgs := []func(*rego.Rego){rego.Query(query)}

	if len(params.dataPaths.v) > 0 {
		f := loaderFilter{
			Ignore: []string{""},
		}

		regoArgs = append(regoArgs, rego.Load(params.dataPaths.v, f.Apply))
	}

	evalArgs := []rego.EvalOption{
		rego.EvalRuleIndexing(true),
		rego.EvalEarlyExit(true),
	}

	inputBytes, err := readInputBytes(params)
	if err != nil {
		return false, err
	}
	if inputBytes != nil {
		var input interface{}
		err := util.Unmarshal(inputBytes, &input)
		if err != nil {
			return false, fmt.Errorf("unable to parse input: %s", err.Error())
		}
		evalArgs = append(evalArgs, rego.EvalInput(input))
	}

	r := rego.New(regoArgs...)

	var pq rego.PreparedEvalQuery
	pq, resultErr := r.PrepareForEval(ctx)
	if resultErr != nil {
		return false, resultErr
	}

	result, resultErr := pq.Eval(ctx, evalArgs...)
	if resultErr != nil {
		return false, resultErr
	}

	if params.resultPath != "" {
		parse := jsonpath.New("")
		parse.EnableJSONOutput(false)
		resultPathErr := parse.Parse(params.resultPath)
		if resultPathErr != nil {
			return false, resultPathErr
		}
		res := myUtil.ResultSetTArrayMap(result)
		printErr := parse.Execute(w, res)
		if printErr != nil {
			return false, printErr
		}
		return true, nil
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	res := myUtil.ResultSetTArrayMap(result)
	resultJSONErr := encoder.Encode(res)
	if resultJSONErr != nil {
		return false, resultJSONErr
	}

	return true, nil
}
