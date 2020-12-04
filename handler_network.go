package kuberun

import (
	"context"
	"encoding/hex"
	"fmt"
	goLog "log"
	"net"
	"sync"
	"time"

	"github.com/containerssh/log"
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
)

type networkHandler struct {
	mutex        *sync.Mutex
	client       net.TCPAddr
	connectionID []byte
	config       Config

	// onDisconnect contains a per-channel disconnect handler
	onDisconnect map[uint64]func()
	onShutdown   map[uint64]func(shutdownContext context.Context)

	cli              *kubernetes.Clientset
	restClient       *restclient.RESTClient
	pod              *core.Pod
	cancelStart      func()
	labels           map[string]string
	logger           log.Logger
	restClientConfig restclient.Config
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
			for _, condition := range conditions {
				if condition.Type == core.PodReady &&
					condition.Status == core.ConditionTrue {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

//This function waits for a pod to be either running or already complete.
func (n *networkHandler) waitForPodAvailable(ctx context.Context) (err error) {

	fieldSelector := fields.
		OneTermEqualSelector("metadata.name", n.pod.Name).
		String()
	listWatch := &cache.ListWatch{
		ListFunc: func(options meta.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			return n.cli.
				CoreV1().
				Pods(n.pod.Namespace).
				List(ctx, options)
		},
		WatchFunc: func(options meta.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			return n.cli.
				CoreV1().
				Pods(n.pod.Namespace).
				Watch(ctx, options)
		},
	}

	event, err := watchTools.UntilWithSync(
		ctx,
		listWatch,
		&core.Pod{},
		nil,
		n.isPodAvailableEvent,
	)
	if event != nil {
		n.pod = event.Object.(*core.Pod)
	}
	return err
}

func (n *networkHandler) OnHandshakeSuccess(username string) (connection sshserver.SSHConnectionHandler, failureReason error) {
	n.mutex.Lock()
	if n.pod != nil {
		n.mutex.Unlock()
		return nil, fmt.Errorf("handshake already complete")
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), n.config.Timeout)
	n.cancelStart = cancelFunc
	defer func() {
		n.cancelStart = nil
		n.mutex.Unlock()
	}()

	goLogger := log.NewGoLogWriter(n.logger)
	oldFlags := goLog.Flags()
	oldOutput := goLog.Writer()
	oldPrefix := goLog.Prefix()

	goLog.SetFlags(0)
	goLog.SetOutput(goLogger)
	goLog.SetPrefix("")
	defer func() {
		goLog.SetFlags(oldFlags)
		goLog.SetOutput(oldOutput)
		goLog.SetPrefix(oldPrefix)
	}()
	// TODO set logger redirection because the Kubernetes libraries log using the default logger.

	spec := n.config.Pod.Spec

	spec.Containers[n.config.Pod.ConsoleContainerNumber].Command = n.config.Pod.IdleCommand
	n.labels = map[string]string{
		"containerssh_connection_id": hex.EncodeToString(n.connectionID),
		"containerssh_ip":            n.client.IP.String(),
		"containerssh_username":      username,
	}

	var err error
	n.pod, err = n.createPod(ctx, spec)
	if err != nil {
		return nil, err
	}

	n.mutex.Unlock()
	err = n.waitForPodAvailable(ctx)
	n.mutex.Lock()
	if err != nil {
		return nil, err
	}

	return &sshConnectionHandler{
		networkHandler: n,
		username:       username,
	}, nil
}

func (n *networkHandler) createPod(ctx context.Context, spec core.PodSpec) (pod *core.Pod, err error) {
	for {
		pod, err = n.cli.CoreV1().Pods(n.config.Pod.Namespace).Create(
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
		if err == nil {
			return pod, err
		} else {
			select {
			case <-ctx.Done():
				n.logger.Errorf("failed to create pod, giving up (%v)", err)
				return pod, err
			default:
				n.logger.Warningf("failed to create pod, retrying in 10 seconds (%v)", err)
			}
			time.Sleep(10 * time.Second)
		}
	}
}

func (n *networkHandler) OnDisconnect() {
	n.mutex.Lock()

	shutdownContext, cancelFunc := context.WithTimeout(context.Background(), n.config.Timeout)

	success := true
	var lastError error
	for {
		lastError = nil
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
				lastError = result.Error()
				n.logger.Warningf("failed to remove pod, retrying in 10 seconds (%v)", lastError)
				n.mutex.Unlock()
				select {
				case <-shutdownContext.Done():
					break
				default:
					time.Sleep(10 * time.Second)
					n.mutex.Lock()
				}
			} else {
				success = true
				n.mutex.Unlock()
				break
			}
		}
	}
	if !success {
		n.logger.Errorf("failed to remove pod, giving up (%v)", lastError)
	}
	cancelFunc()
}
