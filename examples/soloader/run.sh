#!/bin/bash

go build -o soloader.so -buildmode=c-shared soloader.go
g++ -g -std=c++11 -ldl elf.cpp -w -o elf
go tool compile soloadertest.go
./elf

rm -f soloader.h soloader.so soloadertest.o elf
