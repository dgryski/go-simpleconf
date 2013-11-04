package simpleconf

import (
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
optional = equalsign
desc long line with \
continuation character
longdesc <<EOT
really long description
with extra stuff
EOT
`,
		map[string]interface{}{
			"foo":      "bar",
			"baz":      "qux",
			"optional": "equalsign",
			"desc":     "long line with continuation character",
			"longdesc": "really long description with\nextra stuff",
		},
	},
	{
		// arrays
		`
entry1
entry2
entry3
`,
		map[string]interface{}{"entry1": "", "entry2": "", "entry3": ""},
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
			"array": map[string]interface{}{"entry1": "", "entry2": "", "entry3": ""},
		},
	},
	{
		// kv blocks
		`
<dir dir1>
foo1 bar1
baz1 qux1
</dir>

<dir dir2>
foo2 bar2
baz2 qux2
<file file1>
perms 0700
</file>
</dir>
`,
		map[string]interface{}{
			"dir": map[string]interface{}{
				"dir1": map[string]interface{}{
					"foo1": "bar1",
					"baz1": "qux1",
				},
				"dir2": map[string]interface{}{
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
		if err != nil || !reflect.DeepEqual(m, tt.output) {
			jg, _ := json.MarshalIndent(m, "", "  ")
			je, _ := json.MarshalIndent(tt.output, "", "  ")
			t.Errorf("failed test %d: got\n%s\nexpected\n%s\n(err=%s)\n", i, string(jg), string(je), err)
		}
	}
}
