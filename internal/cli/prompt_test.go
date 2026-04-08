package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsInteractiveTTY(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  *bytes.Buffer
		output *bytes.Buffer
		want   bool
	}{
		{name: "non-tty readers", input: bytes.NewBufferString(""), output: bytes.NewBufferString(""), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, isInteractiveTTY(tc.input, tc.output))
		})
	}
}

func TestMapOptions(t *testing.T) {
	t.Parallel()

	options := []string{"first", "second", "third"}
	mapped := mapOptions(options)

	require.Len(t, mapped, 3)
	assert.Equal(t, "first", mapped[0].Key)
	assert.Equal(t, 0, mapped[0].Value)
	assert.Equal(t, "third", mapped[2].Key)
	assert.Equal(t, 2, mapped[2].Value)
}

func TestPrompter_ChooseIndex(t *testing.T) {
	t.Parallel()

	t.Run("returns error with no options", func(t *testing.T) {
		t.Parallel()

		p := newPrompter(bytes.NewBufferString(""), bytes.NewBufferString(""))
		index, err := p.chooseIndex("Choice", nil)

		require.Error(t, err)
		assert.Equal(t, -1, index)
		assert.ErrorIs(t, err, errNoOptionsAvailable)
	})
}

func TestSelectDescription(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Use arrows and Enter.", selectDescription(false))
	assert.Equal(t, "Use arrows and Enter. Press / to filter.", selectDescription(true))
}
