package kuberun

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/containerssh/sshserver"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

type networkHandler struct {
	mutex        *sync.Mutex
	client       net.TCPAddr
	connectionID []byte
	config       Config

	// onDisconnect contains a per-channel disconnect handler
	onDisconnect map[uint64]func()
	onShutdown   map[uint64]func(shutdownContext context.Context)

	cli        *kubernetes.Clientset
	restClient *restclient.RESTClient
	pod        *core.Pod
}

func (n *networkHandler) OnAuthPassword(_ string, _ []byte) (response sshserver.AuthResponse, reason error) {
	return sshserver.AuthResponseUnavailable, fmt.Errorf("the backend handler does not support authentication")
}

func (n *networkHandler) OnAuthPubKey(_ string, _ []byte) (response sshserver.AuthResponse, reason error) {
	return sshserver.AuthResponseUnavailable, fmt.Errorf("the backend handler does not support authentication")
}

func (n *networkHandler) OnHandshakeFailed(_ error) {
}

func (n *networkHandler) OnHandshakeSuccess(username string) (connection sshserver.SSHConnectionHandler, failureReason error) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	if n.pod != nil {
		return nil, fmt.Errorf("handshake already complete")
	}

	spec := n.config.Pod.Spec

	pod, err := n.cli.CoreV1().Pods(n.config.Pod.Namespace).Create(
		&core.Pod{
			ObjectMeta: meta.ObjectMeta{
				GenerateName: "containerssh-",
				Namespace:    n.config.Pod.Namespace,
			},
			Spec: spec,
		},
	)
	if err != nil {
		return nil, err
	}

	n.pod = pod

	return &sshConnectionHandler{
		networkHandler: n,
		username:       username,
	}, nil
}

func (n *networkHandler) OnDisconnect() {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	success := true
	for i := 0; i < 6; i++ {
		if n.pod != nil {
			success = false
			request := n.restClient.
				Delete().
				Namespace(n.pod.Namespace).
				Resource("pods").
				Name(n.pod.Name).
				Body(&meta.DeleteOptions{})
			result := request.Do()
			if result.Error() != nil {
				//TODO add log
				n.mutex.Unlock()
				time.Sleep(10 * time.Second)
				n.mutex.Lock()
			} else {
				success = true
				break
			}
		}
	}
	if !success {
		//TODO add log
	}
}
