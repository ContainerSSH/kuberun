package kuberun

import (
	"sync"

	"github.com/containerssh/sshserver"
)

type sshConnectionHandler struct {
	networkHandler *networkHandler
	mutex          *sync.Mutex
	username       string
}

func (s *sshConnectionHandler) OnUnsupportedGlobalRequest(_ uint64, _ string, _ []byte) {}

func (s *sshConnectionHandler) OnUnsupportedChannel(_ uint64, _ string, _ []byte) {}

func (s *sshConnectionHandler) OnSessionChannel(channelID uint64, _ []byte) (channel sshserver.SessionChannelHandler, failureReason sshserver.ChannelRejection) {
	return &sessionHandler{
		channelID:      channelID,
		networkHandler: s.networkHandler,
		sshHandler:     s,
	}, nil
}