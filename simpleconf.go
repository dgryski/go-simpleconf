// Package simpleconf parses a subset of perl's Config::General module
package simpleconf

// TODO(dgryski): handle $ENVIRONMENT replacements
// TODO(dgryski): handle quoted tokens?

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

// Config is the base interface for all configuration values
type Config interface {
	insert(string, Config) error
}

// Str is a string configuration value
type Str string

func (c Str) insert(string, Config) error { panic("can't add string to string") }

// KV is a set of key-value configuration items
type KV map[string]Config

// List is a list of configuration values
type List []string

func (c *List) insert(key string, value Config) error {

	switch cc := value.(type) {

	case Str:
		if len(cc) > 0 {
			return fmt.Errorf("can't append k/v pairs to array list")
		}
		*c = append(*c, string(key))

	case *List:
		*c = append(*c, *cc...)

	default:
		return fmt.Errorf("don't know how to append non-string to config list")
	}

	return nil

}

// NewFromReader loads a configuration from r
func NewFromReader(r io.Reader) (Config, error) {
	scanner := bufio.NewScanner(r)
	p := &parser{scanner: scanner}
	return parse(p, "")
}

// NewFromFile loads a configuration file
func NewFromFile(file string) (Config, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dir, _ := filepath.Split(file)

	scanner := bufio.NewScanner(f)

	p := &parser{scanner: scanner, pathList: pathlist([]string{dir})}
	return parse(p, "")
}

func (c KV) insert(key string, value Config) error {

	// no key? add
	var mv interface{}
	var ok bool
	if mv, ok = c[key]; !ok {
		c[key] = value
		return nil
	}

	if vstr, vok := value.(Str); vok {

		switch mm := mv.(type) {
		case Str:
			strs := List([]string{string(mm), string(vstr)})
			c[key] = &strs
		case *List:
			*mm = append(*mm, string(vstr))
			clist := List(*mm)
			c[key] = &clist
		default:
			return fmt.Errorf("bad type for string append: %s", reflect.TypeOf(mm))
		}

		return nil
	}

	return fmt.Errorf("bad type for map insert: %s", reflect.TypeOf(value))
}

func (c KV) update(key string, value Config) error {

	// no key? add
	var mv interface{}
	var ok bool
	if mv, ok = c[key]; !ok {
		c[key] = value
		return nil
	}

	// if target value is a string ...
	if _, ok := mv.(Str); ok {
		if _, vok := value.(Str); !vok {
			return fmt.Errorf("can't overwrite string value for key %s with %s", key, reflect.TypeOf(value))
		}

		c[key] = value
		return nil
	}

	// both blocks? merge
	if _, ok := mv.(KV); ok {
		if value == nil {
			// nothing to do
			return nil
		}
		var vbl KV
		var vok bool
		if vbl, vok = value.(KV); !vok {
			return fmt.Errorf("don't know how to merge block for key %s with %s\n", key, reflect.TypeOf(value))
		}

		err := c.merge(key, "", vbl)
		return err
	}

	return fmt.Errorf("bad type for configKV.add(%s): %s", key, reflect.TypeOf(value))
}

func (c KV) merge(blockType, blockName string, block Config) error {

	// try to get target
	target, ok := c[blockType]
	if !ok {
		// target doesn't exist -- just overwrite
		if blockName == "" {
			c[blockType] = block
		} else {
			c[blockType] = KV(map[string]Config{blockName: block})
		}
		return nil
	}

	// make sure target is a map
	targetkv, ok := target.(KV)
	if !ok {
		return fmt.Errorf("key type conflict while merging block(%s, %s)", blockType, blockName)
	}

	// merge unnamed block
	if blockName == "" {
		if bb, ok := block.(KV); ok {
			for bk, bv := range bb {
				err := targetkv.update(bk, bv)
				if err != nil {
					return err
				}
			}
			return nil
		}

		// FIXME(dgryski): better error message here
		return fmt.Errorf("key type conflict while merging unnamed block(%s, %s)", blockType, blockName)
	}

	// no config for this name?  just assign
	var subtarget Config
	if subtarget, ok = targetkv[blockName]; !ok {
		targetkv[blockName] = block
		return nil
	}

	subtargetkv, ok := subtarget.(KV)
	if !ok {
		return fmt.Errorf("key type conflict while merging block(%s, %s)", blockType, blockName)
	}

	// recursively add all keys from block into subtarget
	if bkv, ok := block.(KV); ok {
		for bk, bv := range bkv {
			err := subtargetkv.update(bk, bv)
			if err != nil {
				return err
			}
		}
		return nil
	}

	return fmt.Errorf("unknown block type for merge for block(%s, %s): %s", blockType, blockName, reflect.TypeOf(block))
}

