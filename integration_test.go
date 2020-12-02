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

	kr, err := kuberun.New(config)
	assert.Nil(t, err, "failed to create handler", err)
	defer kr.OnDisconnect()

	_, err = kr.OnHandshakeSuccess("test")
	assert.Nil(t, err, "failed to create handshake handler", err)

	//TODO add tests
}
