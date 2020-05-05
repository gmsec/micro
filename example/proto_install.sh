#!/bin/bash -x 

version="3.11.4"

# su - xxj -c "qwer"
# download
curl -fLo protobuf.tar.gz https://github.com/protocolbuffers/protobuf/releases/download/v${version}/protoc-${version}-osx-x86_64.zip
mkdir protobuf-${version}
tar -xvf protobuf.tar.gz -C ./protobuf-${version}
cd protobuf-${version}

# install
xattr -c ./bin/protoc
cp -r ./bin/protoc $GOPATH/bin
cd ../
rm -rf protobuf-${version}/

# install go-grpc
go get -u google.golang.org/grpc
go get -u github.com/golang/protobuf/protoc-gen-go

echo "SUCCESS"
#end