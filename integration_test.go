package kuberun_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/containerssh/kuberun"
)

func TestSuccessfulHandshakeShouldCreatePod(t *testing.T) {
	config, err := createConfigFromKubeConfig()
	if err != nil {
		assert.FailNow(t, "failed to create configuration from the current users kubeconfig", err)
	}

	_, err = kuberun.New(config)
	if err != nil {
		assert.FailNow(t, "failed to create handler", err)
	}
}
