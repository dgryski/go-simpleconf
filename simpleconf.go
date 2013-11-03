package simpleconf

// TODO(dgryski): handle parse errors
// TODO(dgryski): handle quoted tokens?

import (
	"bufio"
	"io"
	"log"
	"os"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

func New(r io.Reader) map[string]interface{} {
	scanner := bufio.NewScanner(r)
	m := parse(scanner, "")
	return m
}

func eatSpaces(line string) string {
	return strings.TrimLeftFunc(line, unicode.IsSpace)
}

func match(line, text string) string {
	return strings.TrimPrefix(line, text)
}

func addValue(m map[string]interface{}, key string, value interface{}) {

	// no key? add
	var mv interface{}
	var ok bool
	if mv, ok = m[key]; !ok {
		m[key] = value
		return
	}

	// string key? overwrite
	if _, ok = mv.(string); ok {
		m[key] = value
		return
	}

	// otherwise complain
	log.Fatalf("not overwriting string value for key %s with block\n", key, reflect.TypeOf(mv))
}

func merge(m map[string]interface{}, blockType, blockName string, block map[string]interface{}) {

	var blockMap map[string]interface{}

	bm, ok := m[blockType]
	if ok {
		blockMap = bm.(map[string]interface{})
	} else {
		blockMap = make(map[string]interface{})
	}

	if b, ok := blockMap[blockName]; ok {
		oldBlock := b.(map[string]interface{})
		for bk, bv := range block {
			addValue(oldBlock, bk, bv)
		}
		blockMap[blockName] = oldBlock
	} else {
		blockMap[blockName] = block
	}

	m[blockType] = blockMap
}

func parse(scanner *bufio.Scanner, blockType string) map[string]interface{} {

	m := make(map[string]interface{})

	for scanner.Scan() {
		line := scanner.Text()

		line = eatSpaces(line)

		if len(line) == 0 || line[0] == '#' {
			// blank line or comment, skip
			continue
		}

		if strings.HasPrefix(line, "include") {
			include := parseInclude(scanner, line)
			log.Println("include=", include)
			continue
			// merge m and include
		}

		if line[0] == '<' && line[1] == '/' {
			strs := closeRegex.FindStringSubmatch(line)
			if strs[1] != blockType {
				log.Fatal("unexpected closing block")
			}

			return m
		}

		if line[0] == '<' {
			blockType, blockName, block := parseBlock(scanner, line)

			merge(m, blockType, blockName, block)
			continue
		}

		// single-line config item
		k, v := parseItem(line)
		addValue(m, k, v)
	}

	return m
}

// include filename
func parseInclude(scanner *bufio.Scanner, line string) map[string]interface{} {
	line = match(line, "include")

	line = strings.TrimSpace(line)
	r, err := os.Open(line)
	if err != nil {
		log.Fatalf("can't open config file: ", err)
	}

	newscanner := bufio.NewScanner(r)
	m := parse(newscanner, "")

	return m
}

var blockRegex = regexp.MustCompile(`^\s*<\s*(\w+)\s+(.+?)\s*>\s*$`)
var closeRegex = regexp.MustCompile(`^\s*</\s*(\w+)\s*>\s*$`)

func parseBlock(scanner *bufio.Scanner, line string) (string, string, map[string]interface{}) {
	strs := blockRegex.FindStringSubmatch(line)
	blockType, blockName := strs[1], strs[2]
	m := parse(scanner, blockType)
	return blockType, blockName, m
}

var lineRegex = regexp.MustCompile(`^\s*(\w*)\s*=?\s*(.+)\s*$`)

func parseItem(line string) (string, string) {
	strs := lineRegex.FindStringSubmatch(line)
	return strs[1], strs[2]
}
