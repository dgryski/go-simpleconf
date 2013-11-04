package simpleconf

// TODO(dgryski): handle $ENVIRONMENT replacements
// TODO(dgryski): handle quoted tokens?

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

// NewFromReader loads a configuration from r
func NewFromReader(r io.Reader) (map[string]interface{}, error) {
	scanner := bufio.NewScanner(r)
	p := &parser{scanner: scanner}
	return parse(p, "")
}

// NewFromFile loads a configuration file
func NewFromFile(file string) (map[string]interface{}, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}

	dir, _ := filepath.Split(file)

	scanner := bufio.NewScanner(f)

	p := &parser{scanner: scanner, pathList: pathlist([]string{dir})}
	return parse(p, "")
}

func addValue(m map[string]interface{}, key string, value interface{}) error {

	// no key? add
	var mv interface{}
	var ok bool
	if mv, ok = m[key]; !ok {
		m[key] = value
		return nil
	}

	// if target value is a string ...
	if _, ok = mv.(string); ok {
		if _, vok := value.(string); !vok {
			return fmt.Errorf("can't overwrite string value for key %s with %s", key, reflect.TypeOf(value))
		}

		m[key] = value
		return nil
	}

	// both blocks? merge
	if _, ok := mv.(map[string]interface{}); ok {

		var vbl map[string]interface{}
		var vok bool
		if vbl, vok = value.(map[string]interface{}); !vok {
			return fmt.Errorf("don't know how to merge block for key %s with %s\n", key, reflect.TypeOf(value))
		}

		err := merge(m, key, "", vbl)
		return err
	}

	return nil
}

func merge(m map[string]interface{}, blockType, blockName string, block map[string]interface{}) error {

	var blockMap map[string]interface{}

	bm, ok := m[blockType]
	if ok {
		blockMap, ok = bm.(map[string]interface{})
		if !ok {
			return fmt.Errorf("key type conflict while merging block(%s, %s)", blockType, blockName)
		}
	} else {
		blockMap = make(map[string]interface{})
	}

	if blockName == "" {
		for bk, bv := range block {
			err := addValue(blockMap, bk, bv)
			if err != nil {
				return err
			}
		}
	} else if b, ok := blockMap[blockName]; ok {
		oldBlock := b.(map[string]interface{})

		if !ok {
			return fmt.Errorf("internal error while merging block(%s, %s)", blockType, blockName)
		}

		for bk, bv := range block {
			err := addValue(oldBlock, bk, bv)
			if err != nil {
				return err
			}
		}
		blockMap[blockName] = oldBlock
	} else {
		blockMap[blockName] = block
	}

	m[blockType] = blockMap

	return nil
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

func parse(state *parser, blockType string) (map[string]interface{}, error) {

	m := make(map[string]interface{})

	for state.scanner.Scan() {
		line := state.scanner.Text()
		line = strings.TrimLeftFunc(line, unicode.IsSpace)

		if len(line) == 0 || line[0] == '#' {
			// blank line or comment, skip
			continue
		}

		if strings.HasPrefix(line, "include") {
			include, err := parseInclude(state, line)
			if err != nil {
				return nil, fmt.Errorf("error processing include [%s]: %s", line, err)
			}

			for k, v := range include {
				err := addValue(m, k, v)
				if err != nil {
					return nil, err
				}
			}

			continue
		}

		if line[0] == '<' && line[1] == '/' {
			strs := closeRegex.FindStringSubmatch(line)
			if strs[1] != blockType {
				return nil, fmt.Errorf("unexpected closing block while looking for %s", blockType)
			}

			return m, nil
		}

		if line[0] == '<' {
			blockType, blockName, block, err := parseBlock(state, line)
			if err != nil {
				return nil, err
			}

			err = merge(m, blockType, blockName, block)
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
		err = addValue(m, k, v)
		if err != nil {
			return nil, err
		}
	}

	return m, nil
}

// include filename
func parseInclude(state *parser, line string) (map[string]interface{}, error) {

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
var closeRegex = regexp.MustCompile(`^\s*</\s*(\w+)\s*>\s*$`)

// <foo bar>
// baz qux
// </foo>
func parseBlock(state *parser, line string) (string, string, map[string]interface{}, error) {
	strs := blockRegex.FindStringSubmatch(line)

	var blockType, blockName string

	if strs == nil {
		return "", "", nil, fmt.Errorf("error parsing block header [%s]", line)
	}

	blockType, blockName = strs[1], strs[2]

	m, err := parse(state, blockType)
	return blockType, blockName, m, err
}

var tokRegex = regexp.MustCompile(`^\s*(\S+)\s*=?\s*`)

// var = val1 val2 val3
// var val val2
// val
var heredocRegexp = regexp.MustCompile(`<<(\w+)$`)

func parseItem(state *parser, line string) (string, string, error) {

	strs := tokRegex.FindStringSubmatch(line)

	if len(strs) != 2 {
		return "", "", fmt.Errorf("error parsing line [%s]", line)
	}

	tok := strs[1]
	line = strings.TrimPrefix(line, strs[0])

	var buf bytes.Buffer

	// if we have a value, check for continuation characters and heredocs
	if len(line) > 0 {

		if line[len(line)-1] == '\\' {
			for line[len(line)-1] == '\\' && state.scanner.Scan() {
				buf.WriteString(line[:len(line)-1])
				line = state.scanner.Text()
			}
			buf.WriteString(line)
		} else if strs := heredocRegexp.FindStringSubmatch(line); len(strs) != 0 {
			nl := false
			for state.scanner.Scan() {
				line = state.scanner.Text()
				if strings.HasSuffix(line, strs[1]) {
					indent := strings.TrimSuffix(line, strs[1])
					s := strings.Replace(buf.String(), indent, "", 1)
					s = strings.Replace(s, "\n"+indent, "\n", -1)
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
			buf.WriteString(line)
		}
	}

	return tok, buf.String(), nil
}
