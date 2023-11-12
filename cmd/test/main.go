package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"connectrpc.com/vanguard"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
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
	p := protoparse.Parser{
		ImportPaths: []string{
			"proto",
			"googleapis",
		},
		Accessor: func(filename string) (io.ReadCloser, error) {
			return ReadFileContent(filename)
		},
	}

	fds, err := p.ParseFiles(
		"user/v1/user.proto",
		"google/api/annotations.proto",
		"google/api/http.proto",
		"google/protobuf/descriptor.proto",
	)
	if err != nil {
		log.Err(err).Msg("could not parse given files")
		return
	}

	files := make([]*descriptorpb.FileDescriptorProto, 0)
	for _, fd := range fds {
		files = append(files, fd.AsFileDescriptorProto())
	}

	newFiles, err := protodesc.NewFiles(&descriptorpb.FileDescriptorSet{
		File: files,
	})
	if err != nil {
		log.Err(err).Msg("could not parse given files")
		return
	}

	path, err := newFiles.FindFileByPath("user/v1/user.proto")
	if err != nil {
		log.Err(err).Msg("could not parse given files")
		return
	}

	serviceDesc := path.Services().ByName("UserService")
	types := dynamicpb.NewTypes(newFiles)

	remote, err := url.Parse("http://localhost:8080")
	if err != nil {
		log.Err(err).Msg("Could not parse remote")
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(remote)

	services := []*vanguard.Service{
		vanguard.NewServiceWithSchema(serviceDesc, proxy, vanguard.WithTypeResolver(types)),
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

	log.Info().Str("filename", filePath).Send()

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	readCloser := io.NopCloser(bytes.NewReader(content))
	return readCloser, nil
}
