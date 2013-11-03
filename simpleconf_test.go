package simpleconf

import (
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
zot = frob
`,
		map[string]interface{}{
			"foo": "bar",
			"baz": "qux",
			"zot": "frob",
		},
	},
	{
		// blocks
		`
<dir dir1>
foo1 bar1
baz1 qux1
</dir>

<dir dir2>
foo2 bar2
baz2 qux2
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
				},
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
}

func TestReadConfig(t *testing.T) {

	for i, tt := range tests {
		r := strings.NewReader(tt.input)
		m := New(r)
		if !reflect.DeepEqual(m, tt.output) {
			t.Errorf("failed test %d: got %#v expected %#v\n", i, m, tt.output)
		}
	}
}
