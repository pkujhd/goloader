#!/bin/bash

go build -o soloader.so -buildmode=c-shared soloader.go
g++ -std=c++11 -ldl elf.cpp -w -o elf
go tool compile -o soloadertest.o ../inline/inline.go
./elf

rm -f soloader.h soloader.so soloadertest.o elf
