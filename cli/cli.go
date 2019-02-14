package cli

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/taskie/jc"
	"github.com/taskie/osplus"
	"github.com/taskie/reinc"
)

func buildPreset() map[string]reinc.Rules {
	panicIfErr := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	sh1, err := reinc.NewRule(&reinc.RuleConfig{
		Pattern:     `(?m)^\s*(?:\.|source)\s+"([^"]+)"\s*$`,
		PathFormat:  "$1",
		IgnoreError: true,
		Mode:        reinc.RuleModeWorkDir,
	})
	panicIfErr(err)
	sh2, err := reinc.NewRule(&reinc.RuleConfig{
		Pattern:     `(?m)^\s*(?:\.|source)\s+'([^']+)'\s*$`,
		PathFormat:  "$1",
		IgnoreError: true,
		Mode:        reinc.RuleModeWorkDir,
	})
	panicIfErr(err)
	sh3, err := reinc.NewRule(&reinc.RuleConfig{
		Pattern:     `(?m)^\s*(?:\.|source)\s+(\S+)\s*$`,
		PathFormat:  "$1",
		IgnoreError: true,
		Mode:        reinc.RuleModeWorkDir,
	})
	panicIfErr(err)
	return map[string]reinc.Rules{
		"sh": reinc.Rules{sh1, sh2, sh3},
	}
}

var preset = buildPreset()

func mainImpl() error {
	rulesPath := flag.String("rules", "", "rules file")
	presetName := flag.String("preset", "", "preset name (sh)")
	flag.Parse()
	args := flag.Args()
	var rc io.ReadCloser
	var err error
	path := ""
	if len(args) == 0 {
		rc, err = osplus.NewOpener().Open(path)
	} else if len(args) == 1 {
		if path != "-" {
			path = args[0]
		}
		rc, err = osplus.NewOpener().Open(args[0])
	} else {
		err = fmt.Errorf("invalid number of arguments")
	}
	if err != nil {
		return err
	}
	repl := reinc.NewReplacer(os.Stdout, rc)
	repl.Path = path
	if *rulesPath != "" {
		var rules reinc.Rules
		err := jc.DecodeFile(*rulesPath, "", &rules)
		if err != nil {
			return err
		}
		repl.Rules = rules
	} else if *presetName != "" {
		if v, ok := preset[*presetName]; ok {
			repl.Rules = v
		} else {
			return fmt.Errorf("preset is not found: %s", *presetName)
		}
	}
	_, err = repl.Replace()
	return err
}

func Main() {
	err := mainImpl()
	if err != nil {
		log.Fatal(err)
	}
}