type parser struct {
	scanner  *bufio.Scanner
	pathList pathlist
}

// array-based stack
type pathlist []string

func (p *pathlist) push(s string) {
	*p = append(*p, s)
}

func (p *pathlist) pop() string {
	l := len(*p) - 1
	n := (*p)[l]
	(*p) = (*p)[:l]
	return n
}

var commentRegexp = regexp.MustCompile(`[^\\]#.*$`)

func parse(state *parser, blockType string) (Config, error) {

	var m Config

	for state.scanner.Scan() {
		line := state.scanner.Text()
		line = strings.TrimLeftFunc(line, unicode.IsSpace)

		line = commentRegexp.ReplaceAllString(line, "")

		if len(line) == 0 || line[0] == '#' {
			// blank line or comment, skip
			continue
		}

		if strings.HasPrefix(line, "include") {
			include, err := parseInclude(state, line)
			if err != nil {
				return nil, fmt.Errorf("error processing include [%s]: %s", line, err)
			}

			if include == nil {
				// nothing to do
				continue
			}

			incKV := include.(KV)

			if m == nil {
				m = KV{}
			}

			mkv, ok := m.(KV)
			if !ok {
				return nil, fmt.Errorf("can't merge include into non-k/v config section")
			}

			for k, v := range incKV {
				err := mkv.update(k, v)
				if err != nil {
					return nil, err
				}
			}

			continue
		}

		if line[0] == '<' && line[1] == '/' {
			strs := closeRegex.FindStringSubmatch(line)
			if len(strs) == 0 || strs[1] != blockType {
				return nil, fmt.Errorf("unexpected closing block while looking for %s", blockType)
			}

			return m, nil
		}

		if line[0] == '<' {
			blockType, blockName, block, err := parseBlock(state, line)
			if err != nil {
				return nil, err
			}

			if m == nil {
				m = KV{}
			}

			mkv, ok := m.(KV)
			if !ok {
				return nil, fmt.Errorf("can't merge (%s,%s) into non-k/v config section", blockType, blockName)
			}

			err = mkv.merge(blockType, blockName, block)
			if err != nil {
				return nil, err
			}

			continue
		}

		// single-line config item
		k, v, err := parseItem(state, line)
		if err != nil {
			return nil, err
		}

		if m == nil {
			if v != "" {
				m = KV{}
			} else {
				m = &List{}
			}

		}

		cstr := Str(v)

		err = m.insert(k, cstr)
		if err != nil {
			return nil, err
		}
	}

	return m, nil
}

// include filename
func parseInclude(state *parser, line string) (Config, error) {

	file := strings.TrimSpace(strings.TrimPrefix(line, "include"))

	var r *os.File
	var fullpath string
	if file[0] == '/' {
		// absolute path
		var err error
		r, err = os.Open(file)
		if err != nil {
			return nil, err
		}
		fullpath = file
	} else {
		for i := len(state.pathList) - 1; i >= 0; i-- {
			dir := state.pathList[i]
			var err error
			fullpath = dir + "/" + file
			r, err = os.Open(fullpath)
			if err == nil {
				break
			}
		}

		if r == nil {
			// not found :(
			return nil, fmt.Errorf("could not find %s in search path [%v]", file, state.pathList)
		}
	}

	dir, _ := filepath.Split(fullpath)
	state.pathList.push(dir)

	newscanner := bufio.NewScanner(r)
	p := &parser{scanner: newscanner, pathList: state.pathList}

	m, err := parse(p, "")

	state.pathList.pop()

	return m, err
}

