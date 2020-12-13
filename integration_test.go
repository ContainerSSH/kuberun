package kuberun_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/containerssh/log"
	"github.com/containerssh/sshserver"
	"github.com/containerssh/structutils"
	"github.com/stretchr/testify/assert"
	v1Api "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/containerssh/kuberun"
)

func TestSuccessfulHandshakeShouldCreatePod(t *testing.T) {
	t.Parallel()

	config := kuberun.Config{}
	structutils.Defaults(&config)

	config.Pod.Spec.Containers[0].Image = "docker.io/library/busybox"

	if err := kuberun.SetConfigFromKubeConfig(&config); err != nil {
		assert.FailNow(t, "failed to create configuration from the current users kubeconfig (%v)", err)
	}

	connectionID := sshserver.GenerateConnectionID()
	logger := getLogger(t)

	kr := createKuberun(t, connectionID, config, logger)
	defer kr.OnDisconnect()

	_, err := kr.OnHandshakeSuccess("test")
	assert.Nil(t, err, "failed to create handshake handler (%v)", err)

	k8sConfig := kuberun.CreateConnectionConfig(config)
	cli, err := kubernetes.NewForConfig(&k8sConfig)
	assert.Nil(t, err, "failed to create k8s client (%v)", err)

	podList, err := cli.CoreV1().Pods(config.Pod.Namespace).List(context.Background(), v1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", "containerssh_connection_id", connectionID),
	})
	assert.Nil(t, err, "failed to list k8s pods (%v)", err)
	assert.Equal(t, 1, len(podList.Items))
	assert.Equal(t, v1Api.PodRunning, podList.Items[0].Status.Phase)
	assert.Equal(t, true, *podList.Items[0].Status.ContainerStatuses[0].Started)
}

func TestSingleSessionShouldRunProgram(t *testing.T) {
	t.Parallel()

	_, session, kr := setupKuberun(t)
	defer kr.OnDisconnect()

	stdin := bytes.NewReader([]byte{})
	var stdoutBytes bytes.Buffer
	stdout := bufio.NewWriter(&stdoutBytes)
	var stderrBytes bytes.Buffer
	stderr := bufio.NewWriter(&stderrBytes)
	done := make(chan struct{})
	status := 0
	err := session.OnExecRequest(
		0,
		"echo \"Hello world!\"",
		stdin,
		stdout,
		stderr,
		func(exitStatus sshserver.ExitStatus) {
			status = int(exitStatus)
			done <- struct{}{}
		},
	)
	assert.Nil(t, err)
	<-done
	assert.Nil(t, stdout.Flush())
	assert.Equal(t, "Hello world!\n", stdoutBytes.String())
	assert.Equal(t, "", stderrBytes.String())
	assert.Equal(t, 0, status)
}

func TestCommandExecutionShouldReturnStatusCode(t *testing.T) {
	t.Parallel()

	_, session, kr := setupKuberun(t)
	defer kr.OnDisconnect()

	stdin := bytes.NewReader([]byte{})
	var stdoutBytes bytes.Buffer
	stdout := bufio.NewWriter(&stdoutBytes)
	var stderrBytes bytes.Buffer
	stderr := bufio.NewWriter(&stderrBytes)
	done := make(chan struct{})
	status := 0
	err := session.OnExecRequest(
		0,
		"exit 42",
		stdin,
		stdout,
		stderr,
		func(exitStatus sshserver.ExitStatus) {
			status = int(exitStatus)
			done <- struct{}{}
		},
	)
	assert.Nil(t, err)
	<-done
	assert.Nil(t, stdout.Flush())
	assert.Equal(t, 42, status)
}

