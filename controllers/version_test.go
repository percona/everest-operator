package controllers

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func TestVersion(t *testing.T) {
	t.Parallel()
	v, err := NewVersion("1.11.0")
	require.NoError(t, err)

	assert.Equal(t, "1.11.0", v.ToCRVersion())
	assert.Equal(t, "1.11.0", v.String())
	assert.Equal(t, "v1.11.0", v.ToSemver())
	assert.Equal(t, "pxc.percona.com/v1-11-0", v.ToAPIVersion("pxc.percona.com"))
}
