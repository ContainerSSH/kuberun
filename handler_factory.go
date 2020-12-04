package kuberun

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/containerssh/log"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
)

func New(config Config, connectionID []byte, client net.TCPAddr, logger log.Logger) (*networkHandler, error) {
	connectionConfig := createConnectionConfig(config)

	cli, err := kubernetes.NewForConfig(&connectionConfig)
	if err != nil {
		return nil, err
	}

	restClient, err := restclient.RESTClientFor(&connectionConfig)
	if err != nil {
		return nil, err
	}

	return &networkHandler{
		restClientConfig: connectionConfig,
		mutex:            &sync.Mutex{},
		client:           client,
		connectionID:     connectionID,
		config:           config,
		onDisconnect:     map[uint64]func(){},
		onShutdown:       map[uint64]func(shutdownContext context.Context){},
		cli:              cli,
		restClient:       restClient,
		pod:              nil,
		cancelStart:      nil,
		labels:           nil,
		logger:           logger,
	}, nil
}

func createConnectionConfig(config Config) restclient.Config {
	return restclient.Config{
		Host:    config.Connection.Host,
		APIPath: config.Connection.APIPath,
		ContentConfig: restclient.ContentConfig{
			GroupVersion:         &v1.SchemeGroupVersion,
			NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		},
		Username:        config.Connection.Username,
		Password:        config.Connection.Password,
		BearerToken:     config.Connection.BearerToken,
		BearerTokenFile: config.Connection.BearerTokenFile,
		Impersonate:     restclient.ImpersonationConfig{},
		TLSClientConfig: restclient.TLSClientConfig{
			Insecure:   config.Connection.Insecure,
			ServerName: config.Connection.ServerName,
			CertFile:   config.Connection.CertFile,
			KeyFile:    config.Connection.KeyFile,
			CAFile:     config.Connection.CAFile,
			CertData:   []byte(config.Connection.CertData),
			KeyData:    []byte(config.Connection.KeyData),
			CAData:     []byte(config.Connection.CAData),
		},
		UserAgent: "ContainerSSH",
		QPS:       config.Connection.QPS,
		Burst:     config.Connection.Burst,
		Timeout:   60 * time.Second,
	}
}
