package nfigure

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvArrayPtr1(t *testing.T) {
	require.NoError(t, os.Setenv("X", "382:32"), "set X")
	var testData struct {
		X *[]int `env:"X,split=:"`
	}
	registry := NewRegistry()
	require.NoError(t, registry.Request(&testData), "add model")
	err := registry.Configure()
	require.NoError(t, err, "configure")
	want := []int{382, 32}
	assert.Equal(t, &want, testData.X, "X")
}

func TestEnvArrayPtr2(t *testing.T) {
	require.NoError(t, os.Setenv("X", "382:32"), "set X")
	var testData struct {
		X **[]int `env:"X,split=:"`
	}
	registry := NewRegistry()
	require.NoError(t, registry.Request(&testData), "add model")
	err := registry.Configure()
	require.NoError(t, err, "configure")
	want := []int{382, 32}
	assert.Equal(t, &want, *testData.X, "X")
}

func TestEnvArrayPtr3(t *testing.T) {
	require.NoError(t, os.Setenv("X", "382:32"), "set X")
	var testData struct {
		X ***[]int `env:"X,split=:"`
	}
	registry := NewRegistry()
	require.NoError(t, registry.Request(&testData), "add model")
	err := registry.Configure()
	require.NoError(t, err, "configure")
	want := []int{382, 32}
	assert.Equal(t, &want, **testData.X, "X")
}

