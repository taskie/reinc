package reinc

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/taskie/jc"

	"github.com/iancoleman/strcase"
	"github.com/k0kubun/pp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/taskie/osplus"
	"github.com/taskie/reinc"
)

type Config struct {
	Replacer, Preset, Output, LogLevel string
}

var configFile string
var config Config
var (
	verbose, debug, version bool
)

const CommandName = "reinc"

func init() {
	Command.PersistentFlags().StringVarP(&configFile, "config", "c", "", `config file (default "`+CommandName+`.yml")`)
	Command.Flags().StringP("replacer", "r", "reinc-replacer.yml", "replacer config file")
	Command.Flags().StringP("preset", "p", "", "preset name")
	Command.Flags().StringP("output", "o", "", "output file")
	Command.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	Command.Flags().BoolVar(&debug, "debug", false, "debug output")
	Command.Flags().BoolVarP(&version, "version", "V", false, "show Version")

	for _, s := range []string{"replacer", "preset", "output"} {
		envKey := strcase.ToSnake(s)
		structKey := strcase.ToCamel(s)
		viper.BindPFlag(envKey, Command.Flags().Lookup(s))
		viper.RegisterAlias(structKey, envKey)
	}

	cobra.OnInitialize(initConfig)
}

func initConfig() {
	if debug {
		log.SetLevel(log.DebugLevel)
	} else if verbose {
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}

	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName(CommandName)
		conf, err := osplus.GetXdgConfigHome()
		if err != nil {
			log.Info(err)
		} else {
			viper.AddConfigPath(filepath.Join(conf, CommandName))
		}
		viper.AddConfigPath(".")
	}
	viper.BindEnv("no_color")
	viper.SetEnvPrefix(CommandName)
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		log.Debug(err)
	}
	err = viper.Unmarshal(&config)
	if err != nil {
		log.Warn(err)
	}
}

func Main() {
	Command.Execute()
}

var Command = &cobra.Command{
	Use:  CommandName,
	Args: cobra.RangeArgs(0, 2),
	Run: func(cmd *cobra.Command, args []string) {
		err := run(cmd, args)
		if err != nil {
			log.Fatal(err)
		}
	},
}

func buildPreset() map[string]*reinc.ReplacerConfig {
	return map[string]*reinc.ReplacerConfig{
		"sh": &reinc.ReplacerConfig{
			Rules: []*reinc.RuleConfig{
				&reinc.RuleConfig{
					Pattern:     `(?m)^\s*(?:\.|source)\s+"([^"]+)"\s*$`,
					PathFormat:  "$1",
					IgnoreError: true,
					Mode:        reinc.RuleModeWorkDir,
				},
				&reinc.RuleConfig{
					Pattern:     `(?m)^\s*(?:\.|source)\s+'([^']+)'\s*$`,
					PathFormat:  "$1",
					IgnoreError: true,
					Mode:        reinc.RuleModeWorkDir,
				},
				&reinc.RuleConfig{
					Pattern:     `(?m)^\s*(?:\.|source)\s+(\S+)\s*$`,
					PathFormat:  "$1",
					IgnoreError: true,
					Mode:        reinc.RuleModeWorkDir,
				},
			},
		},
		"c": &reinc.ReplacerConfig{
			Rules: []*reinc.RuleConfig{
				&reinc.RuleConfig{
					Pattern:     `(?m)^#\s*(?:include)\s+"([^"]+)"\s*$`,
					PathFormat:  "$1",
					IgnoreError: true,
					Mode:        reinc.RuleModeFileDir,
				},
			},
		},
	}
}

var preset = buildPreset()

func process(w io.Writer, opener *osplus.Opener, fpath string, replacerConfig *reinc.ReplacerConfig) error {
	r, err := opener.Open(fpath)
	if err != nil {
		return err
	}
	defer r.Close()
	repl, err := reinc.NewReplacerWithConfig(w, r, replacerConfig)
	if err != nil {
		return err
	}
	_, err = repl.Replace()
	return err
}

func run(cmd *cobra.Command, args []string) error {
	if version {
		fmt.Println(reinc.Version)
		return nil
	}
	if config.LogLevel != "" {
		lv, err := log.ParseLevel(config.LogLevel)
		if err != nil {
			log.Warn(err)
		} else {
			log.SetLevel(lv)
		}
	}
	if debug {
		if viper.ConfigFileUsed() != "" {
			log.Debugf("Using config file: %s", viper.ConfigFileUsed())
		}
		log.Debug(pp.Sprint(config))
	}

	var replacerConfig reinc.ReplacerConfig
	if cmd.Flags().Changed("replacer") {
		err := jc.DecodeFile(config.Replacer, "", &replacerConfig)
		if err != nil {
			return err
		}
	} else if config.Preset != "" {
		pReplacerConfig := preset[config.Preset]
		if pReplacerConfig == nil {
			return fmt.Errorf("no such preset: %s", config.Preset)
		}
		replacerConfig = *pReplacerConfig
	} else {
		err := jc.DecodeFile(config.Replacer, "", &replacerConfig)
		if err != nil {
			return err
		}
	}

	opener := osplus.NewOpener()
	w, commit, err := opener.CreateTempFileWithDestination(config.Output, "", CommandName+"-")
	if err != nil {
		return err
	}
	defer w.Close()

	for _, arg := range args {
		err := process(w, opener, arg, &replacerConfig)
		if err != nil {
			return err
		}
	}

	commit(true)
	return nil
}
