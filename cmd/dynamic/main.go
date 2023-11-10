package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"connectrpc.com/vanguard"
	"github.com/bufbuild/protocompile"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func init() {
	// UNIX Time is faster and smaller than most timestamps
	consoleWriter := &zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
		NoColor:    false,
	}

	// Multi Writer
	writer := []io.Writer{
		consoleWriter,
	}

	// Caller Marshal Function
	zerolog.CallerMarshalFunc = func(_ uintptr, file string, line int) string {
		return fmt.Sprintf("%s:%d", filepath.Base(file), line)
	}

	l := zerolog.
		New(zerolog.MultiLevelWriter(writer...)).
		With().
		Timestamp().
		Caller().
		Logger()

	log.Logger = l
	zerolog.DefaultContextLogger = &l
}

func main() {
	compiler := protocompile.Compiler{
		Resolver: &protocompile.SourceResolver{
			Accessor: func(filename string) (io.ReadCloser, error) {
				log.Info().Str("filename", filename).Send()

				return ReadFileContent(fmt.Sprintf("proto/%v", filename))
			},
		},
	}

	compile, err := compiler.Compile(context.Background(), "user/v1/user.proto")
	if err != nil {
		log.Err(err).Msg("could not compile given files")
		return
	}

	for _, desc := range compile {
		protoregistry.GlobalFiles.RegisterFile(desc)
	}

	path := compile.FindFileByPath("user/v1/user.proto")
	serviceDesc := path.Services().ByName("UserService")

	remote, err := url.Parse("http://localhost:8080")
	if err != nil {
		log.Err(err).Msg("Could not parse remote")
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(remote)

	services := []*vanguard.Service{
		vanguard.NewServiceWithSchema(serviceDesc, proxy),
	}

	transcoder, err := vanguard.NewTranscoder(services)
	if err != nil {
		log.Err(err).Msg("Could not create transcoder")
		return
	}

	addr := fmt.Sprintf(":%d", 8000)

	// create new http server
	srv := &http.Server{
		Addr: addr,
		Handler: h2c.NewHandler(
			transcoder,
			&http2.Server{},
		),
	}

	// run the server
	panic(srv.ListenAndServe())
}

func ReadFileContent(filePath string) (io.ReadCloser, error) {
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return nil, os.ErrNotExist
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, os.ErrNotExist
	}

	readCloser := io.NopCloser(bytes.NewReader(content))
	return readCloser, nil
}
