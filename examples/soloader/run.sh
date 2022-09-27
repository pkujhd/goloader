#!/bin/bash

go build -o soloader.so -buildmode=c-shared soloader.go
mv elf.txt elf.cpp
g++ -std=c++11 -ldl elf.cpp -w -o elf
mv elf.cpp elf.txt
go tool compile -o soloadertest.o ../inline/inline.go
./elf

rm -f soloader.h soloader.so soloadertest.o elf
