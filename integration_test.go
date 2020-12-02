package kuberun

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"testing"

	"github.com/containerssh/log/standard"
	"github.com/creasty/defaults"
	"github.com/stretchr/testify/assert"
	v1Api "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randomID() []byte {
	bytes := make([]byte, 16)
	for i := range bytes {
		bytes[i] = byte(letters[rand.Intn(len(letters))])
	}
	return bytes
}

func TestSuccessfulHandshakeShouldCreatePod(t *testing.T) {
	config := Config{}
	err := defaults.Set(&config)

	config.Pod.Spec.Containers[0].Image = "docker.io/library/busybox"

	if err := setConfigFromKubeConfig(&config); err != nil {
		assert.FailNow(t, "failed to create configuration from the current users kubeconfig (%v)", err)
	}

	connectionID := randomID()

	kr, err := New(config, connectionID, net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 2222,
		Zone: "",
	}, standard.New())
	assert.Nil(t, err, "failed to create handler (%v)", err)
	defer kr.OnDisconnect()

	_, err = kr.OnHandshakeSuccess("test")
	assert.Nil(t, err, "failed to create handshake handler (%v)", err)

	k8sConfig := createConnectionConfig(config)
	cli, err := kubernetes.NewForConfig(&k8sConfig)
	assert.Nil(t, err, "failed to create k8s client (%v)", err)

	podList, err := cli.CoreV1().Pods(config.Pod.Namespace).List(context.Background(), v1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", "containerssh_connection_id", hex.EncodeToString(connectionID)),
	})
	assert.Nil(t, err, "failed to list k8s pods (%v)", err)
	assert.Equal(t, 1, len(podList.Items))
	assert.Equal(t, v1Api.PodRunning, podList.Items[0].Status.Phase)
	assert.Equal(t, true, *podList.Items[0].Status.ContainerStatuses[0].Started)
}
