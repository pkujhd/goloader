//go:build darwin && !vmmap_fallback
// +build darwin,!vmmap_fallback

package vmmap

/*

#include <mach/mach.h>

#if defined(__x86_64__)
#include <mach/mach_vm.h>
#endif

#if defined(__arm64__)
#include "mach_vm.h"
#endif

mach_port_t get_mach_task_self() {
	return mach_task_self();
}
*/
import "C"
import (
	"fmt"
	"github.com/pkujhd/goloader/mmap/mapping"
	"math"
	"unsafe"
)

func Vmmap() ([]mapping.Mapping, error) {
	// TODO - could populate more stuff from vm_region_recurse_info_t and libproc (e.g. permissions etc into mappings), for now only addresses are populated
	var start C.mach_vm_address_t = 0
	var end C.mach_vm_address_t = math.MaxUint64
	var depth C.uint32_t = 2048

	var info C.vm_region_submap_info_data_64_t
	var count C.mach_msg_type_number_t = C.VM_REGION_SUBMAP_INFO_COUNT_64
	task := C.get_mach_task_self()

	var mappings []mapping.Mapping
	isFirst := true
	for {
		var address = start
		var size C.mach_vm_size_t = 0
		var depth0 = depth
		var kr C.kern_return_t = C.mach_vm_region_recurse(task, &address, &size,
			&depth0, (C.vm_region_recurse_info_t)(unsafe.Pointer(&info)), &count)
		if kr != C.KERN_SUCCESS || address > end {
			if isFirst {
				if start == end {
					return nil, fmt.Errorf("no virtual memory region contains address 0x%x\n", uintptr(start))
				} else {
					return nil, fmt.Errorf("no virtual memory region intersects 0x%x-0x%x\n", uintptr(start), uintptr(end))
				}
			}
			break
		}

		isFirst = false
		mappings = append(mappings, mapping.Mapping{
			StartAddr: uintptr(address),
			EndAddr:   uintptr(address + size),
		})
		start = address + size
	}
	return mappings, nil
}