var blockRegex = regexp.MustCompile(`^\s*<\s*(\w+)\s*(.+?)?\s*>\s*$`) // ugly regexp :(
// note the extra (?:\w+\s*)? -- this is needed because it is a common mistake to
// do this:
// <foo bar>
//   baz
// </foo bar>
//
// With an extraneous 'bar' after the closing </foo>.  Correctness says
// we should reject this, but most simpleconf parsers (e.g. Perl's) just
// silently accept it.
var closeRegex = regexp.MustCompile(`^\s*</\s*(\w+)\s*(?:\w+\s*)?>\s*$`)

// <foo bar>
// baz qux
// </foo>
func parseBlock(state *parser, line string) (string, string, Config, error) {
	strs := blockRegex.FindStringSubmatch(line)

	var blockType, blockName string

	if strs == nil {
		return "", "", nil, fmt.Errorf("error parsing block header [%s]", line)
	}

	blockType, blockName = strs[1], strs[2]

	m, err := parse(state, blockType)
	return strings.ToLower(blockType), blockName, m, err
}

var keyRegex = regexp.MustCompile(`(\s*=\s*|\s+)`)

// var = val1 val2 val3
// var val val2
// val
var heredocRegexp = regexp.MustCompile(`<<(\w+)$`)

func parseItem(state *parser, line string) (string, string, error) {

	strs := keyRegex.Split(line, 2)

	if len(strs) == 0 {
		return "", "", fmt.Errorf("error parsing line [%s]", line)
	}

	if len(strs) == 1 {
		// just a key
		return strings.ToLower(strs[0]), "", nil
	}

	key := strings.ToLower(strs[0])
	line = strs[1]

	var buf bytes.Buffer

	// if we have a value, check for continuation characters and heredocs
	if len(line) > 0 {

		if line[len(line)-1] == '\\' {
			for line[len(line)-1] == '\\' && state.scanner.Scan() {
				buf.WriteString(line[:len(line)-1])
				line = state.scanner.Text()
				// remove leading space for continued lines
				line = strings.TrimLeftFunc(line, unicode.IsSpace)
			}
			line = strings.TrimRightFunc(line, unicode.IsSpace)
			buf.WriteString(line)
		} else if strs := heredocRegexp.FindStringSubmatch(line); len(strs) != 0 {
			nl := false
			for state.scanner.Scan() {
				line = state.scanner.Text()
				if strings.HasSuffix(line, strs[1]) {
					indent := strings.TrimSuffix(line, strs[1])
					s := strings.TrimPrefix(buf.String(), indent)
					s = strings.Replace(s, "\n"+indent, "\n", -1)
					s = strings.TrimRightFunc(s, unicode.IsSpace)
					buf.Reset()
					buf.WriteString(s)
					break
				}
				if nl {
					buf.WriteByte('\n')
				}
				buf.WriteString(line)
				nl = true
			}
		} else {
			if line[0] == '"' && line[len(line)-1] == '"' {
				// remove the quotes
				line = line[1 : len(line)-1]
			}
			// nope, our line is just our value,
			// trim trailing spaces
			line = strings.TrimSpace(line)
			buf.WriteString(line)
		}
	}

	val := buf.String()

	if strings.ContainsRune(val, '\\') {
		val = strings.Replace(val, `\#`, `#`, -1)
		val = strings.Replace(val, `\$`, `$`, -1)
	}

	switch val {
	case "yes", "true":
		val = "1"
	case "no", "false":
		val = "0"
	}

	return key, val, nil
}

func UnmarshalConfig(conf Config, key string, v interface{}) error {

	if key != "" {

		keys := strings.Split(key, ">")

		for _, k := range keys {
			kv, ok := conf.(KV)
			if !ok {
				return errors.New("not a simpeconf.KV for key=" + k)
			}
			conf = kv[k]
		}
	}

	// easiest way to get our configuration data into the structure is to go via JSON
	js, _ := json.Marshal(conf)
	return json.Unmarshal(js, v)
}
