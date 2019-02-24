package reinc

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
)

const (
	Version = "0.1.0-beta"
)

type Rule struct {
	Regexp            *regexp.Regexp
	PathFormat        []byte
	Once              bool
	OnceFormat        []byte
	IgnoreError       bool
	IgnoreErrorFormat []byte
	Mode              string
}

const (
	RuleModeDefault string = ""
	RuleModeFileDir        = "filedir"
	RuleModeWorkDir        = "workdir"
)

type RuleConfig struct {
	Pattern           string `json:"pattern"`
	PathFormat        string `json:"path_format"`
	Once              bool   `json:"once"`
	OnceFormat        string `json:"once_format"`
	IgnoreError       bool   `json:"ignore_error"`
	IgnoreErrorFormat string `json:"ignore_error_format"`
	Mode              string `json:"mode"`
}

type ReplacerConfig struct {
	Rules []*RuleConfig `json:"rules"`
}

type Rules []*Rule

func NewRule(config *RuleConfig) (*Rule, error) {
	re, err := regexp.Compile(config.Pattern)
	if err != nil {
		return nil, err
	}
	return &Rule{
		Regexp:            re,
		PathFormat:        []byte(config.PathFormat),
		Once:              config.Once,
		OnceFormat:        []byte(config.OnceFormat),
		IgnoreError:       config.IgnoreError,
		IgnoreErrorFormat: []byte(config.IgnoreErrorFormat),
		Mode:              config.Mode,
	}, nil
}

type Replacer struct {
	Writer          io.Writer
	Reader          io.Reader
	Path            string
	Rules           Rules
	MaxDepth        int
	RootDir         string
	IncludedPathSet map[string]struct{}
	depth           int
}

func NewReplacer(w io.Writer, r io.Reader) *Replacer {
	repl, err := NewReplacerWithConfig(w, r, nil)
	if err != nil {
		panic(err)
	}
	return repl
}

func NewReplacerWithConfig(w io.Writer, r io.Reader, config *ReplacerConfig) (*Replacer, error) {
	rules := make(Rules, 0)
	if config != nil {
		for _, ruleConfig := range config.Rules {
			rule, err := NewRule(ruleConfig)
			if err != nil {
				return nil, err
			}
			rules = append(rules, rule)
		}
	}
	return &Replacer{
		Writer:          w,
		Reader:          r,
		Rules:           rules,
		MaxDepth:        32,
		IncludedPathSet: make(map[string]struct{}),
	}, nil
}

func (r *Replacer) includeFile(fpath string) error {
	file, err := os.Open(fpath)
	if err != nil {
		return err
	}
	defer file.Close()
	subr := &Replacer{
		Writer:          r.Writer,
		Reader:          file,
		Rules:           r.Rules,
		Path:            fpath,
		MaxDepth:        r.MaxDepth,
		RootDir:         r.RootDir,
		IncludedPathSet: r.IncludedPathSet,
		depth:           r.depth + 1,
	}
	_, err = subr.Replace()
	return err
}

func (r *Replacer) resolvePathImpl(fpath string, mode string) (string, error) {
	var err error
	if filepath.IsAbs(fpath) {
		return fpath, nil
	}
	if r.Path != "" {
		var baseDir string
		switch mode {
		case RuleModeDefault, RuleModeFileDir:
			baseDir = filepath.Dir(r.Path)
		case RuleModeWorkDir:
			baseDir, err = os.Getwd()
			if err != nil {
				return fpath, err
			}
		default:
			return fpath, fmt.Errorf("invalid mode: %s", mode)
		}
		joined := filepath.Join(baseDir, fpath)
		return filepath.Abs(joined)
	}
	return filepath.Abs(fpath)
}

func (r *Replacer) resolvePath(fpath string, mode string) (string, error) {
	fpath, err := r.resolvePathImpl(fpath, mode)
	if err != nil {
		return "", err
	}
	if r.RootDir != "" && !filepath.HasPrefix(fpath, r.RootDir) {
		return "", fmt.Errorf("banned: %s", fpath)
	}
	return fpath, nil
}

func (r *Replacer) replaceFirst(bs []byte, offset int) (bool, int, error) {
	var err error
	found := false
	minOffset := len(bs)
	minEnd := len(bs)
	var minRule *Rule
	for _, rule := range r.Rules {
		loc := rule.Regexp.FindIndex(bs[offset:])
		if loc == nil || offset+loc[0] >= minOffset {
			continue
		}
		minOffset = offset + loc[0]
		minEnd = offset + loc[1]
		minRule = rule
		found = true
	}
	if !found {
		n, err := r.Writer.Write(bs[offset:])
		return false, offset + n, err
	}
	_, err = r.Writer.Write(bs[offset:minOffset])
	if err != nil {
		return true, offset, err
	}
	target := bs[minOffset:minEnd]
	fpath := minRule.Regexp.ReplaceAll(target, minRule.PathFormat)
	resolvedFpath, err := r.resolvePath(string(fpath), minRule.Mode)
	if err == nil {
		if _, ok := r.IncludedPathSet[resolvedFpath]; ok {
			if minRule.Once || len(minRule.Regexp.ReplaceAll(target, minRule.OnceFormat)) != 0 {
				_, err = r.Writer.Write(target)
				if err != nil {
					return true, minOffset, err
				}
			}
		}
		err = r.includeFile(resolvedFpath)
		if err != nil {
			if minRule.IgnoreError || len(minRule.Regexp.ReplaceAll(target, minRule.IgnoreErrorFormat)) != 0 {
				_, err = r.Writer.Write(target)
				if err != nil {
					return true, minOffset, err
				}
			} else {
				return true, minOffset, err
			}
		}
		r.IncludedPathSet[resolvedFpath] = struct{}{}
	} else {
		if minRule.IgnoreError || len(minRule.Regexp.ReplaceAll(target, minRule.IgnoreErrorFormat)) != 0 {
			_, err = r.Writer.Write(target)
			if err != nil {
				return true, minOffset, err
			}
		} else {
			return true, minOffset, err
		}
	}
	return true, minEnd, nil
}

func (r *Replacer) Replace() (int, error) {
	if r.depth > r.MaxDepth {
		return 0, fmt.Errorf("too many levels of recursion: %d", r.depth)
	}
	var err error
	bs, err := ioutil.ReadAll(r.Reader)
	if err != nil {
		return 0, err
	}
	found := false
	offset := 0
	for {
		found, offset, err = r.replaceFirst(bs, offset)
		if err != nil {
			return offset, err
		}
		if !found {
			break
		}
	}
	return offset, nil
}
