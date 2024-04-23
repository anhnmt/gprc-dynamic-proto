package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/vanguard"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/protobuf/types/known/emptypb"

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
	transcoder, err := vanguard.NewTranscoder(services)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create transcoder")
	}

	reflector := grpcreflect.NewStaticReflector(
		userv1connect.UserServiceName,
	)

	mux := http.NewServeMux()
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))
	mux.Handle("/", transcoder)

	// create new http server
	srv := &http.Server{
		Addr: ":8080",
		// We use the h2c package in order to support HTTP/2 without TLS,
		// so we can handle gRPC requests, which requires HTTP/2, in
		// addition to Connect and gRPC-Web (which work with HTTP 1.1).
		Handler: h2c.NewHandler(
			mux,
			&http2.Server{},
		),
	}

	log.Info().Msg("Starting server on http://localhost:8080")

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

func ConvertBytesToFile(data []byte, filename string) (*os.File, error) {
	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	_, err = file.Write(data)
	if err != nil {
		return nil, err
	}

	_, err = file.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (s *UserService) Upload(_ context.Context, req *connect.Request[userv1.UploadRequest]) (*connect.Response[emptypb.Empty], error) {
	file := req.Msg.GetFile()

	res := connect.NewResponse(&emptypb.Empty{})

	mediaType, params, err := mime.ParseMediaType(file.GetContentType())
	if err != nil {
		return res, err
	}

	buf := bytes.NewReader(file.GetData())

	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(buf, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return res, err
			}
			slurp, err := io.ReadAll(p)
			if err != nil {
				return res, err
			}

			newFile, err := ConvertBytesToFile(slurp, p.FileName())
			if err != nil {
				fmt.Println("Lỗi:", err)
				return res, nil
			}
			defer newFile.Close()
		}
	}

	log.Info().
		Str("contentType", file.GetContentType()).
		Send()

	return res, nil
}
