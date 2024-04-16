package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
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

// protocompile is tested
func main() {
	// protocompile not support http rule
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{
				"proto",
				"googleapis",
			},
		}),
	}

	compile, err := compiler.Compile(context.Background(), "user/v1/user.proto")
	if err != nil {
		log.Err(err).Msg("could not compile given files")
		return
	}

	for _, fileDesc := range compile {
		if err = protoregistry.GlobalFiles.RegisterFile(fileDesc); err != nil {
			log.Err(err).Msg("could not register given files")
			return
		}
	}

	path, err := protoregistry.GlobalFiles.FindFileByPath("user/v1/user.proto")
	if err != nil {
		log.Err(err).Msg("could not find given files")
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: "http", Host: "localhost:8080"})
	proxy.Transport = &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			// If you're also using this client for non-h2c traffic, you may want
			// to delegate to tls.Dial if the network isn't TCP or the addr isn't
			// in an allowlist.
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}

	types := dynamicpb.NewTypes(protoregistry.GlobalFiles)
	svcOpts := []vanguard.ServiceOption{
		vanguard.WithTypeResolver(types),
	}

	services := make([]*vanguard.Service, 0)
	svcDescs := path.Services()

	for i := 0; i < svcDescs.Len(); i++ {
		svc := vanguard.NewServiceWithSchema(
			svcDescs.Get(i),
			proxy,
			svcOpts...,
		)

		services = append(services, svc)
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

	log.Info().Msg("Starting server on http://localhost:8000")

	// run the server
	panic(srv.ListenAndServe())
}
