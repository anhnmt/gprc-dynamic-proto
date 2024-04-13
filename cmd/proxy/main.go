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

	"connectrpc.com/grpcreflect"
	"connectrpc.com/vanguard"
	"github.com/jhump/protoreflect/desc/protoparse"
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

func main() {
	p := protoparse.Parser{
		ImportPaths: []string{
			"proto",
			"googleapis",
		},
	}

	fds, err := p.ParseFiles("user/v1/user.proto")
	if err != nil {
		log.Err(err).Msg("could not parse given files")
		return
	}

	// resolver := protoregistry.GlobalFiles
	for _, fileDesc := range fds {
		if err = protoregistry.GlobalFiles.RegisterFile(fileDesc.UnwrapFile()); err != nil {
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

	serviceNames := make([]string, 0, svcDescs.Len())
	if svcDescs.Len() > 0 {
		for i := 0; i < svcDescs.Len(); i++ {
			svc := vanguard.NewServiceWithSchema(
				svcDescs.Get(i),
				proxy,
				svcOpts...,
			)
			services = append(services, svc)
			serviceNames = append(serviceNames, string(svcDescs.Get(i).FullName()))
		}
	}

	transcoder, err := vanguard.NewTranscoder(services)
	if err != nil {
		log.Err(err).Msg("Could not create transcoder")
		return
	}

	reflector := grpcreflect.NewStaticReflector(
		serviceNames...,
	)

	// reflector := grpcreflect.NewStaticReflector(
	//     userv1connect.UserServiceName,
	// )

	mux := http.NewServeMux()
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	// Many tools still expect the older version of the server reflection API, so
	// most servers should mount both handlers.
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))
	mux.Handle("/", transcoder)

	addr := fmt.Sprintf(":%d", 8000)

	// create new http server
	srv := &http.Server{
		Addr: addr,
		Handler: h2c.NewHandler(
			mux,
			&http2.Server{},
		),
	}

	log.Info().Msg("Starting server on http://localhost:8000")

	// run the server
	panic(srv.ListenAndServe())
}
