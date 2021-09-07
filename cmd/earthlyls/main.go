package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"
	earthlyast "github.com/earthly/earthly/ast"
	earthlyastspec "github.com/earthly/earthly/ast/spec"
	"github.com/sirupsen/logrus"
	"github.com/wingyplus/earthlylsp/internal/protocol"
)

type LangServer struct {
	// NOTE: This is a bit kind of unsafe. We need to revisit
	// again.
	indexes map[protocol.DocumentURI]earthlyastspec.Earthfile
}

func (langsrv *LangServer) Window_LogMessage(ctx context.Context, level protocol.MessageType, message string) error {
	return jrpc2.ServerFromContext(ctx).Notify(ctx, "window/logMessage", protocol.LogMessageParams{
		Type:    level,
		Message: message,
	})
}

func (langsrv *LangServer) Initialize(ctx context.Context, req protocol.InitializeParams) protocol.InitializeResult {
	langsrv.Window_LogMessage(ctx, protocol.Info, "Running initializing...")
	result := protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			DefinitionProvider: true,
		},
	}
	result.ServerInfo.Name = "earthlyls"
	result.ServerInfo.Version = "0.1.0"

	return result
}

// TextDocument_DidOpen indexing a file when editor client open a file.
func (langsrv *LangServer) TextDocument_DidOpen(ctx context.Context, req protocol.DidOpenTextDocumentParams) (int, error) {
	langsrv.Window_LogMessage(ctx, protocol.Info, "Receive file open.")

	u, err := url.Parse(string(req.TextDocument.URI))
	if err != nil {
		langsrv.Window_LogMessage(ctx, protocol.Error, fmt.Sprintf("Cannot parse %s: %s", req.TextDocument.URI, err.Error()))
		return 0, nil
	}
	earthfile, err := earthlyast.Parse(ctx, u.Path, true)
	if err != nil {
		langsrv.Window_LogMessage(ctx, protocol.Error, fmt.Sprintf("Cannot parse %s: %s", req.TextDocument.URI, err.Error()))
		return 0, nil
	}
	langsrv.indexes[req.TextDocument.URI] = earthfile
	logrus.Debug(langsrv.indexes)
	return 0, nil
}

// TextDocument_DocumentLink returns empty document link to prevent client error due to method not found.
func (langsrv *LangServer) TextDocument_DocumentLink(ctx context.Context, req protocol.DocumentLinkParams) []protocol.DocumentLink {
	return []protocol.DocumentLink{}
}

func (langsrv *LangServer) TextDocument_Definition(ctx context.Context, req protocol.DefinitionParams) *protocol.Location {
	return nil
}

func main() {
	// TODO: refactoring this.
	home, err := os.UserHomeDir()
	if err != nil {
		logrus.Fatal(err)
	}
	f, err := os.Create(filepath.Join(home, "earthlyls.log"))
	if err != nil {
		logrus.Fatal(err)
	}
	defer f.Close()

	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(f)

	langsrv := &LangServer{
		indexes: make(map[protocol.DocumentURI]earthlyastspec.Earthfile),
	}
	// Create handler to handle language server protocol method.
	assigner := handler.Map{
		"initialize":                handler.New(langsrv.Initialize),
		"textDocument/didOpen":      handler.New(langsrv.TextDocument_DidOpen),
		"textDocument/documentLink": handler.New(langsrv.TextDocument_DocumentLink),
		"textDocument/definition":   handler.New(langsrv.TextDocument_Definition),
	}

	// Starting a server.
	srv := jrpc2.NewServer(assigner, &jrpc2.ServerOptions{AllowPush: true})
	srv.Start(channel.LSP(os.Stdin, os.Stdout))
	if err := srv.Wait(); err != nil {
		logrus.Fatal(err)
	}
}
