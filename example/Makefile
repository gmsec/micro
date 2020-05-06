NAME := gmsec
gen:# 编译hello.proto
	protoc --proto_path="./apidoc/proto/hello/" --gmsec_out=plugins=gmsec:./rpc/ hello.proto
install:
	./proto_install.sh
source_install:
	./proto_install.sh
build:
	go build *.go
