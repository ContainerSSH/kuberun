package kuberun

import (
	"fmt"
	"io"

	"github.com/containerssh/sshserver"
)

type sessionHandler struct {
	networkHandler *networkHandler
	env            map[string]string
	sshHandler     *sshConnectionHandler
	running        bool
	pty            bool
	columns        uint32
	rows           uint32
	channelID      uint64
}

func (s *sessionHandler) OnUnsupportedChannelRequest(_ uint64, _ string, _ []byte) {}

func (s *sessionHandler) OnFailedDecodeChannelRequest(_ uint64, _ string, _ []byte, _ error) {}

func (s *sessionHandler) OnEnvRequest(_ uint64, name string, value string) error {
	s.sshHandler.mutex.Lock()
	defer s.sshHandler.mutex.Unlock()
	if s.running {
		return fmt.Errorf("program already running")
	}
	s.env[name] = value
	return nil
}

func (s *sessionHandler) OnPtyRequest(_ uint64, term string, columns uint32, rows uint32, _ uint32, _ uint32, _ []byte) error {
	s.sshHandler.mutex.Lock()
	defer s.sshHandler.mutex.Unlock()
	if s.running {
		return fmt.Errorf("program already running")
	}
	s.env["TERM"] = term
	s.pty = true
	s.columns = columns
	s.rows = rows
	return nil
}

func (s *sessionHandler) OnExecRequest(_ uint64, program string, stdin io.Reader, stdout io.Writer, stderr io.Writer, onExit func(exitStatus sshserver.ExitStatus)) error {
	s.sshHandler.mutex.Lock()
	defer s.sshHandler.mutex.Unlock()

	return nil
}

func (s *sessionHandler) OnShell(_ uint64, stdin io.Reader, stdout io.Writer, stderr io.Writer, onExit func(exitStatus sshserver.ExitStatus)) error {
	s.sshHandler.mutex.Lock()
	s.sshHandler.mutex.Unlock()

	return nil
}

func (s *sessionHandler) OnSubsystem(_ uint64, subsystem string, stdin io.Reader, stdout io.Writer, stderr io.Writer, onExit func(exitStatus sshserver.ExitStatus)) error {
	s.sshHandler.mutex.Lock()
	s.sshHandler.mutex.Unlock()

	return nil
}

func (s *sessionHandler) OnSignal(_ uint64, _ string) error {
	return fmt.Errorf("signals are not supported")
}

func (s *sessionHandler) OnWindow(_ uint64, columns uint32, rows uint32, _ uint32, _ uint32) error {
	s.sshHandler.mutex.Lock()
	defer s.sshHandler.mutex.Unlock()
	if !s.running {
		return fmt.Errorf("program not running")
	}

	return nil
}
