//go:build go1.18 && !go1.21
// +build go1.18,!go1.21

package obj

import "cmd/objfile/objabi"

const (
	// If you add a FuncID, you probably also want to add an entry to the map in
	// ../../cmd/internal/objabi/funcid.go

	FuncIDNormal              = objabi.FuncID_normal
	FuncID_abort              = objabi.FuncID_abort
	FuncID_asmcgocall         = objabi.FuncID_asmcgocall
	FuncID_asyncPreempt       = objabi.FuncID_asyncPreempt
	FuncID_cgocallback        = objabi.FuncID_cgocallback
	FuncID_debugCallV2        = objabi.FuncID_debugCallV2
	FuncID_gcBgMarkWorker     = objabi.FuncID_gcBgMarkWorker
	FuncID_goexit             = objabi.FuncID_goexit
	FuncID_gogo               = objabi.FuncID_gogo
	FuncID_gopanic            = objabi.FuncID_gopanic
	FuncID_handleAsyncEvent   = objabi.FuncID_handleAsyncEvent
	FuncID_mcall              = objabi.FuncID_mcall
	FuncID_morestack          = objabi.FuncID_morestack
	FuncID_mstart             = objabi.FuncID_mstart
	FuncID_panicwrap          = objabi.FuncID_panicwrap
	FuncID_rt0_go             = objabi.FuncID_rt0_go
	FuncID_runfinq            = objabi.FuncID_runfinq
	FuncID_runtime_main       = objabi.FuncID_runtime_main
	FuncID_sigpanic           = objabi.FuncID_sigpanic
	FuncID_systemstack        = objabi.FuncID_systemstack
	FuncID_systemstack_switch = objabi.FuncID_systemstack_switch
	FuncIDWrapper             = objabi.FuncID_wrapper
)
