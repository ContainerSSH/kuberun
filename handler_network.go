package kuberun

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/containerssh/sshserver"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	watchTools "k8s.io/client-go/tools/watch"
	"k8s.io/kubectl/pkg/util/interrupt"
)

type networkHandler struct {
	mutex        *sync.Mutex
	client       net.TCPAddr
	connectionID []byte
	config       Config

	// onDisconnect contains a per-channel disconnect handler
	onDisconnect map[uint64]func()
	onShutdown   map[uint64]func(shutdownContext context.Context)

	cli         *kubernetes.Clientset
	restClient  *restclient.RESTClient
	pod         *core.Pod
	cancelStart func()
	labels      map[string]string
}

func (n *networkHandler) OnAuthPassword(_ string, _ []byte) (response sshserver.AuthResponse, reason error) {
	return sshserver.AuthResponseUnavailable, fmt.Errorf("the backend handler does not support authentication")
}

func (n *networkHandler) OnAuthPubKey(_ string, _ []byte) (response sshserver.AuthResponse, reason error) {
	return sshserver.AuthResponseUnavailable, fmt.Errorf("the backend handler does not support authentication")
}

func (n *networkHandler) OnHandshakeFailed(_ error) {
}

// isPodAvailableEvent returns true if the event signals that the pod is either running or has already finished running.
func (n *networkHandler) isPodAvailableEvent(event watch.Event) (bool, error) {
	if event.Type == watch.Deleted {
		return false, errors.NewNotFound(schema.GroupResource{Resource: "pods"}, "")
	}

	switch eventObject := event.Object.(type) {
	case *core.Pod:
		switch eventObject.Status.Phase {
		case core.PodFailed, core.PodSucceeded:
			return true, nil
		case core.PodRunning:
			conditions := eventObject.Status.Conditions
			if conditions != nil {
				for _, condition := range conditions {
					if condition.Type == core.PodReady &&
						condition.Status == core.ConditionTrue {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}

//This function waits for a pod to be either running or already complete.
func (n *networkHandler) waitForPodAvailable(ctx context.Context, pod *core.Pod) (result *core.Pod, err error) {
	timeoutContext, cancelTimeoutContext := watchTools.ContextWithOptionalTimeout(
		ctx, n.config.Timeout,
	)
	defer cancelTimeoutContext()

	fieldSelector := fields.
		OneTermEqualSelector("metadata.name", pod.Name).
		String()
	listWatch := &cache.ListWatch{
		ListFunc: func(options meta.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			return n.cli.
				CoreV1().
				Pods(pod.Namespace).
				List(ctx, options)
		},
		WatchFunc: func(options meta.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			return n.cli.
				CoreV1().
				Pods(pod.Namespace).
				Watch(ctx, options)
		},
	}

	err = interrupt.
		New(nil, cancelTimeoutContext).
		Run(
			func() error {
				event, err := watchTools.UntilWithSync(
					timeoutContext,
					listWatch,
					&core.Pod{},
					nil,
					n.isPodAvailableEvent,
				)
				if event != nil {
					result = event.Object.(*core.Pod)
				}
				return err
			},
		)

	return result, err
}

func (n *networkHandler) OnHandshakeSuccess(username string) (connection sshserver.SSHConnectionHandler, failureReason error) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	ctx, cancelFunc := context.WithTimeout(context.Background(), n.config.Timeout)
	n.cancelStart = cancelFunc

	if n.pod != nil {
		return nil, fmt.Errorf("handshake already complete")
	}

	spec := n.config.Pod.Spec

	n.labels = map[string]string{
		"containerssh_connection_id": hex.EncodeToString(n.connectionID),
		"containerssh_ip":            n.client.IP.String(),
		"containerssh_username":      username,
	}

	pod, err := n.cli.CoreV1().Pods(n.config.Pod.Namespace).Create(
		ctx,
		&core.Pod{
			ObjectMeta: meta.ObjectMeta{
				GenerateName: "containerssh-",
				Namespace:    n.config.Pod.Namespace,
				Labels:       n.labels,
			},
			Spec: spec,
		},
		meta.CreateOptions{},
	)
	if err != nil {
		return nil, err
	}

	n.pod = pod

	n.mutex.Unlock()
	pod, err = n.waitForPodAvailable(ctx, pod)
	n.mutex.Lock()
	if err != nil {
		return nil, err
	}

	return &sshConnectionHandler{
		networkHandler: n,
		username:       username,
	}, nil
}

func (n *networkHandler) OnDisconnect() {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	//TODO shutdown timeout handling
	shutdownContext := context.TODO()

	success := true
	for i := 0; i < 6; i++ {
		if n.pod != nil {
			if n.cancelStart != nil {
				n.cancelStart()
				n.cancelStart = nil
			}
			success = false
			request := n.restClient.
				Delete().
				Namespace(n.pod.Namespace).
				Resource("pods").
				Name(n.pod.Name).
				Body(&meta.DeleteOptions{})
			result := request.Do(shutdownContext)
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
