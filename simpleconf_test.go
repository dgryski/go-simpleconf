package simpleconf

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

var tests = []struct {
	input  string
	output interface{}
}{
	{
		// basics
		`
# this is a comment, followed by a blank line

foo bar
baz qux
yes yes
no no
t true
f false
valuewith trailing spaces     
optional = equalsign
equals= nospace
UPPERCASE lowercase
desc long line with \
    at least two \
    continuation characters \
    trailing space 
longdesc <<EOT
really long description
with extra stuff
EOT
   indenteddesc <<EOT
   a thing which is
   indented a bit
   trailing space 
   EOT
quoted = "a quoted string\nwith\nignored escapes" #comment
notquoted1 = string with "quotes"
notquoted2 = "string with no trailing quote
phash str\#foo
pdollar str\$foo
`,
		map[string]interface{}{
			"foo":          "bar",
			"baz":          "qux",
			"optional":     "equalsign",
			"valuewith":    "trailing spaces",
			"uppercase":    "lowercase",
			"equals":       "nospace",
			"desc":         "long line with at least two continuation characters trailing space",
			"longdesc":     "really long description\nwith extra stuff",
			"indenteddesc": "a thing which is\nindented a bit\ntrailing space",
			"yes":          "1",
			"no":           "0",
			"t":            "1",
			"f":            "0",
			"quoted":       `a quoted string\nwith\nignored escapes`,
			"phash":        "str#foo",
			"pdollar":      "str$foo",
			"notquoted1":   `string with "quotes"`,
			"notquoted2":   `"string with no trailing quote`,
		},
	},
	{
		// arrays
		`
entry1
entry2
entry3
		`,
		[]string{"entry1", "entry2", "entry3"},
	},

	{
		// array block
		`
<array>
    entry1
    entry2
    entry3
</array>
`,
		map[string]interface{}{
			"array": []string{"entry1", "entry2", "entry3"},
		},
	},
	{
		// k/v array block
		`
<array>
    entry entry1
    entry entry2
    entry entry3
</array>
`,
		map[string]interface{}{
			"array": map[string]interface{}{
				"entry": []string{"entry1", "entry2", "entry3"},
			},
		},
	},
	{
		// kv blocks
		`
<dir dir1> # ignore this comment
foo1 bar1 # this one too
baz1 qux1
</dir>

<Dir Dir2>
foo2 bar2
baz2 qux2
<file file1>
perms 0700
</file>
</Dir>
`,

		map[string]interface{}{
			"dir": map[string]interface{}{
				"dir1": map[string]interface{}{
					"foo1": "bar1",
					"baz1": "qux1",
				},
				"Dir2": map[string]interface{}{
					"foo2": "bar2",
					"baz2": "qux2",
					"file": map[string]interface{}{
						"file1": map[string]interface{}{
							"perms": "0700",
						},
					},
				},
			},
		},
	},
	{
		// kv blocks with extra data after the closing directive
		`
<dir dir1> # ignore this comment
foo1 bar1 # this one too
baz1 qux1
</dir dir1>

<Dir Dir2>
foo2 bar2
baz2 qux2
<file file1>
perms 0700
</file filebar>
</Dir Dir2>
`,

		map[string]interface{}{
			"dir": map[string]interface{}{
				"dir1": map[string]interface{}{
					"foo1": "bar1",
					"baz1": "qux1",
				},
				"Dir2": map[string]interface{}{
					"foo2": "bar2",
					"baz2": "qux2",
					"file": map[string]interface{}{
						"file1": map[string]interface{}{
							"perms": "0700",
						},
					},
				},
			},
		},
	},
	{
		// unnamed blocks
		`
<dir>
foo1 bar1
baz1 qux1
</dir>

<dir>
foo1 bar2
baz2 qux2
</dir>
`,
		map[string]interface{}{
			"dir": map[string]interface{}{
				"foo1": "bar2",
				"baz1": "qux1",
				"baz2": "qux2",
			},
		},
	},

	{
		// block merging, overwrite and add
		`
<dir dir1>
foo1 bar1
baz1 qux1
</dir>

<dir dir1>
foo1 bar2
baz2 qux2
</dir>
`,
		map[string]interface{}{
			"dir": map[string]interface{}{
				"dir1": map[string]interface{}{
					"foo1": "bar2",
					"baz1": "qux1",
					"baz2": "qux2",
				},
			},
		},
	},
	{
		// merging nested blocks
		`
<dir dir1>
<file file1>
foo1 bar1
baz1 qux1
</file>
</dir>

<dir dir1>
<file file1>
foo1 bar2
baz2 qux2
</file>
</dir>
`,
		map[string]interface{}{
			"dir": map[string]interface{}{
				"dir1": map[string]interface{}{
					"file": map[string]interface{}{
						"file1": map[string]interface{}{
							"foo1": "bar2",
							"baz1": "qux1",
							"baz2": "qux2",
						},
					},
				},
			},
		},
	},
}

func TestReadConfig(t *testing.T) {

	for i, tt := range tests {
		r := strings.NewReader(tt.input)
		m, err := NewFromReader(r)

		jg, _ := json.MarshalIndent(m, "", "  ")
		je, _ := json.MarshalIndent(tt.output, "", "  ")

		if err != nil || !bytes.Equal(jg, je) {
			t.Errorf("failed test %d: got\n%s\nexpected\n%s\n(err=%s)\n", i, string(jg), string(je), err)
		}
	}
}

func TestUnmarshalConfig(t *testing.T) {

	var tests = []struct {
		input  string
		path   string
		dst    []string
		output []string
	}{
		{`
<map>
    <array>
        entry entry1
        entry entry2
        entry entry3
    </array>
</map>
`,

			"map>array>entry",
			[]string{},
			[]string{"entry1", "entry2", "entry3"},
		},
	}

	for _, tt := range tests {

		r := strings.NewReader(tt.input)
		m, _ := NewFromReader(r)

		UnmarshalConfig(m, tt.path, &tt.dst)

		if !reflect.DeepEqual(tt.output, tt.dst) {
			t.Errorf("Unmarshal(%q,...)=%#v, want %#v", tt.path, tt.dst, tt.output)
		}
	}
}
