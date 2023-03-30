package clibase_test

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/cli/clibase"
)

func TestOptionSet_ToYAML(t *testing.T) {
	t.Parallel()

	t.Run("RequireKey", func(t *testing.T) {
		t.Parallel()
		var workspaceName clibase.String
		os := clibase.OptionSet{
			clibase.Option{
				Name:    "Workspace Name",
				Value:   &workspaceName,
				Default: "billie",
			},
		}

		node, err := os.ToYAML()
		require.NoError(t, err)
		require.Len(t, node.Content, 0)
	})

	t.Run("SimpleString", func(t *testing.T) {
		t.Parallel()

		var workspaceName clibase.String

		os := clibase.OptionSet{
			clibase.Option{
				Name:        "Workspace Name",
				Value:       &workspaceName,
				Default:     "billie",
				Description: "The workspace's name.",
				Group:       &clibase.Group{Name: "Names"},
				YAML:        "workspaceName",
			},
		}

		err := os.SetDefaults()
		require.NoError(t, err)

		n, err := os.ToYAML()
		require.NoError(t, err)
		// Visually inspect for now.
		byt, err := yaml.Marshal(n)
		require.NoError(t, err)
		t.Logf("Raw YAML:\n%s", string(byt))
	})
}

func TestOptionSet_YAMLIsomorphism(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		os        clibase.OptionSet
		zeroValue func() pflag.Value
	}{
		{
			name: "SimpleString",
			os: clibase.OptionSet{
				{
					Name:        "Workspace Name",
					Default:     "billie",
					Description: "The workspace's name.",
					Group:       &clibase.Group{YAMLName: "names"},
					YAML:        "workspaceName",
				},
			},
			zeroValue: func() pflag.Value {
				return clibase.StringOf(new(string))
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for i := range tc.os {
				tc.os[i].Value = tc.zeroValue()
			}
			err := tc.os.SetDefaults()
			require.NoError(t, err)

			y, err := tc.os.ToYAML()
			require.NoError(t, err)

			toByt, err := yaml.Marshal(y)
			require.NoError(t, err)

			t.Logf("Raw YAML:\n%s", string(toByt))

			var y2 yaml.Node
			err = yaml.Unmarshal(toByt, &y2)
			require.NoError(t, err)

			os2 := slices.Clone(tc.os)
			for i := range os2 {
				os2[i].Value = tc.zeroValue()
			}

			// os2 values should be zeroed whereas tc.os should be
			// set to defaults.
			// This makes sure we aren't mixing pointers.
			require.NotEqual(t, tc.os, os2)
			err = os2.FromYAML(&y2)
			require.NoError(t, err)

			require.Equal(t, tc.os, os2)
		})
	}
}
