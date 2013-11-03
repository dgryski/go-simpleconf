package simpleconf

// TODO(dgryski): handle $ENVIRONMENT replacements
// TODO(dgryski): handle quoted tokens?

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// NewFromReader loads a configuration from r
func NewFromReader(r io.Reader) (map[string]interface{}, error) {
	scanner := bufio.NewScanner(r)
	return parse(scanner, "")
}

// NewFromFile loads a configuration file
func NewFromFile(file string) (map[string]interface{}, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}

	return NewFromReader(f)
}

func addValue(m map[string]interface{}, key string, value interface{}) error {

	// no key? add
	var mv interface{}
	var ok bool
	if mv, ok = m[key]; !ok {
		m[key] = value
		return nil
	}

	// string key? overwrite
	if _, ok = mv.(string); ok {
		m[key] = value
		return nil
	}

	return fmt.Errorf("can't overwrite string value for key %s with block", key)
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

func parse(scanner *bufio.Scanner, blockType string) (map[string]interface{}, error) {

	m := make(map[string]interface{})

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimLeftFunc(line, unicode.IsSpace)

		if len(line) == 0 || line[0] == '#' {
			// blank line or comment, skip
			continue
		}

		if strings.HasPrefix(line, "include") {
			include, err := parseInclude(scanner, line)
			if err != nil {
				return nil, fmt.Errorf("error processing include %s: %s", line, err)
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
			blockType, blockName, block, err := parseBlock(scanner, line)
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
		k, v, err := parseItem(line)
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
func parseInclude(scanner *bufio.Scanner, line string) (map[string]interface{}, error) {

	file := strings.TrimSpace(strings.TrimPrefix(line, "include"))

	r, err := os.Open(file)
	if err != nil {
		return nil, err
	}

	newscanner := bufio.NewScanner(r)
	return parse(newscanner, "")
}

var blockRegex = regexp.MustCompile(`^\s*<\s*(\w+)\s*(.+?)?\s*>\s*$`) // ugly regexp :(
var closeRegex = regexp.MustCompile(`^\s*</\s*(\w+)\s*>\s*$`)

// <foo bar>
// baz qux
// </foo>
func parseBlock(scanner *bufio.Scanner, line string) (string, string, map[string]interface{}, error) {
	strs := blockRegex.FindStringSubmatch(line)

	var blockType, blockName string

	if strs == nil {
		return "", "", nil, fmt.Errorf("error parsing block header [%s]", line)
	}

	blockType, blockName = strs[1], strs[2]

	m, err := parse(scanner, blockType)
	return blockType, blockName, m, err
}

var lineRegex = regexp.MustCompile(`^\s*(\w*)(?:(?:\s+=\s+)|(?:\s+))(.+?)\s*$`)

// var = val
func parseItem(line string) (string, string, error) {
	strs := lineRegex.FindStringSubmatch(line)

	if len(strs) != 3 {
		return "", "", fmt.Errorf("error parsing line [%s]", line)
	}

	return strs[1], strs[2], nil
}