func TestSingleSessionShouldRunShell(t *testing.T) {
	t.Parallel()

	logger, session, kr := setupKuberun(t)
	defer kr.OnDisconnect()

	var err error
	stdin, stdinWriter := io.Pipe()
	stdoutReader, stdout := io.Pipe()
	_, stderr := io.Pipe()
	done := make(chan struct{})
	status := 0
	must(t, assert.NoError(t, session.OnEnvRequest(0, "foo", "bar")))
	must(t, assert.NoError(t, session.OnPtyRequest(1, "xterm", 80, 25, 800, 600, []byte{})))
	go func() {
		must(t, assert.NoError(t, readUntil(stdoutReader, []byte("# "))))

		must(t, assert.NoError(t, session.OnWindow(2, 120, 25, 800, 600)))

		// HACK: Kubernetes doesn't handle window resizes synchronously. See HACKS.md
		time.Sleep(200 * time.Millisecond)
		logger.Warningf("The following test step may hang indefinitely. See HACKS.md \"Kubernetes doesn't handle window resizes synchronously\" for details...")
		_, err = stdinWriter.Write([]byte("tput cols\n"))
		must(t, assert.NoError(t, readUntil(stdoutReader, []byte("tput cols\r\n120\r\n# "))))
		logger.Warningf("Test step complete.")

		_, err = stdinWriter.Write([]byte("echo \"Hello world!\"\n"))
		must(t, assert.NoError(t, err))

		must(t, assert.NoError(t, readUntil(stdoutReader, []byte("echo \"Hello world!\"\r\nHello world!\r\n# "))))

		_, err = stdinWriter.Write([]byte("exit\n"))
		must(t, assert.NoError(t, err))

		must(t, assert.NoError(t, readUntil(stdoutReader, []byte("exit\r\n"))))
	}()
	err = session.OnShell(
		3,
		stdin,
		stdout,
		stderr,
		func(exitStatus sshserver.ExitStatus) {
			status = int(exitStatus)
			done <- struct{}{}
		},
	)
	must(t, assert.Nil(t, err))
	<-done
	assert.Equal(t, 0, status)
}

func setupKuberun(t *testing.T) (log.Logger, sshserver.SessionChannelHandler, sshserver.NetworkConnectionHandler) {
	config := kuberun.Config{}
	structutils.Defaults(&config)

	config.Pod.ShellCommand = []string{"/bin/sh"}

	err := kuberun.SetConfigFromKubeConfig(&config)
	assert.Nil(t, err, "failed to set up kube config (%v)", err)

	connectionID := sshserver.GenerateConnectionID()
	logger := getLogger(t)

	kr := createKuberun(t, connectionID, config, logger)

	ssh, err := kr.OnHandshakeSuccess("test")
	assert.Nil(t, err, "failed to create handshake handler (%v)", err)

	session, err := ssh.OnSessionChannel(0, []byte{})
	assert.Nil(t, err, "failed to to create session channel (%v)", err)
	return logger, session, kr
}

func must(t *testing.T, arg bool) {
	if !arg {
		t.FailNow()
	}
}

func readUntil(reader io.Reader, buffer []byte) error {
	byteBuffer := bytes.NewBuffer([]byte{})
	for {
		buf := make([]byte, 1024)
		n, err := reader.Read(buf)
		if err != nil {
			return err
		}
		byteBuffer.Write(buf[:n])
		if bytes.Equal(byteBuffer.Bytes(), buffer) {
			return nil
		}
	}
}

func createKuberun(
	t *testing.T,
	connectionID string,
	config kuberun.Config,
	logger log.Logger,
) sshserver.NetworkConnectionHandler {
	kr, err := kuberun.New(
		net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 2222,
			Zone: "",
		},
		connectionID,
		config,
		logger,
	)
	assert.NoError(t, err, "failed to create handler (%v)", err)
	return kr
}

func getLogger(t *testing.T) log.Logger {
	logger, err := log.New(
		log.Config{
			Level:  log.LevelDebug,
			Format: log.FormatText,
		},
		"kuberun",
		os.Stdout,
	)
	assert.NoError(t, err)
	return logger
}
