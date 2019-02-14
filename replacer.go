package reinc

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
)

type Rule struct {
	Regexp            *regexp.Regexp
	PathFormat        []byte
	Once              bool
	OnceFormat        []byte
	IgnoreError       bool
	IgnoreErrorFormat []byte
}

type RuleConfig struct {
	Pattern           string `json:"pattern"`
	PathFormat        string `json:"path_format"`
	Once              bool   `json:"once"`
	OnceFormat        string `json:"once_format"`
	IgnoreError       bool   `json:"ignore_error"`
	IgnoreErrorFormat string `json:"ignore_error_format"`
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
	return &Replacer{
		Writer:          w,
		Reader:          r,
		MaxDepth:        32,
		IncludedPathSet: make(map[string]struct{}),
	}
}

func (repl *Replacer) includeFile(fpath string) error {
	file, err := os.Open(fpath)
	if err != nil {
		return err
	}
	defer file.Close()
	subRepl := &Replacer{
		Writer:          repl.Writer,
		Reader:          file,
		Rules:           repl.Rules,
		Path:            fpath,
		MaxDepth:        repl.MaxDepth,
		RootDir:         repl.RootDir,
		IncludedPathSet: repl.IncludedPathSet,
		depth:           repl.depth + 1,
	}
	_, err = subRepl.Replace()
	return err
}

func (repl *Replacer) resolvePathImpl(fpath string) (string, error) {
	if filepath.IsAbs(fpath) {
		return fpath, nil
	}
	if repl.Path != "" {
		joined := filepath.Join(filepath.Dir(repl.Path), fpath)
		return filepath.Abs(joined)
	}
	return filepath.Abs(fpath)
}

func (repl *Replacer) resolvePath(fpath string) (string, error) {
	fpath, err := repl.resolvePathImpl(fpath)
	if err != nil {
		return "", err
	}
	if repl.RootDir != "" && !filepath.HasPrefix(fpath, repl.RootDir) {
		return "", fmt.Errorf("banned: %s", fpath)
	}
	return fpath, nil
}

func (repl *Replacer) replaceFirst(bs []byte, offset int) (bool, int, error) {
	var err error
	found := false
	minOffset := len(bs)
	minEnd := len(bs)
	var minRule *Rule
	for _, rule := range repl.Rules {
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
		n, err := repl.Writer.Write(bs[offset:])
		return false, offset + n, err
	}
	_, err = repl.Writer.Write(bs[offset:minOffset])
	if err != nil {
		return true, offset, err
	}
	target := bs[minOffset:minEnd]
	fpath := minRule.Regexp.ReplaceAll(target, minRule.PathFormat)
	resolvedFpath, err := repl.resolvePath(string(fpath))
	if err == nil {
		if _, ok := repl.IncludedPathSet[resolvedFpath]; ok {
			if minRule.Once || len(minRule.Regexp.ReplaceAll(target, minRule.OnceFormat)) != 0 {
				_, err = repl.Writer.Write(target)
				if err != nil {
					return true, minOffset, err
				}
			}
		}
		err = repl.includeFile(resolvedFpath)
		if err != nil {
			if minRule.IgnoreError || len(minRule.Regexp.ReplaceAll(target, minRule.IgnoreErrorFormat)) != 0 {
				_, err = repl.Writer.Write(target)
				if err != nil {
					return true, minOffset, err
				}
			} else {
				return true, minOffset, err
			}
		}
		repl.IncludedPathSet[resolvedFpath] = struct{}{}
	} else {
		if minRule.IgnoreError || len(minRule.Regexp.ReplaceAll(target, minRule.IgnoreErrorFormat)) != 0 {
			_, err = repl.Writer.Write(target)
			if err != nil {
				return true, minOffset, err
			}
		} else {
			return true, minOffset, err
		}
	}
	return true, minEnd, nil
}

func (repl *Replacer) Replace() (int, error) {
	if repl.depth > repl.MaxDepth {
		return 0, fmt.Errorf("too many levels of recursion: %d", repl.depth)
	}
	var err error
	bs, err := ioutil.ReadAll(repl.Reader)
	if err != nil {
		return 0, err
	}
	found := false
	offset := 0
	for {
		found, offset, err = repl.replaceFirst(bs, offset)
		if err != nil {
			return offset, err
		}
		if !found {
			break
		}
	}
	return offset, nil
}
