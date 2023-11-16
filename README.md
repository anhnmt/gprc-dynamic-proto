# gRPC Dynamic Proto

This project is inspired by deploying a grpc gateway without having to generate the `.pb.go` file from the proto file.

To see more ideas and implementations of this project, please [see here](https://github.com/connectrpc/vanguard-go/issues/104)

# How to use it

- Parse from the proto file to get `*dynamicpb.Types` and `protoreflect.ServiceDescriptor`

```go
p := protoparse.Parser{
    ImportPaths: []string{
        "proto",
        "googleapis",
    },
}

fds, err := p.ParseFiles(files...)
if err != nil {
    log.Err(err).Msg("could not parse given files")
    return
}

resolver := &protoregistry.Files{}
for _, fileDesc := range fds {
    if err = resolver.RegisterFile(fileDesc.UnwrapFile()); err != nil {
        log.Err(err).Msg("could not register given files")
        return
    }
}

path, err := resolver.FindFileByPath("user/v1/user.proto")
if err != nil {
    log.Err(err).Msg("could not find given files")
    return
}
```

- Initialize reverse proxy

```go
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
```

- Declare a new service

```go
types := dynamicpb.NewTypes(resolver)
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
```

# Special thanks to
- [jhump](https://github.com/jhump)
- [emcfarlane](https://github.com/emcfarlane)