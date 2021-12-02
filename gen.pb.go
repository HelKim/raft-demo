package main

//go:generate protoc -I=.\proto -I=$GOPATH\src -I=$GOPATH\src\github.com\gogo\protobuf --gogofaster_out=plugins=grpc:.\proto\service.proto
