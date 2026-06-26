package commands

import (
	"bytes"
	"crypto/ed25519"
	"io"
	"log"
	"net"
	"strings"
	"testing"
	"time"

	clientconnection "github.com/NHAS/reverse_ssh/internal/client/connection"
	clienthandlers "github.com/NHAS/reverse_ssh/internal/client/handlers"
	"github.com/NHAS/reverse_ssh/internal/server/users"
	"github.com/NHAS/reverse_ssh/internal/terminal"
	"github.com/NHAS/reverse_ssh/pkg/logger"
	"golang.org/x/crypto/ssh"
)

func TestModuleCommandListIntegration(t *testing.T) {
	oldLogOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(oldLogOutput)

	serverConn, clientConn, chans, cleanup := testReverseClientConnection(t)
	defer cleanup()

	id, _, err := users.AssociateClient(serverConn)
	if err != nil {
		t.Fatalf("AssociateClient returned error: %v", err)
	}
	defer users.DisassociateClient(id, serverConn)

	user, _, err := users.CreateOrGetUser("module-test-user", nil)
	if err != nil {
		t.Fatalf("CreateOrGetUser returned error: %v", err)
	}

	session := clientconnection.NewSession(clientConn)
	go clientconnection.RegisterChannelCallbacks(chans, logger.NewLog("module-test-client"), map[string]func(ssh.NewChannel, logger.Logger){
		"session": clienthandlers.Session(session),
	})

	var tty bytes.Buffer
	line := terminal.ParseLine("module "+id+" list", 0)
	if err := (&moduleCommand{}).Run(user, &tty, line); err != nil {
		t.Fatalf("module command returned error: %v", err)
	}

	output := tty.String()
	if !strings.Contains(output, "list\n") || !strings.Contains(output, "sftp\n") {
		t.Fatalf("module list output missing core modules:\n%s", output)
	}
}

func testReverseClientConnection(t *testing.T) (*ssh.ServerConn, ssh.Conn, <-chan ssh.NewChannel, func()) {
	t.Helper()

	serverSigner := testSigner(t)
	clientSigner := testSigner(t)

	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			return &ssh.Permissions{Extensions: map[string]string{
				"comment":   "module-test",
				"pubkey-fp": "module-test-fp",
				"owners":    "",
			}}, nil
		},
	}
	serverConfig.AddHostKey(serverSigner)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen returned error: %v", err)
	}
	serverReady := make(chan *ssh.ServerConn, 1)
	serverErr := make(chan error, 1)
	go func() {
		serverSide, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		conn, _, reqs, err := ssh.NewServerConn(serverSide, serverConfig)
		if err != nil {
			serverErr <- err
			return
		}
		go ssh.DiscardRequests(reqs)
		serverReady <- conn
	}()

	clientSide, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}
	clientConfig := &ssh.ClientConfig{
		User:            "module-test-client",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(clientSigner)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	clientConn, chans, reqs, err := ssh.NewClientConn(clientSide, "pipe", clientConfig)
	if err != nil {
		t.Fatalf("NewClientConn returned error: %v", err)
	}
	go ssh.DiscardRequests(reqs)

	var serverConn *ssh.ServerConn
	select {
	case serverConn = <-serverReady:
	case err := <-serverErr:
		t.Fatalf("NewServerConn returned error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server SSH connection")
	}

	cleanup := func() {
		_ = listener.Close()
		_ = clientConn.Close()
		_ = serverConn.Close()
	}
	return serverConn, clientConn, chans, cleanup
}

func testSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("NewSignerFromKey returned error: %v", err)
	}
	return signer
}
