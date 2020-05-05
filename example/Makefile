NAME := gmsec
gen:# 编译hello.proto
	protoc --proto_path="./apidoc/proto/hello/" --gmsec_out=plugins=gmsec:./rpc/ hello.proto
source_build:
	./proto_install.sh
build:
	./proto_install.sh
