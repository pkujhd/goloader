package protobufunload

import (
	"fmt"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"reflect"
	"sync"
	"unsafe"
)

type ProtoRegistryFiles struct {
	// The map of descsByName contains:
	//	EnumDescriptor
	//	EnumValueDescriptor
	//	MessageDescriptor
	//	ExtensionDescriptor
	//	ServiceDescriptor
	//	*packageDescriptor
	//
	// Note that files are stored as a slice, since a package may contain
	// multiple files. Only top-level declarations are registered.
	// Note that enum values are in the top-level since that are in the same
	// scope as the parent enum.
	descsByName map[protoreflect.FullName]interface{}
	filesByPath map[string][]protoreflect.FileDescriptor
	numFiles    int
}

type ProtoRegistryTypes struct {
	typesByName         typesByName
	extensionsByMessage extensionsByMessage

	numEnums      int
	numMessages   int
	numExtensions int
}

type (
	typesByName         map[protoreflect.FullName]interface{}
	extensionsByMessage map[protoreflect.FullName]extensionsByNumber
	extensionsByNumber  map[protoreflect.FieldNumber]protoreflect.ExtensionType
)

type packageDescriptor struct {
	files []protoreflect.FileDescriptor
}

var globalFiles = (*ProtoRegistryFiles)(unsafe.Pointer(protoregistry.GlobalFiles))
var globalTypes = (*ProtoRegistryTypes)(unsafe.Pointer(protoregistry.GlobalTypes))

//go:linkname globalMutex google.golang.org/protobuf/reflect/protoregistry.globalMutex
var globalMutex sync.RWMutex

type emptyInterface struct {
	typ  unsafe.Pointer
	data unsafe.Pointer
}

func deregisterProtobufPackages(dataStart, dataEnd uintptr) {
	globalMutex.Lock()
	defer globalMutex.Unlock()

	for k, desc := range globalTypes.typesByName {
		eface := (*emptyInterface)(unsafe.Pointer(&desc))
		if uintptr(eface.typ) < dataEnd && uintptr(eface.typ) >= dataStart {
			delete(globalTypes.typesByName, k)
			continue
		}
		gopkger, ok := desc.(interface{ GoPackagePath() string })
		if ok {
			strVal := gopkger.GoPackagePath()
			s := (*reflect.StringHeader)(unsafe.Pointer(&strVal))
			if s.Data < dataEnd && s.Data >= dataStart {
				delete(globalTypes.typesByName, k)
			}
		}
	}

	for k, v := range globalFiles.descsByName {
		eface := (*emptyInterface)(unsafe.Pointer(&v))
		if uintptr(eface.typ) < dataEnd && uintptr(eface.typ) >= dataStart {
			delete(globalFiles.descsByName, k)
			continue
		}
		switch desc := v.(type) {
		case protoreflect.EnumDescriptor, protoreflect.EnumValueDescriptor, protoreflect.MessageDescriptor,
			protoreflect.ExtensionDescriptor, protoreflect.ServiceDescriptor:
			gopkger, ok := desc.(interface{ GoPackagePath() string })
			if ok {
				strVal := gopkger.GoPackagePath()
				s := (*reflect.StringHeader)(unsafe.Pointer(&strVal))
				if s.Data < dataEnd && s.Data >= dataStart {
					delete(globalFiles.descsByName, k)
				}
			}
		default:
			if fmt.Sprintf("%T", desc) == "*protoregistry.packageDescriptor" {
				p := (*packageDescriptor)(reflect.ValueOf(desc).UnsafePointer())
				for _, file := range p.files {
					gopkger, ok := file.(interface{ GoPackagePath() string })
					if ok {
						strVal := gopkger.GoPackagePath()
						s := (*reflect.StringHeader)(unsafe.Pointer(&strVal))
						if s.Data < dataEnd && s.Data >= dataStart {
							delete(globalFiles.descsByName, k)
						}
					}
				}
			} else {
				panic(fmt.Sprintf("unexpected descriptor: %T", desc))
			}
		}
	}

	for path, files := range globalFiles.filesByPath {
		for i, file := range files {
			gopkger, ok := file.(interface{ GoPackagePath() string })
			if ok {
				strVal := gopkger.GoPackagePath()
				s := (*reflect.StringHeader)(unsafe.Pointer(&strVal))
				if s.Data < dataEnd && s.Data >= dataStart {
					globalFiles.filesByPath[path] = append(globalFiles.filesByPath[path][:i], globalFiles.filesByPath[path][i+1:]...)
				}
			}
		}
	}
}
