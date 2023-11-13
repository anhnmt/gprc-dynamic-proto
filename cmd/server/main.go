package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/vanguard"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	userv1 "github.com/anhnmt/gprc-dynamic-proto/proto/gengo/user/v1"
	"github.com/anhnmt/gprc-dynamic-proto/proto/gengo/user/v1/userv1connect"
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
	userService := NewUserService()

	services := []*vanguard.Service{
		vanguard.NewService(userv1connect.NewUserServiceHandler(userService)),
	}

	// Using Vanguard, the server can also accept RESTful requests. The Vanguard
	// Transcoder handles both REST and RPC traffic, so there's no need to mount
	// the RPC-only handler.
	transcoder, err := vanguard.NewTranscoder(services, vanguard.WithDefaultServiceOptions(
		vanguard.WithTargetProtocols(
			vanguard.ProtocolREST,
			vanguard.ProtocolConnect,
			vanguard.ProtocolGRPC,
			vanguard.ProtocolGRPCWeb,
		),
		vanguard.WithNoTargetCompression(),
		vanguard.WithTargetCodecs(vanguard.CodecProto, vanguard.CodecJSON),
	))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create transcoder")
	}

	// create new http server
	srv := &http.Server{
		Addr: ":8080",
		// We use the h2c package in order to support HTTP/2 without TLS,
		// so we can handle gRPC requests, which requires HTTP/2, in
		// addition to Connect and gRPC-Web (which work with HTTP 1.1).
		Handler: h2c.NewHandler(
			transcoder,
			&http2.Server{},
		),
	}

	// run the server
	panic(srv.ListenAndServe())
}

var _ userv1connect.UserServiceHandler = &UserService{}

type UserService struct {
	userv1connect.UnimplementedUserServiceHandler
}

func NewUserService() *UserService {
	return &UserService{}
}

func (s *UserService) List(context.Context, *connect.Request[userv1.ListRequest]) (*connect.Response[userv1.ListResponse], error) {
	return connect.NewResponse(&userv1.ListResponse{
		Data: nil,
	}), nil
}
