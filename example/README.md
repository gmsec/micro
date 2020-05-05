# install 

[protoc-gen-gmsec]()

# build

```
protoc --proto_path="./apidoc/proto/hello/" --gmsec_out=plugins=gmsec:./rpc/ hello.proto
go build hello.go main.go
```

# server

```
 ./hello -tag=server
```

# client

```
 ./hello -tag=client
```

